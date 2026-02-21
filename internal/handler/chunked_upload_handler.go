package handler

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"go-file-explorer/internal/model"
	"go-file-explorer/internal/service"
	"go-file-explorer/pkg/apierror"
)

type ChunkedUploadHandler struct {
	service      *service.ChunkedUploadService
	maxChunkSize int64
}

func NewChunkedUploadHandler(service *service.ChunkedUploadService, maxChunkSize int64) *ChunkedUploadHandler {
	return &ChunkedUploadHandler{service: service, maxChunkSize: maxChunkSize}
}

// Init handles POST /api/v1/uploads/init
func (h *ChunkedUploadHandler) Init(w http.ResponseWriter, r *http.Request) {
	defer r.Body.Close()

	var req model.ChunkedUploadInitRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, apierror.New("BAD_REQUEST", "invalid JSON body", err.Error(), http.StatusBadRequest))
		return
	}

	// Server-side cap on chunk size for safety.
	if req.ChunkSize > h.maxChunkSize {
		writeError(w, apierror.New(
			"BAD_REQUEST",
			"chunk_size exceeds server maximum",
			strconv.FormatInt(h.maxChunkSize, 10),
			http.StatusBadRequest,
		))
		return
	}

	resp, err := h.service.InitUpload(r.Context(), req)
	if err != nil {
		writeError(w, err)
		return
	}

	writeSuccess(w, http.StatusCreated, resp, nil)
}

// UploadChunk handles PUT /api/v1/uploads/{upload_id}/chunks/{chunk_index}
func (h *ChunkedUploadHandler) UploadChunk(w http.ResponseWriter, r *http.Request) {
	uploadID := chi.URLParam(r, "upload_id")
	if uploadID == "" {
		writeError(w, apierror.New("BAD_REQUEST", "upload_id is required", "", http.StatusBadRequest))
		return
	}

	chunkIndexStr := chi.URLParam(r, "chunk_index")
	chunkIndex, err := strconv.Atoi(chunkIndexStr)
	if err != nil {
		writeError(w, apierror.New("BAD_REQUEST", "chunk_index must be an integer", chunkIndexStr, http.StatusBadRequest))
		return
	}

	// Limit body size to chunk max + 1 MB headroom.
	r.Body = http.MaxBytesReader(w, r.Body, h.maxChunkSize+1024*1024)
	defer r.Body.Close()

	resp, err := h.service.WriteChunk(r.Context(), uploadID, chunkIndex, r.Body)
	if err != nil {
		writeError(w, err)
		return
	}

	writeSuccess(w, http.StatusOK, resp, nil)
}

// Complete handles POST /api/v1/uploads/{upload_id}/complete
func (h *ChunkedUploadHandler) Complete(w http.ResponseWriter, r *http.Request) {
	uploadID := chi.URLParam(r, "upload_id")
	if uploadID == "" {
		writeError(w, apierror.New("BAD_REQUEST", "upload_id is required", "", http.StatusBadRequest))
		return
	}

	item, err := h.service.CompleteUpload(r.Context(), uploadID)
	if err != nil {
		writeError(w, err)
		return
	}

	writeSuccess(w, http.StatusOK, model.ChunkedUploadCompleteResponse{File: item}, nil)
}

// Abort handles DELETE /api/v1/uploads/{upload_id}
func (h *ChunkedUploadHandler) Abort(w http.ResponseWriter, r *http.Request) {
	uploadID := chi.URLParam(r, "upload_id")
	if uploadID == "" {
		writeError(w, apierror.New("BAD_REQUEST", "upload_id is required", "", http.StatusBadRequest))
		return
	}

	if err := h.service.AbortUpload(r.Context(), uploadID); err != nil {
		writeError(w, err)
		return
	}

	writeSuccess(w, http.StatusOK, map[string]string{"upload_id": uploadID, "status": "aborted"}, nil)
}
