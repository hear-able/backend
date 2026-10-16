package volunteer

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"time"

	"hearable/backend/package/audio"
	apperrors "hearable/backend/package/errors"
	"hearable/backend/package/jsonresponder"
	"hearable/backend/package/middleware"
	"hearable/backend/package/request"
)

// Module submits upload up to 100 MB and then call the AI and storage, so
// they get longer deadlines than the server defaults.
const (
	uploadReadTimeout  = 3 * time.Minute
	uploadWriteTimeout = 5 * time.Minute

	// multipartMemory keeps small forms in memory; larger submits spill to
	// temp files which are removed when the request ends.
	multipartMemory = 32 << 20
)

type serviceInterface interface {
	Transcribe(ctx context.Context, file AudioFile) (TranscriptionResponse, error)
	CreateListeningModule(ctx context.Context, userID string, req CreateListeningRequest, files []AudioFile) (CreatedModule, error)
	CreateSpeakingModule(ctx context.Context, userID string, req CreateSpeakingRequest) (CreatedModule, error)
}

type Handler struct {
	service       serviceInterface
	jsonResponder jsonresponder.JSONResponder
}

func NewHandler(service serviceInterface, jr jsonresponder.JSONResponder) *Handler {
	return &Handler{service: service, jsonResponder: jr}
}

// Transcribe accepts multipart/form-data with a single "audio" file and
// returns the transcription and a suggested instruction. Nothing is stored.
func (h *Handler) Transcribe(w http.ResponseWriter, r *http.Request) {
	extendDeadlines(w)
	if err := parseMultipart(r, MaxAudioBytes+(1<<20)); err != nil {
		h.jsonResponder.WriteError(w, r, err)
		return
	}
	defer r.MultipartForm.RemoveAll()

	file, err := readAudio(r, "audio", "Audio")
	if err != nil {
		h.jsonResponder.WriteError(w, r, err)
		return
	}
	resp, err := h.service.Transcribe(r.Context(), file)
	if err != nil {
		h.jsonResponder.WriteError(w, r, err)
		return
	}
	h.write(w, r, http.StatusOK, resp)
}

// CreateListeningModule accepts multipart/form-data with:
//   - payload: JSON CreateListeningRequest
//   - audio_0 .. audio_{n-1}: one audio file per question, in order
func (h *Handler) CreateListeningModule(w http.ResponseWriter, r *http.Request) {
	id, ok := h.identity(w, r)
	if !ok {
		return
	}
	extendDeadlines(w)
	if err := parseMultipart(r, multipartMemory); err != nil {
		h.jsonResponder.WriteError(w, r, err)
		return
	}
	defer r.MultipartForm.RemoveAll()

	var req CreateListeningRequest
	if err := decodePayload(r.FormValue("payload"), &req); err != nil {
		h.jsonResponder.WriteError(w, r, err)
		return
	}
	if err := validateCount(len(req.Questions), "soal"); err != nil {
		h.jsonResponder.WriteError(w, r, err)
		return
	}

	files := make([]AudioFile, len(req.Questions))
	for i := range req.Questions {
		f, err := readAudio(r, fmt.Sprintf("audio_%d", i), fmt.Sprintf("Audio soal %d", i+1))
		if err != nil {
			h.jsonResponder.WriteError(w, r, err)
			return
		}
		files[i] = f
	}

	module, err := h.service.CreateListeningModule(r.Context(), id.UserID, req, files)
	if err != nil {
		h.jsonResponder.WriteError(w, r, err)
		return
	}
	h.write(w, r, http.StatusCreated, module)
}

func (h *Handler) CreateSpeakingModule(w http.ResponseWriter, r *http.Request) {
	id, ok := h.identity(w, r)
	if !ok {
		return
	}
	var req CreateSpeakingRequest
	if err := request.DecodeJSON(r, &req); err != nil {
		h.jsonResponder.WriteError(w, r, err)
		return
	}
	module, err := h.service.CreateSpeakingModule(r.Context(), id.UserID, req)
	if err != nil {
		h.jsonResponder.WriteError(w, r, err)
		return
	}
	h.write(w, r, http.StatusCreated, module)
}

func extendDeadlines(w http.ResponseWriter) {
	rc := http.NewResponseController(w)
	_ = rc.SetReadDeadline(time.Now().Add(uploadReadTimeout))
	_ = rc.SetWriteDeadline(time.Now().Add(uploadWriteTimeout))
}

func parseMultipart(r *http.Request, maxMemory int64) error {
	if err := r.ParseMultipartForm(maxMemory); err != nil {
		var maxErr *http.MaxBytesError
		if errors.As(err, &maxErr) {
			return apperrors.New(http.StatusRequestEntityTooLarge, "payload_too_large", "Ukuran permintaan terlalu besar. Setiap audio maksimal 10 MB.")
		}
		return apperrors.BadRequest("Permintaan harus berupa multipart/form-data.")
	}
	return nil
}

func decodePayload(raw string, dst any) error {
	if raw == "" {
		return apperrors.BadRequest("Field payload wajib diisi.")
	}
	dec := json.NewDecoder(bytes.NewReader([]byte(raw)))
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil || dec.More() {
		return apperrors.BadRequest("Field payload harus berisi satu objek JSON yang valid.")
	}
	return nil
}

func readAudio(r *http.Request, field, name string) (AudioFile, error) {
	file, _, err := r.FormFile(field)
	if errors.Is(err, http.ErrMissingFile) {
		return AudioFile{}, apperrors.Unprocessable(name + " wajib diunggah.")
	}
	if err != nil {
		return AudioFile{}, apperrors.BadRequest(name + " tidak dapat dibaca.")
	}
	defer file.Close()
	return readFile(file, name)
}

func readFile(file multipart.File, name string) (AudioFile, error) {
	data, err := io.ReadAll(io.LimitReader(file, MaxAudioBytes+1))
	if err != nil {
		return AudioFile{}, apperrors.BadRequest(name + " tidak dapat dibaca.")
	}
	if len(data) > MaxAudioBytes {
		return AudioFile{}, apperrors.New(http.StatusRequestEntityTooLarge, "payload_too_large", name+" maksimal 10 MB.")
	}
	if len(data) == 0 {
		return AudioFile{}, apperrors.Unprocessable(name + " kosong.")
	}
	format, ok := audio.Detect(data)
	if !ok {
		return AudioFile{}, apperrors.New(http.StatusUnsupportedMediaType, "unsupported_media_type",
			name+": format tidak didukung. Gunakan mp3, m4a/mp4, wav, ogg, webm, atau flac.")
	}
	return AudioFile{Data: data, Format: format}, nil
}

func (h *Handler) identity(w http.ResponseWriter, r *http.Request) (middleware.Identity, bool) {
	id, ok := middleware.IdentityFromContext(r.Context())
	if !ok {
		h.jsonResponder.WriteError(w, r, apperrors.Unauthorized("Token akses diperlukan."))
	}
	return id, ok
}

func (h *Handler) write(w http.ResponseWriter, r *http.Request, status int, data any) {
	if err := h.jsonResponder.Write(w, status, data); err != nil {
		h.jsonResponder.WriteError(w, r, err)
	}
}
