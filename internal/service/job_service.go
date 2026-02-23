package service

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"

	"go-file-explorer/internal/event"
	"go-file-explorer/internal/model"
	"go-file-explorer/internal/repository"
	"go-file-explorer/pkg/apierror"
)

type queuedOperationJob struct {
	jobID string
}

type JobUpdate struct {
	JobID          string `json:"job_id"`
	Status         string `json:"status"`
	Progress       int    `json:"progress"`
	ProcessedItems int    `json:"processed_items"`
	TotalItems     int    `json:"total_items"`
	SuccessItems   int    `json:"success_items"`
	FailedItems    int    `json:"failed_items"`
}

type JobService struct {
	operations  *OperationsService
	mu          sync.RWMutex
	jobs        map[string]*model.JobData
	requests    map[string]model.JobOperationRequest
	queue       chan queuedOperationJob
	subsMu      sync.RWMutex
	subscribers map[string][]chan JobUpdate

	jobRepo *repository.JobRepository
	bus     event.Bus
}

func NewJobService(operations *OperationsService, jobRepo *repository.JobRepository, bus event.Bus) *JobService {
	s := &JobService{
		operations:  operations,
		jobs:        map[string]*model.JobData{},
		requests:    map[string]model.JobOperationRequest{},
		queue:       make(chan queuedOperationJob, 256),
		subscribers: map[string][]chan JobUpdate{},
		jobRepo:     jobRepo,
		bus:         bus,
	}

	go s.workerLoop()
	return s
}

func (s *JobService) CreateOperationJob(request model.JobOperationRequest, actor model.AuditActor) (model.JobData, error) {
	_ = actor
	operation := strings.ToLower(strings.TrimSpace(request.Operation))
	if operation != "copy" && operation != "move" && operation != "delete" && operation != "compress" && operation != "decompress" {
		return model.JobData{}, fmt.Errorf("%w: operation must be one of: copy|move|delete|compress|decompress", model.ErrInvalidInput)
	}

	total := len(request.Sources)
	if operation == "delete" || operation == "restore" {
		total = len(request.Paths)
	}
	if total == 0 {
		return model.JobData{}, fmt.Errorf("%w: job requires at least one source/path", model.ErrInvalidInput)
	}

	if operation == "copy" || operation == "move" || operation == "compress" || operation == "decompress" {
		if strings.TrimSpace(request.Destination) == "" {
			return model.JobData{}, fmt.Errorf("%w: destination is required for copy/move/compress/decompress", model.ErrInvalidInput)
		}
	}

	policy := strings.TrimSpace(request.ConflictPolicy)
	if operation == "copy" || operation == "move" || operation == "decompress" {
		normalized, err := normalizeConflictPolicy(policy)
		if err != nil {
			return model.JobData{}, err
		}
		policy = normalized
	}

	now := time.Now().UTC().Format(time.RFC3339Nano)
	job := &model.JobData{
		JobID:          uuid.NewString(),
		Operation:      operation,
		Status:         "queued",
		ConflictPolicy: policy,
		TotalItems:     total,
		ProcessedItems: 0,
		SuccessItems:   0,
		FailedItems:    0,
		Progress:       0,
		CreatedAt:      now,
	}

	s.mu.Lock()
	s.jobs[job.JobID] = job
	s.requests[job.JobID] = request
	s.mu.Unlock()

	s.persistJobCreate(job)

	s.queue <- queuedOperationJob{jobID: job.JobID}

	return cloneJob(job, false), nil
}

func (s *JobService) GetJob(jobID string, actor model.AuditActor) (model.JobData, error) {
	job, err := s.getAuthorizedJob(jobID, actor)
	if err != nil {
		return model.JobData{}, err
	}

	return cloneJob(job, false), nil
}

func (s *JobService) GetJobItems(jobID string, actor model.AuditActor, page int, limit int) (model.JobItemsData, model.Meta, error) {
	job, err := s.getAuthorizedJob(jobID, actor)
	if err != nil {
		return model.JobItemsData{}, model.Meta{}, err
	}

	if page < 1 {
		page = 1
	}
	if limit <= 0 {
		limit = 100
	}
	if limit > 500 {
		limit = 500
	}

	items := job.Items
	total := len(items)
	start := (page - 1) * limit
	if start > total {
		start = total
	}
	end := start + limit
	if end > total {
		end = total
	}

	totalPages := 0
	if total > 0 {
		totalPages = (total + limit - 1) / limit
	}

	meta := model.Meta{Page: page, Limit: limit, Total: total, TotalPages: totalPages}
	data := model.JobItemsData{JobID: jobID, Items: append([]model.JobItemResult(nil), items[start:end]...)}
	return data, meta, nil
}

func (s *JobService) workerLoop() {
	for next := range s.queue {
		s.process(next.jobID)
	}
}

func (s *JobService) process(jobID string) {
	s.mu.Lock()
	job, exists := s.jobs[jobID]
	if !exists {
		s.mu.Unlock()
		return
	}
	job.Status = "running"
	job.Progress = 5
	job.StartedAt = time.Now().UTC().Format(time.RFC3339Nano)
	s.mu.Unlock()

	s.persistJobUpdate(job)

	s.notifySubscribers(jobID, JobUpdate{
		JobID:          jobID,
		Status:         "running",
		Progress:       5,
		ProcessedItems: 0,
		TotalItems:     job.TotalItems,
		SuccessItems:   0,
		FailedItems:    0,
	})

	ctx := context.Background()
	items := make([]model.JobItemResult, 0, job.TotalItems)

	switch job.Operation {
	case "copy":
		request := s.lookupRequest(jobID)
		result, err := s.operations.Copy(ctx, request.Sources, request.Destination, request.ConflictPolicy, model.AuditActor{})
		if err != nil {
			items = append(items, model.JobItemResult{Status: "failed", Reason: err.Error()})
		}
		for _, copied := range result.Copied {
			items = append(items, model.JobItemResult{From: copied.From, To: copied.To, Status: "success"})
		}
		for _, failed := range result.Failed {
			status := "failed"
			if strings.Contains(strings.ToLower(failed.Reason), "skipped") {
				status = "skipped"
			}
			items = append(items, model.JobItemResult{From: failed.From, Status: status, Reason: failed.Reason})
		}
	case "move":
		request := s.lookupRequest(jobID)
		result, err := s.operations.Move(ctx, request.Sources, request.Destination, request.ConflictPolicy, model.AuditActor{})
		if err != nil {
			items = append(items, model.JobItemResult{Status: "failed", Reason: err.Error()})
		}
		for _, moved := range result.Moved {
			items = append(items, model.JobItemResult{From: moved.From, To: moved.To, Status: "success"})
		}
		for _, failed := range result.Failed {
			status := "failed"
			if strings.Contains(strings.ToLower(failed.Reason), "skipped") {
				status = "skipped"
			}
			items = append(items, model.JobItemResult{From: failed.From, Status: status, Reason: failed.Reason})
		}
	case "delete":
		request := s.lookupRequest(jobID)
		result, err := s.operations.Delete(ctx, request.Paths, model.AuditActor{})
		if err != nil {
			items = append(items, model.JobItemResult{Status: "failed", Reason: err.Error()})
		}
		for _, deleted := range result.Deleted {
			items = append(items, model.JobItemResult{Path: deleted, Status: "success"})
		}
		for _, failed := range result.Failed {
			items = append(items, model.JobItemResult{Path: failed.Path, Status: "failed", Reason: failed.Reason})
		}
	case "compress":
		request := s.lookupRequest(jobID)
		// Compress treats sources as inputs
		result, err := s.operations.Compress(ctx, request.Sources, request.Destination, request.Name, model.AuditActor{})
		if err != nil {
			items = append(items, model.JobItemResult{Status: "failed", Reason: err.Error()})
		} else {
			// One item representing the zip file
			items = append(items, model.JobItemResult{Path: result.Path, Status: "success"})
		}
	case "decompress":
		request := s.lookupRequest(jobID)
		// Decompress treats sources[0] as the zip file
		if len(request.Sources) == 0 {
			items = append(items, model.JobItemResult{Status: "failed", Reason: "no source file provided"})
		} else {
			result, err := s.operations.Decompress(ctx, request.Sources[0], request.Destination, request.ConflictPolicy, model.AuditActor{})
			if err != nil {
				items = append(items, model.JobItemResult{Status: "failed", Reason: err.Error()})
			} else {
				for _, file := range result.Files {
					items = append(items, model.JobItemResult{Path: file, Status: "success"})
				}
			}
		}
	}

	s.finalize(jobID, items)
}

func (s *JobService) lookupRequest(jobID string) model.JobOperationRequest {
	s.mu.RLock()
	defer s.mu.RUnlock()
	request, exists := s.requests[jobID]
	if !exists {
		return model.JobOperationRequest{}
	}
	return request
}

func (s *JobService) finalize(jobID string, items []model.JobItemResult) {
	s.mu.Lock()
	defer s.mu.Unlock()

	job, exists := s.jobs[jobID]
	if !exists {
		return
	}
	delete(s.requests, jobID)

	job.Items = items
	job.ProcessedItems = len(items)
	job.SuccessItems = 0
	job.FailedItems = 0

	for _, item := range items {
		if item.Status == "success" {
			job.SuccessItems++
			continue
		}
		job.FailedItems++
	}

	if job.TotalItems <= 0 {
		job.TotalItems = len(items)
	}
	job.Progress = 100
	job.FinishedAt = time.Now().UTC().Format(time.RFC3339Nano)

	switch {
	case job.SuccessItems == 0 && job.FailedItems > 0:
		job.Status = "failed"
	case job.SuccessItems > 0 && job.FailedItems > 0:
		job.Status = "partial"
	default:
		job.Status = "completed"
	}

	s.notifySubscribers(jobID, JobUpdate{
		JobID:          jobID,
		Status:         job.Status,
		Progress:       job.Progress,
		ProcessedItems: job.ProcessedItems,
		TotalItems:     job.TotalItems,
		SuccessItems:   job.SuccessItems,
		FailedItems:    job.FailedItems,
	})

	s.persistJobUpdate(job)
	s.persistJobItems(jobID, items)

	// Close all subscriber channels for this job
	s.subsMu.Lock()
	for _, ch := range s.subscribers[jobID] {
		close(ch)
	}
	delete(s.subscribers, jobID)
	s.subsMu.Unlock()
}

func (s *JobService) getAuthorizedJob(jobID string, actor model.AuditActor) (*model.JobData, error) {
	s.mu.RLock()
	job, exists := s.jobs[jobID]
	s.mu.RUnlock()
	if !exists {
		return nil, apierror.New("NOT_FOUND", "job not found", jobID, http.StatusNotFound)
	}

	_ = actor
	return job, nil
}

func (s *JobService) Subscribe(jobID string) (chan JobUpdate, error) {
	s.mu.RLock()
	_, exists := s.jobs[jobID]
	s.mu.RUnlock()
	if !exists {
		return nil, apierror.New("NOT_FOUND", "job not found", jobID, http.StatusNotFound)
	}

	ch := make(chan JobUpdate, 64)

	s.subsMu.Lock()
	s.subscribers[jobID] = append(s.subscribers[jobID], ch)
	s.subsMu.Unlock()

	return ch, nil
}

func (s *JobService) Unsubscribe(jobID string, ch chan JobUpdate) {
	s.subsMu.Lock()
	defer s.subsMu.Unlock()

	subs := s.subscribers[jobID]
	for i, sub := range subs {
		if sub == ch {
			s.subscribers[jobID] = append(subs[:i], subs[i+1:]...)
			return
		}
	}
}

func (s *JobService) notifySubscribers(jobID string, update JobUpdate) {
	s.subsMu.RLock()
	subs := s.subscribers[jobID]
	s.subsMu.RUnlock()

	for _, ch := range subs {
		select {
		case ch <- update:
		default:
		}
	}

	if s.bus != nil {
		eventType := event.TypeJobProgress
		if update.Status == "running" && update.Progress <= 5 {
			eventType = event.TypeJobStarted
		} else if update.Status == "completed" {
			eventType = event.TypeJobCompleted
		} else if update.Status == "failed" {
			eventType = event.TypeJobFailed
		}

		s.bus.Publish(event.Event{
			ID:        uuid.NewString(),
			Type:      eventType,
			Payload:   update,
			Timestamp: time.Now().UTC().Format(time.RFC3339Nano),
		})
	}
}

func cloneJob(value *model.JobData, includeItems bool) model.JobData {
	cloned := *value
	if includeItems {
		cloned.Items = append([]model.JobItemResult(nil), value.Items...)
		return cloned
	}
	cloned.Items = nil
	return cloned
}

// persistJobCreate saves a newly created job to the DB.
func (s *JobService) persistJobCreate(job *model.JobData) {
	ctx := context.Background()
	_ = s.jobRepo.Create(ctx, *job)
}

// persistJobUpdate saves job state changes to the DB.
func (s *JobService) persistJobUpdate(job *model.JobData) {
	ctx := context.Background()
	_ = s.jobRepo.Update(ctx, *job)
}

// persistJobItems saves completed job items to the DB.
func (s *JobService) persistJobItems(jobID string, items []model.JobItemResult) {
	ctx := context.Background()
	_ = s.jobRepo.SaveItems(ctx, jobID, items)
}
