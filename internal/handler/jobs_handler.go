package handler

import (
	"encoding/json"
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
