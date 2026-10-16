package speaking

import (
	"context"
	"errors"
	"io"
	"net/http"

	"github.com/go-chi/chi/v5"

	"hearable/backend/package/audio"
	apperrors "hearable/backend/package/errors"
	"hearable/backend/package/jsonresponder"
	"hearable/backend/package/middleware"
	"hearable/backend/package/request"
)

type serviceInterface interface {
	ListModules(ctx context.Context, level *int) ([]Module, error)
	StartSession(ctx context.Context, userID, moduleID string) (SessionDetail, error)
	GetSession(ctx context.Context, userID, sessionID string) (SessionDetail, error)
	SubmitAttempt(ctx context.Context, userID, sessionID string, in AttemptInput) (Attempt, error)
	CompleteSession(ctx context.Context, userID, sessionID string) (CompleteResponse, error)
}

type Handler struct {
	service       serviceInterface
	jsonResponder jsonresponder.JSONResponder
}

func NewHandler(service serviceInterface, jr jsonresponder.JSONResponder) *Handler {
	return &Handler{service: service, jsonResponder: jr}
}

func (h *Handler) ListModules(w http.ResponseWriter, r *http.Request) {
	level, err := request.ParseDifficulty(r)
	if err != nil {
		h.jsonResponder.WriteError(w, r, err)
		return
	}
	modules, err := h.service.ListModules(r.Context(), level)
	if err != nil {
		h.jsonResponder.WriteError(w, r, err)
		return
	}
	h.write(w, r, http.StatusOK, map[string]any{"modules": modules})
}

func (h *Handler) StartSession(w http.ResponseWriter, r *http.Request) {
	id, ok := h.identity(w, r)
	if !ok {
		return
	}
	var req StartSessionRequest
	if err := request.DecodeJSON(r, &req); err != nil {
		h.jsonResponder.WriteError(w, r, err)
		return
	}
	detail, err := h.service.StartSession(r.Context(), id.UserID, req.ModuleID)
	if err != nil {
		h.jsonResponder.WriteError(w, r, err)
		return
	}
	h.write(w, r, http.StatusCreated, detail)
}

func (h *Handler) GetSession(w http.ResponseWriter, r *http.Request) {
	id, ok := h.identity(w, r)
	if !ok {
		return
	}
	detail, err := h.service.GetSession(r.Context(), id.UserID, chi.URLParam(r, "sessionID"))
	if err != nil {
		h.jsonResponder.WriteError(w, r, err)
		return
	}
	h.write(w, r, http.StatusOK, detail)
}

// SubmitAttempt accepts multipart/form-data with fields:
//   - sentence_id (required)
//   - audio (file, required unless skipped=true)
//   - skipped ("true" to skip the sentence)
func (h *Handler) SubmitAttempt(w http.ResponseWriter, r *http.Request) {
	id, ok := h.identity(w, r)
	if !ok {
		return
	}
	in, err := parseAttempt(r)
	if err != nil {
		h.jsonResponder.WriteError(w, r, err)
		return
	}
	attempt, err := h.service.SubmitAttempt(r.Context(), id.UserID, chi.URLParam(r, "sessionID"), in)
	if err != nil {
		h.jsonResponder.WriteError(w, r, err)
		return
	}
	h.write(w, r, http.StatusCreated, attempt)
}

func (h *Handler) CompleteSession(w http.ResponseWriter, r *http.Request) {
	id, ok := h.identity(w, r)
	if !ok {
		return
	}
	resp, err := h.service.CompleteSession(r.Context(), id.UserID, chi.URLParam(r, "sessionID"))
	if err != nil {
		h.jsonResponder.WriteError(w, r, err)
		return
	}
	h.write(w, r, http.StatusOK, resp)
}

func parseAttempt(r *http.Request) (AttemptInput, error) {
	tooLarge := apperrors.New(http.StatusRequestEntityTooLarge, "payload_too_large", "Rekaman maksimal 10 MB.")

	// Keep everything in memory (bounded by the route's MaxBodyBytes) so the
	// recording is never written to disk.
	if err := r.ParseMultipartForm(MaxAudioBytes + (1 << 20)); err != nil {
		var maxErr *http.MaxBytesError
		if errors.As(err, &maxErr) {
			return AttemptInput{}, tooLarge
		}
		return AttemptInput{}, apperrors.BadRequest("Permintaan harus berupa multipart/form-data.")
	}
	defer r.MultipartForm.RemoveAll()

	in := AttemptInput{
		SentenceID: r.FormValue("sentence_id"),
		Skipped:    r.FormValue("skipped") == "true",
	}

	file, _, err := r.FormFile("audio")
	if errors.Is(err, http.ErrMissingFile) {
		if !in.Skipped {
			return AttemptInput{}, apperrors.Unprocessable("Rekaman suara (audio) wajib diunggah.")
		}
		return in, nil
	}
	if err != nil {
		return AttemptInput{}, apperrors.BadRequest("Berkas audio tidak dapat dibaca.")
	}
	defer file.Close()

	if in.Skipped {
		return AttemptInput{}, apperrors.Unprocessable("Kalimat yang dilewati tidak boleh berisi rekaman.")
	}

	data, err := io.ReadAll(io.LimitReader(file, MaxAudioBytes+1))
	if err != nil {
		return AttemptInput{}, apperrors.BadRequest("Berkas audio tidak dapat dibaca.")
	}
	if len(data) > MaxAudioBytes {
		return AttemptInput{}, tooLarge
	}
	if len(data) == 0 {
		return AttemptInput{}, apperrors.Unprocessable("Berkas audio kosong.")
	}
	format, ok := audio.Detect(data)
	if !ok {
		return AttemptInput{}, apperrors.New(http.StatusUnsupportedMediaType, "unsupported_media_type",
			"Format audio tidak didukung. Gunakan webm, ogg, m4a/mp4, wav, mp3, atau flac.")
	}
	in.Audio = data
	in.Filename = "recording." + format.Ext
	return in, nil
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
