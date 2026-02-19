package handler

import (
	"net/http"
	"strings"

	"go-file-explorer/internal/model"
	"go-file-explorer/internal/service"
)

type AuditHandler struct {
	service *service.AuditService
}

func NewAuditHandler(service *service.AuditService) *AuditHandler {
	return &AuditHandler{service: service}
}

func (h *AuditHandler) List(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query()

	items, meta, err := h.service.Query(model.AuditQuery{
		Action:  strings.TrimSpace(query.Get("action")),
		ActorID: strings.TrimSpace(query.Get("actor_id")),
		Status:  strings.TrimSpace(query.Get("status")),
		Path:    strings.TrimSpace(query.Get("path")),
		From:    strings.TrimSpace(query.Get("from")),
		To:      strings.TrimSpace(query.Get("to")),
		Page:    parseIntOrDefault(query.Get("page"), 1),
		Limit:   parseIntOrDefault(query.Get("limit"), 50),
	})
	if err != nil {
		writeError(w, err)
		return
	}

	writeSuccess(w, http.StatusOK, model.AuditListData{Items: items}, &meta)
}
