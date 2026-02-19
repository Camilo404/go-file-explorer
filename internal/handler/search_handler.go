package handler

import (
	"net/http"
	"strings"

	"go-file-explorer/internal/service"
)

type SearchHandler struct {
	service *service.SearchService
}

func NewSearchHandler(service *service.SearchService) *SearchHandler {
	return &SearchHandler{service: service}
}

func (h *SearchHandler) Search(w http.ResponseWriter, r *http.Request) {
	query := strings.TrimSpace(r.URL.Query().Get("q"))
	path := strings.TrimSpace(r.URL.Query().Get("path"))
	itemType := strings.TrimSpace(r.URL.Query().Get("type"))
	ext := strings.TrimSpace(r.URL.Query().Get("ext"))
	page := parseIntOrDefault(r.URL.Query().Get("page"), 1)
	limit := parseIntOrDefault(r.URL.Query().Get("limit"), 20)

	data, meta, err := h.service.Search(r.Context(), query, path, itemType, ext, page, limit)
	if err != nil {
		writeError(w, err)
		return
	}

	writeSuccess(w, http.StatusOK, data, &meta)
}
