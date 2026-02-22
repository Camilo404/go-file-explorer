package handler

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"

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

	result, err := h.service.Move(r.Context(), payload.Sources, payload.Destination, payload.ConflictPolicy, actorFromRequest(r))
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

	result, err := h.service.Copy(r.Context(), payload.Sources, payload.Destination, payload.ConflictPolicy, actorFromRequest(r))
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

func (h *OperationsHandler) ListTrash(w http.ResponseWriter, r *http.Request) {
	includeRestored := false
	rawIncludeRestored := strings.TrimSpace(r.URL.Query().Get("include_restored"))
	if rawIncludeRestored != "" {
		parsedValue, err := strconv.ParseBool(rawIncludeRestored)
		if err != nil {
			writeError(w, apierror.New("BAD_REQUEST", "include_restored must be true or false", "include_restored", http.StatusBadRequest))
			return
		}
		includeRestored = parsedValue
	}

	records, err := h.service.ListTrash(r.Context(), includeRestored)
	if err != nil {
		writeError(w, err)
		return
	}

	writeSuccess(w, http.StatusOK, map[string]any{"items": records}, nil)
}

func (h *OperationsHandler) PermanentDeleteTrash(w http.ResponseWriter, r *http.Request) {
	trashID := chi.URLParam(r, "id")
	if trashID == "" {
		writeError(w, apierror.New("BAD_REQUEST", "trash id is required", "id", http.StatusBadRequest))
		return
	}

	if err := h.service.PermanentDeleteTrash(r.Context(), trashID, actorFromRequest(r)); err != nil {
		writeError(w, apierror.New("NOT_FOUND", "trash item not found", trashID, http.StatusNotFound))
		return
	}

	writeSuccess(w, http.StatusOK, map[string]any{"deleted": true}, nil)
}

func (h *OperationsHandler) EmptyTrash(w http.ResponseWriter, r *http.Request) {
	count, err := h.service.EmptyTrash(r.Context(), actorFromRequest(r))
	if err != nil {
		writeError(w, err)
		return
	}

	writeSuccess(w, http.StatusOK, map[string]any{"deleted_count": count}, nil)
}

func (h *OperationsHandler) Compress(w http.ResponseWriter, r *http.Request) {
	defer r.Body.Close()

	var payload model.CompressRequest
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		writeError(w, apierror.New("BAD_REQUEST", "invalid JSON body", "", http.StatusBadRequest))
		return
	}

	result, err := h.service.Compress(r.Context(), payload.Sources, payload.Destination, payload.Name, actorFromRequest(r))
	if err != nil {
		writeError(w, err)
		return
	}

	writeSuccess(w, http.StatusOK, result, nil)
}

func (h *OperationsHandler) Decompress(w http.ResponseWriter, r *http.Request) {
	defer r.Body.Close()

	var payload model.DecompressRequest
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		writeError(w, apierror.New("BAD_REQUEST", "invalid JSON body", "", http.StatusBadRequest))
		return
	}

	result, err := h.service.Decompress(r.Context(), payload.Source, payload.Destination, payload.ConflictPolicy, actorFromRequest(r))
	if err != nil {
		// If it's a conflict error with data, we want to return the data (list of conflicts)
		if apiErr, ok := err.(*apierror.APIError); ok && apiErr.Code == "CONFLICT" {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusConflict)
			// Return a JSON response manually
			_ = json.NewEncoder(w).Encode(map[string]any{
				"success": false,
				"error":   apiErr,
				"data":    result,
			})
			return
		}
		writeError(w, err)
		return
	}

	writeSuccess(w, http.StatusOK, result, nil)
}
