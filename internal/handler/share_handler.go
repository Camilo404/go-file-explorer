package handler

import (
	"encoding/json"
	"mime"
	"net/http"
	"path/filepath"
	"strings"

	"github.com/go-chi/chi/v5"

	"go-file-explorer/internal/middleware"
	"go-file-explorer/internal/model"
	"go-file-explorer/internal/service"
	"go-file-explorer/internal/util"
	"go-file-explorer/pkg/apierror"
)

type ShareHandler struct {
	shares *service.ShareService
	files  *service.FileService
}

func NewShareHandler(shares *service.ShareService, files *service.FileService) *ShareHandler {
	return &ShareHandler{shares: shares, files: files}
}

func (h *ShareHandler) Create(w http.ResponseWriter, r *http.Request) {
	defer r.Body.Close()

	claims, ok := middleware.ClaimsFromContext(r.Context())
	if !ok {
		writeError(w, apierror.New("UNAUTHORIZED", "authentication required", "", http.StatusUnauthorized))
		return
	}

	var payload model.CreateShareRequest
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		writeError(w, apierror.New("BAD_REQUEST", "invalid JSON body", "", http.StatusBadRequest))
		return
	}

	record, err := h.shares.Create(strings.TrimSpace(payload.Path), claims.UserID, strings.TrimSpace(payload.ExpiresIn))
	if err != nil {
		writeError(w, err)
		return
	}

	writeSuccess(w, http.StatusCreated, record, nil)
}

func (h *ShareHandler) List(w http.ResponseWriter, r *http.Request) {
	claims, ok := middleware.ClaimsFromContext(r.Context())
	if !ok {
		writeError(w, apierror.New("UNAUTHORIZED", "authentication required", "", http.StatusUnauthorized))
		return
	}

	records, err := h.shares.List(claims.UserID)
	if err != nil {
		writeError(w, err)
		return
	}

	writeSuccess(w, http.StatusOK, model.ShareListData{Shares: records}, nil)
}

func (h *ShareHandler) Revoke(w http.ResponseWriter, r *http.Request) {
	shareID := chi.URLParam(r, "id")
	if shareID == "" {
		writeError(w, apierror.New("BAD_REQUEST", "share id is required", "id", http.StatusBadRequest))
		return
	}

	claims, ok := middleware.ClaimsFromContext(r.Context())
	if !ok {
		writeError(w, apierror.New("UNAUTHORIZED", "authentication required", "", http.StatusUnauthorized))
		return
	}

	if err := h.shares.Revoke(shareID, claims.UserID); err != nil {
		writeError(w, err)
		return
	}

	writeSuccess(w, http.StatusOK, map[string]any{"revoked": true}, nil)
}

func (h *ShareHandler) PublicDownload(w http.ResponseWriter, r *http.Request) {
	token := chi.URLParam(r, "token")
	if token == "" {
		writeError(w, apierror.New("BAD_REQUEST", "token is required", "token", http.StatusBadRequest))
		return
	}

	record, err := h.shares.ResolveToken(token)
	if err != nil {
		writeError(w, err)
		return
	}

	file, info, mimeType, err := h.files.GetFile(record.Path)
	if err != nil {
		// If it's a directory, serve as zip archive
		resolved, name, archiveErr := h.files.GetDirectoryForArchive(record.Path)
		if archiveErr != nil {
			writeError(w, err)
			return
		}
		w.Header().Set("Content-Type", "application/zip")
		w.Header().Set("Content-Disposition", "attachment; filename=\""+name+".zip\"")
		if streamErr := util.StreamZipFromDirectory(resolved, w); streamErr != nil {
			return
		}
		return
	}
	defer file.Close()

	filename := filepath.Base(record.Path)
	w.Header().Set("Content-Type", mimeType)
	w.Header().Set("Content-Disposition", mime.FormatMediaType("inline", map[string]string{"filename": filename}))
	http.ServeContent(w, r, filename, info.ModTime(), file)
}
