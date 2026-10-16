package progress

import (
	"context"
	"net/http"
	"strconv"

	apperrors "hearable/backend/package/errors"
	"hearable/backend/package/jsonresponder"
	"hearable/backend/package/middleware"
)

type serviceInterface interface {
	Activity(ctx context.Context, userID string, weekOf string) (Activity, error)
	History(ctx context.Context, userID string, q HistoryQuery) (History, error)
}

type Handler struct {
	service       serviceInterface
	jsonResponder jsonresponder.JSONResponder
}

func NewHandler(service serviceInterface, jr jsonresponder.JSONResponder) *Handler {
	return &Handler{service: service, jsonResponder: jr}
}

func (h *Handler) Activity(w http.ResponseWriter, r *http.Request) {
	id, ok := h.identity(w, r)
	if !ok {
		return
	}
	act, err := h.service.Activity(r.Context(), id.UserID, r.URL.Query().Get("week_of"))
	if err != nil {
		h.jsonResponder.WriteError(w, r, err)
		return
	}
	h.write(w, r, http.StatusOK, act)
}

func (h *Handler) History(w http.ResponseWriter, r *http.Request) {
	id, ok := h.identity(w, r)
	if !ok {
		return
	}
	q := HistoryQuery{
		Cursor: r.URL.Query().Get("cursor"),
		Status: r.URL.Query().Get("status"),
	}
	if raw := r.URL.Query().Get("limit"); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil {
			h.jsonResponder.WriteError(w, r, apperrors.BadRequest("limit harus berupa angka."))
			return
		}
		q.Limit = n
	}
	history, err := h.service.History(r.Context(), id.UserID, q)
	if err != nil {
		h.jsonResponder.WriteError(w, r, err)
		return
	}
	h.write(w, r, http.StatusOK, history)
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
