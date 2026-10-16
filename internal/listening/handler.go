package listening

import (
	"context"
	"net/http"

	"github.com/go-chi/chi/v5"

	apperrors "hearable/backend/package/errors"
	"hearable/backend/package/jsonresponder"
	"hearable/backend/package/middleware"
	"hearable/backend/package/request"
)

type serviceInterface interface {
	ListModules(ctx context.Context, level *int) ([]Module, error)
	StartSession(ctx context.Context, userID, moduleID string) (SessionDetail, error)
	GetSession(ctx context.Context, userID, sessionID string) (SessionDetail, error)
	SubmitAnswer(ctx context.Context, userID, sessionID string, req SubmitAnswerRequest) (Answer, error)
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

func (h *Handler) SubmitAnswer(w http.ResponseWriter, r *http.Request) {
	id, ok := h.identity(w, r)
	if !ok {
		return
	}
	var req SubmitAnswerRequest
	if err := request.DecodeJSON(r, &req); err != nil {
		h.jsonResponder.WriteError(w, r, err)
		return
	}
	answer, err := h.service.SubmitAnswer(r.Context(), id.UserID, chi.URLParam(r, "sessionID"), req)
	if err != nil {
		h.jsonResponder.WriteError(w, r, err)
		return
	}
	h.write(w, r, http.StatusCreated, answer)
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
