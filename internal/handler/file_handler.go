package handler

import (
	"errors"
	"io"
	"mime"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"

	"go-file-explorer/internal/model"
	"go-file-explorer/internal/service"
	"go-file-explorer/internal/util"
	"go-file-explorer/pkg/apierror"
)

type FileHandler struct {
	service       *service.FileService
	maxUploadSize int64
}

func NewFileHandler(service *service.FileService, maxUploadSize int64) *FileHandler {
	return &FileHandler{service: service, maxUploadSize: maxUploadSize}
}

func (h *FileHandler) Upload(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, h.maxUploadSize)

	reader, err := r.MultipartReader()
	if err != nil {
		writeError(w, apierror.New("BAD_REQUEST", "invalid multipart body", "", http.StatusBadRequest))
		return
	}

	destination := "/"
	conflictPolicy := strings.TrimSpace(r.URL.Query().Get("conflict_policy"))
	result := model.UploadResponse{Uploaded: []model.UploadItem{}, Failed: []model.UploadFailure{}}

	for {
		part, nextErr := reader.NextPart()
		if nextErr == io.EOF {
			break
		}
		if nextErr != nil {
			if isPayloadTooLarge(nextErr) {
				writeError(w, apierror.New("PAYLOAD_TOO_LARGE", "request body exceeds MAX_UPLOAD_SIZE", "MAX_UPLOAD_SIZE", http.StatusRequestEntityTooLarge))
				return
			}
			writeError(w, apierror.New("BAD_REQUEST", "invalid multipart stream", nextErr.Error(), http.StatusBadRequest))
			return
		}

		if part.FormName() == "path" {
			pathBytes, _ := io.ReadAll(part)
			pathValue := strings.TrimSpace(string(pathBytes))
			if pathValue != "" {
				destination = pathValue
			}
			_ = part.Close()
			continue
		}

		if part.FormName() == "conflict_policy" {
			policyBytes, _ := io.ReadAll(part)
			policyValue := strings.TrimSpace(string(policyBytes))
			if policyValue != "" {
				conflictPolicy = policyValue
			}
			_ = part.Close()
			continue
		}

		if part.FormName() != "files" || strings.TrimSpace(part.FileName()) == "" {
			_ = part.Close()
			continue
		}

		uploaded, uploadErr := h.service.Upload(r.Context(), destination, part.FileName(), conflictPolicy, part)
		if uploadErr != nil {
			if isPayloadTooLarge(uploadErr) {
				writeError(w, apierror.New("PAYLOAD_TOO_LARGE", "request body exceeds MAX_UPLOAD_SIZE", "MAX_UPLOAD_SIZE", http.StatusRequestEntityTooLarge))
				_ = part.Close()
				return
			}
			result.Failed = append(result.Failed, model.UploadFailure{Name: part.FileName(), Reason: uploadErr.Error()})
			_ = part.Close()
			continue
		}

		result.Uploaded = append(result.Uploaded, uploaded)
		_ = part.Close()
	}

	writeSuccess(w, http.StatusOK, result, nil)
}

func isPayloadTooLarge(err error) bool {
	var maxBytesErr *http.MaxBytesError
	if errors.As(err, &maxBytesErr) {
		return true
	}

	return strings.Contains(strings.ToLower(err.Error()), "request body too large")
}

func (h *FileHandler) Download(w http.ResponseWriter, r *http.Request) {
	requestedPath := strings.TrimSpace(r.URL.Query().Get("path"))
	if requestedPath == "" {
		writeError(w, apierror.New("BAD_REQUEST", "query parameter 'path' is required", "path", http.StatusBadRequest))
		return
	}

	archive := strings.EqualFold(r.URL.Query().Get("archive"), "true")
	if archive {
		directory, archiveName, err := h.service.GetDirectoryForArchive(requestedPath)
		if err != nil {
			writeError(w, err)
			return
		}

		w.Header().Set("Content-Type", "application/zip")
		w.Header().Set("Content-Disposition", `attachment; filename="`+archiveName+`.zip"`)
		if err := util.StreamZipFromDirectory(directory, w); err != nil {
			writeError(w, err)
		}
		return
	}

	file, info, mimeType, err := h.service.GetFile(requestedPath)
	if err != nil {
		writeError(w, err)
		return
	}
	defer file.Close()

	filename := filepath.Base(requestedPath)
	if decoded, decodeErr := strconv.Unquote(`"` + filename + `"`); decodeErr == nil {
		filename = decoded
	}

	w.Header().Set("Content-Type", mimeType)
	w.Header().Set("Content-Disposition", mime.FormatMediaType("attachment", map[string]string{"filename": filename}))
	http.ServeContent(w, r, filename, info.ModTime(), file)
}

func (h *FileHandler) Preview(w http.ResponseWriter, r *http.Request) {
	requestedPath := strings.TrimSpace(r.URL.Query().Get("path"))
	if requestedPath == "" {
		writeError(w, apierror.New("BAD_REQUEST", "query parameter 'path' is required", "path", http.StatusBadRequest))
		return
	}

	file, info, mimeType, err := h.service.GetFile(requestedPath)
	if err != nil {
		writeError(w, err)
		return
	}
	defer file.Close()

	filename := filepath.Base(requestedPath)
	w.Header().Set("Content-Type", mimeType)
	w.Header().Set("Content-Disposition", mime.FormatMediaType("inline", map[string]string{"filename": filename}))
	http.ServeContent(w, r, filename, info.ModTime(), file)
}

func (h *FileHandler) Thumbnail(w http.ResponseWriter, r *http.Request) {
	requestedPath := strings.TrimSpace(r.URL.Query().Get("path"))
	if requestedPath == "" {
		writeError(w, apierror.New("BAD_REQUEST", "query parameter 'path' is required", "path", http.StatusBadRequest))
		return
	}

	size := parseIntOrDefault(r.URL.Query().Get("size"), 256)
	if size < 32 {
		size = 32
	}
	if size > 2048 {
		size = 2048
	}

	file, info, err := h.service.GetThumbnail(requestedPath, size)
	if err != nil {
		var apiErr *apierror.APIError
		if errors.As(err, &apiErr) && apiErr.Code == "UNSUPPORTED_TYPE" {
			w.WriteHeader(http.StatusNoContent)
			return
		}

		writeError(w, err)
		return
	}
	defer file.Close()

	filename := filepath.Base(requestedPath) + ".jpg"
	w.Header().Set("Content-Type", "image/jpeg")
	w.Header().Set("Content-Disposition", mime.FormatMediaType("inline", map[string]string{"filename": filename}))
	http.ServeContent(w, r, filename, info.ModTime(), file)
}

func (h *FileHandler) Info(w http.ResponseWriter, r *http.Request) {
	requestedPath := strings.TrimSpace(r.URL.Query().Get("path"))
	if requestedPath == "" {
		writeError(w, apierror.New("BAD_REQUEST", "query parameter 'path' is required", "path", http.StatusBadRequest))
		return
	}

	item, err := h.service.GetInfo(requestedPath)
	if err != nil {
		writeError(w, err)
		return
	}

	writeSuccess(w, http.StatusOK, item, nil)
}
