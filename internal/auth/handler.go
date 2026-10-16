package auth

import (
	"context"
	"net/http"

	apperrors "hearable/backend/package/errors"
	"hearable/backend/package/jsonresponder"
	"hearable/backend/package/middleware"
	"hearable/backend/package/request"
)

type Handler struct {
	service       serviceInterface
	jsonResponder jsonresponder.JSONResponder
}

type serviceInterface interface {
	Register(ctx context.Context, req RegisterRequest) (AuthResponse, error)
	Login(ctx context.Context, req LoginRequest) (AuthResponse, error)
	Refresh(ctx context.Context, rawRefreshToken string) (AuthResponse, error)
	Logout(ctx context.Context, userID, rawRefreshToken string) error
	Me(ctx context.Context, userID string) (User, error)
}

func NewHandler(service serviceInterface, jr jsonresponder.JSONResponder) *Handler {
	return &Handler{
		service:       service,
		jsonResponder: jr,
	}
}

func (h *Handler) Register(w http.ResponseWriter, r *http.Request) {
	var req RegisterRequest
	if err := request.DecodeJSON(r, &req); err != nil {
		h.jsonResponder.WriteError(w, r, err)
		return
	}
	resp, err := h.service.Register(r.Context(), req)
	if err != nil {
		h.jsonResponder.WriteError(w, r, err)
		return
	}
	h.write(w, r, http.StatusCreated, resp)
}

func (h *Handler) Login(w http.ResponseWriter, r *http.Request) {
	var req LoginRequest
	if err := request.DecodeJSON(r, &req); err != nil {
		h.jsonResponder.WriteError(w, r, err)
		return
	}
	resp, err := h.service.Login(r.Context(), req)
	if err != nil {
		h.jsonResponder.WriteError(w, r, err)
		return
	}
	h.write(w, r, http.StatusOK, resp)
}

func (h *Handler) Refresh(w http.ResponseWriter, r *http.Request) {
	var req RefreshRequest
	if err := request.DecodeJSON(r, &req); err != nil {
		h.jsonResponder.WriteError(w, r, err)
		return
	}
	resp, err := h.service.Refresh(r.Context(), req.RefreshToken)
	if err != nil {
		h.jsonResponder.WriteError(w, r, err)
		return
	}
	h.write(w, r, http.StatusOK, resp)
}

func (h *Handler) Logout(w http.ResponseWriter, r *http.Request) {
	id, ok := middleware.IdentityFromContext(r.Context())
	if !ok {
		h.jsonResponder.WriteError(w, r, apperrors.Unauthorized("Token akses diperlukan."))
		return
	}
	var req RefreshRequest
	if err := request.DecodeJSON(r, &req); err != nil {
		h.jsonResponder.WriteError(w, r, err)
		return
	}
	if err := h.service.Logout(r.Context(), id.UserID, req.RefreshToken); err != nil {
		h.jsonResponder.WriteError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) Me(w http.ResponseWriter, r *http.Request) {
	id, ok := middleware.IdentityFromContext(r.Context())
	if !ok {
		h.jsonResponder.WriteError(w, r, apperrors.Unauthorized("Token akses diperlukan."))
		return
	}
	user, err := h.service.Me(r.Context(), id.UserID)
	if err != nil {
		h.jsonResponder.WriteError(w, r, err)
		return
	}
	h.write(w, r, http.StatusOK, user)
}

func (h *Handler) write(w http.ResponseWriter, r *http.Request, status int, data any) {
	if err := h.jsonResponder.Write(w, status, data); err != nil {
		h.jsonResponder.WriteError(w, r, err)
	}
}
