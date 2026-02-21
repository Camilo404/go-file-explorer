package handler

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"

	"go-file-explorer/internal/model"
	"go-file-explorer/internal/service"
	"go-file-explorer/pkg/apierror"
)

type UserHandler struct {
	service *service.AuthService
}

func NewUserHandler(service *service.AuthService) *UserHandler {
	return &UserHandler{service: service}
}

func (h *UserHandler) List(w http.ResponseWriter, r *http.Request) {
	users := h.service.ListUsers()
	writeSuccess(w, http.StatusOK, model.AuthUserList{Users: users}, nil)
}

func (h *UserHandler) Get(w http.ResponseWriter, r *http.Request) {
	userID := chi.URLParam(r, "id")
	if userID == "" {
		writeError(w, apierror.New("BAD_REQUEST", "user id is required", "id", http.StatusBadRequest))
		return
	}

	user, err := h.service.GetUserByID(userID)
	if err != nil {
		writeError(w, err)
		return
	}

	writeSuccess(w, http.StatusOK, user, nil)
}

func (h *UserHandler) Update(w http.ResponseWriter, r *http.Request) {
	defer r.Body.Close()

	userID := chi.URLParam(r, "id")
	if userID == "" {
		writeError(w, apierror.New("BAD_REQUEST", "user id is required", "id", http.StatusBadRequest))
		return
	}

	var payload model.UpdateUserRequest
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		writeError(w, apierror.New("BAD_REQUEST", "invalid JSON body", "", http.StatusBadRequest))
		return
	}

	user, err := h.service.UpdateUser(userID, payload.Role)
	if err != nil {
		writeError(w, err)
		return
	}

	writeSuccess(w, http.StatusOK, user, nil)
}

func (h *UserHandler) Delete(w http.ResponseWriter, r *http.Request) {
	userID := chi.URLParam(r, "id")
	if userID == "" {
		writeError(w, apierror.New("BAD_REQUEST", "user id is required", "id", http.StatusBadRequest))
		return
	}

	claims := actorFromRequest(r)
	if err := h.service.DeleteUser(userID, claims.UserID); err != nil {
		writeError(w, err)
		return
	}

	writeSuccess(w, http.StatusOK, map[string]any{"deleted": true}, nil)
}
