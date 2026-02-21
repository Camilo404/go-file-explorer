package handler

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/go-chi/chi/v5"

	"go-file-explorer/internal/model"
	"go-file-explorer/internal/service"
	"go-file-explorer/pkg/apierror"
)

type JobsHandler struct {
	service *service.JobService
}

func NewJobsHandler(service *service.JobService) *JobsHandler {
	return &JobsHandler{service: service}
}

func (h *JobsHandler) CreateOperationJob(w http.ResponseWriter, r *http.Request) {
	defer r.Body.Close()

	var payload model.JobOperationRequest
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		writeError(w, apierror.New("BAD_REQUEST", "invalid JSON body", "", http.StatusBadRequest))
		return
	}

	job, err := h.service.CreateOperationJob(payload, actorFromRequest(r))
	if err != nil {
		writeError(w, err)
		return
	}

	writeSuccess(w, http.StatusAccepted, job, nil)
}

func (h *JobsHandler) GetJob(w http.ResponseWriter, r *http.Request) {
	jobID := chi.URLParam(r, "job_id")
	if jobID == "" {
		writeError(w, apierror.New("BAD_REQUEST", "job_id is required", "job_id", http.StatusBadRequest))
		return
	}

	job, err := h.service.GetJob(jobID, actorFromRequest(r))
	if err != nil {
		writeError(w, err)
		return
	}

	writeSuccess(w, http.StatusOK, job, nil)
}

func (h *JobsHandler) GetJobItems(w http.ResponseWriter, r *http.Request) {
	jobID := chi.URLParam(r, "job_id")
	if jobID == "" {
		writeError(w, apierror.New("BAD_REQUEST", "job_id is required", "job_id", http.StatusBadRequest))
		return
	}

	page := parseIntOrDefault(r.URL.Query().Get("page"), 1)
	limit := parseIntOrDefault(r.URL.Query().Get("limit"), 100)

	data, meta, err := h.service.GetJobItems(jobID, actorFromRequest(r), page, limit)
	if err != nil {
		writeError(w, err)
		return
	}

	writeSuccess(w, http.StatusOK, data, &meta)
}

func (h *JobsHandler) Stream(w http.ResponseWriter, r *http.Request) {
	jobID := chi.URLParam(r, "job_id")
	if jobID == "" {
		writeError(w, apierror.New("BAD_REQUEST", "job_id is required", "job_id", http.StatusBadRequest))
		return
	}

	flusher, ok := w.(http.Flusher)
	if !ok {
		writeError(w, apierror.New("INTERNAL_ERROR", "streaming not supported", "", http.StatusInternalServerError))
		return
	}

	ch, err := h.service.Subscribe(jobID)
	if err != nil {
		writeError(w, err)
		return
	}
	defer h.service.Unsubscribe(jobID, ch)

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)
	flusher.Flush()

	ctx := r.Context()
	for {
		select {
		case <-ctx.Done():
			return
		case update, open := <-ch:
			if !open {
				return
			}
			data, marshalErr := json.Marshal(update)
			if marshalErr != nil {
				continue
			}
			_, _ = fmt.Fprintf(w, "data: %s\n\n", data)
			flusher.Flush()
		}
	}
}
