package handler

import (
	"encoding/json"
	"net/http"
	"strings"

	"go-file-explorer/internal/middleware"
	"go-file-explorer/internal/model"
	"go-file-explorer/internal/service"
	"go-file-explorer/pkg/apierror"
)

type AuthHandler struct {
	service *service.AuthService
}

func NewAuthHandler(service *service.AuthService) *AuthHandler {
	return &AuthHandler{service: service}
}

func (h *AuthHandler) Login(w http.ResponseWriter, r *http.Request) {
	defer r.Body.Close()

	var payload model.LoginRequest
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		writeError(w, apierror.New("BAD_REQUEST", "invalid JSON body", "", http.StatusBadRequest))
		return
	}

	tokens, err := h.service.Login(payload.Username, payload.Password)
	if err != nil {
		writeError(w, err)
		return
	}

	writeSuccess(w, http.StatusOK, tokens, nil)
}

func (h *AuthHandler) Register(w http.ResponseWriter, r *http.Request) {
	defer r.Body.Close()

	var payload model.RegisterRequest
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		writeError(w, apierror.New("BAD_REQUEST", "invalid JSON body", "", http.StatusBadRequest))
		return
	}

	user, err := h.service.Register(payload.Username, payload.Password, payload.Role)
	if err != nil {
		writeError(w, err)
		return
	}

	writeSuccess(w, http.StatusCreated, user, nil)
}

func (h *AuthHandler) Refresh(w http.ResponseWriter, r *http.Request) {
	defer r.Body.Close()

	var payload model.RefreshRequest
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		writeError(w, apierror.New("BAD_REQUEST", "invalid JSON body", "", http.StatusBadRequest))
		return
	}

	payload.RefreshToken = strings.TrimSpace(payload.RefreshToken)
	if payload.RefreshToken == "" {
		writeError(w, apierror.New("BAD_REQUEST", "refresh_token is required", "refresh_token", http.StatusBadRequest))
		return
	}

	tokens, err := h.service.Refresh(payload.RefreshToken)
	if err != nil {
		writeError(w, err)
		return
	}

	writeSuccess(w, http.StatusOK, tokens, nil)
}

func (h *AuthHandler) Logout(w http.ResponseWriter, r *http.Request) {
	defer r.Body.Close()

	var payload model.RefreshRequest
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		writeError(w, apierror.New("BAD_REQUEST", "invalid JSON body", "", http.StatusBadRequest))
		return
	}

	h.service.Logout(strings.TrimSpace(payload.RefreshToken))
	writeSuccess(w, http.StatusOK, map[string]any{"logged_out": true}, nil)
}

func (h *AuthHandler) Me(w http.ResponseWriter, r *http.Request) {
	claims, ok := middleware.ClaimsFromContext(r.Context())
	if !ok {
		writeError(w, apierror.New("UNAUTHORIZED", "authentication required", "", http.StatusUnauthorized))
		return
	}

	user, err := h.service.GetUserByID(claims.UserID)
	if err != nil {
		writeError(w, err)
		return
	}

	writeSuccess(w, http.StatusOK, user, nil)
}
