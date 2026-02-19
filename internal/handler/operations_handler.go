package handler

import (
	"encoding/json"
	"net/http"
	"strings"

	"go-file-explorer/internal/model"
	"go-file-explorer/internal/service"
	"go-file-explorer/pkg/apierror"
)

type OperationsHandler struct {
	service *service.OperationsService
}

func NewOperationsHandler(service *service.OperationsService) *OperationsHandler {
	return &OperationsHandler{service: service}
}

func (h *OperationsHandler) Rename(w http.ResponseWriter, r *http.Request) {
	defer r.Body.Close()

	var payload model.RenameRequest
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		writeError(w, apierror.New("BAD_REQUEST", "invalid JSON body", "", http.StatusBadRequest))
		return
	}

	if strings.TrimSpace(payload.Path) == "" || strings.TrimSpace(payload.NewName) == "" {
		writeError(w, apierror.New("BAD_REQUEST", "path and new_name are required", "", http.StatusBadRequest))
		return
	}

	result, err := h.service.Rename(r.Context(), payload.Path, payload.NewName, actorFromRequest(r))
	if err != nil {
		writeError(w, err)
		return
	}

	writeSuccess(w, http.StatusOK, result, nil)
}

func (h *OperationsHandler) Move(w http.ResponseWriter, r *http.Request) {
	defer r.Body.Close()

	var payload model.MoveRequest
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		writeError(w, apierror.New("BAD_REQUEST", "invalid JSON body", "", http.StatusBadRequest))
		return
	}

	result, err := h.service.Move(r.Context(), payload.Sources, payload.Destination, actorFromRequest(r))
	if err != nil {
		writeError(w, err)
		return
	}

	writeSuccess(w, http.StatusOK, result, nil)
}

func (h *OperationsHandler) Copy(w http.ResponseWriter, r *http.Request) {
	defer r.Body.Close()

	var payload model.MoveRequest
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		writeError(w, apierror.New("BAD_REQUEST", "invalid JSON body", "", http.StatusBadRequest))
		return
	}

	result, err := h.service.Copy(r.Context(), payload.Sources, payload.Destination, actorFromRequest(r))
	if err != nil {
		writeError(w, err)
		return
	}

	writeSuccess(w, http.StatusOK, result, nil)
}

func (h *OperationsHandler) Delete(w http.ResponseWriter, r *http.Request) {
	defer r.Body.Close()

	var payload model.DeleteRequest
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		writeError(w, apierror.New("BAD_REQUEST", "invalid JSON body", "", http.StatusBadRequest))
		return
	}

	result, err := h.service.Delete(r.Context(), payload.Paths, actorFromRequest(r))
	if err != nil {
		writeError(w, err)
		return
	}

	writeSuccess(w, http.StatusOK, result, nil)
}

func (h *OperationsHandler) Restore(w http.ResponseWriter, r *http.Request) {
	defer r.Body.Close()

	var payload model.RestoreRequest
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		writeError(w, apierror.New("BAD_REQUEST", "invalid JSON body", "", http.StatusBadRequest))
		return
	}

	result, err := h.service.Restore(r.Context(), payload.Paths, actorFromRequest(r))
	if err != nil {
		writeError(w, err)
		return
	}

	writeSuccess(w, http.StatusOK, result, nil)
}
