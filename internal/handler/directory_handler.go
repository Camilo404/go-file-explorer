package handler

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"go-file-explorer/internal/model"
	"go-file-explorer/internal/service"
	"go-file-explorer/pkg/apierror"
)

type DirectoryHandler struct {
	service *service.DirectoryService
}

func NewDirectoryHandler(service *service.DirectoryService) *DirectoryHandler {
	return &DirectoryHandler{service: service}
}

func (h *DirectoryHandler) List(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query()
	requestedPath := query.Get("path")

	page := parseIntOrDefault(query.Get("page"), 1)
	limit := parseIntOrDefault(query.Get("limit"), 50)
	sortBy := strings.TrimSpace(query.Get("sort"))
	order := strings.TrimSpace(query.Get("order"))

	data, meta, err := h.service.List(r.Context(), requestedPath, page, limit, sortBy, order)
	if err != nil {
		writeError(w, err)
		return
	}

	writeSuccess(w, http.StatusOK, data, &meta)
}

func (h *DirectoryHandler) Create(w http.ResponseWriter, r *http.Request) {
	defer r.Body.Close()

	var payload model.CreateDirectoryRequest
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		writeError(w, apierror.New("BAD_REQUEST", "invalid JSON body", "", http.StatusBadRequest))
		return
	}

	if strings.TrimSpace(payload.Name) == "" {
		writeError(w, apierror.New("BAD_REQUEST", "directory name is required", "name", http.StatusBadRequest))
		return
	}

	data, err := h.service.Create(r.Context(), payload.Path, payload.Name)
	if err != nil {
		writeError(w, err)
		return
	}

	writeSuccess(w, http.StatusCreated, data, nil)
}

func parseIntOrDefault(raw string, fallback int) int {
	if strings.TrimSpace(raw) == "" {
		return fallback
	}

	v, err := strconv.Atoi(raw)
	if err != nil {
		return fallback
	}

	return v
}
