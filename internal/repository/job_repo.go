package repository

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"go-file-explorer/internal/model"
)

type JobRepository struct {
	pool *pgxpool.Pool
}

func NewJobRepository(pool *pgxpool.Pool) *JobRepository {
	return &JobRepository{pool: pool}
}

func (r *JobRepository) Create(ctx context.Context, job model.JobData) error {
	var startedAt, finishedAt *time.Time

	if job.StartedAt != "" {
		t, err := time.Parse(time.RFC3339Nano, job.StartedAt)
		if err == nil {
			startedAt = &t
		}
	}
	if job.FinishedAt != "" {
		t, err := time.Parse(time.RFC3339Nano, job.FinishedAt)
		if err == nil {
			finishedAt = &t
		}
	}

	createdAt, _ := time.Parse(time.RFC3339Nano, job.CreatedAt)

	_, err := r.pool.Exec(ctx,
		`INSERT INTO jobs (id, operation, status, conflict_policy,
		  total_items, processed_items, success_items, failed_items, progress,
		  created_at, started_at, finished_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)`,
		job.JobID, job.Operation, job.Status, job.ConflictPolicy,
		job.TotalItems, job.ProcessedItems, job.SuccessItems, job.FailedItems, job.Progress,
		createdAt, startedAt, finishedAt)
	if err != nil {
		return fmt.Errorf("create job: %w", err)
	}
	return nil
}

func (r *JobRepository) Update(ctx context.Context, job model.JobData) error {
	var startedAt, finishedAt *time.Time

	if job.StartedAt != "" {
		t, err := time.Parse(time.RFC3339Nano, job.StartedAt)
		if err == nil {
			startedAt = &t
		}
	}
	if job.FinishedAt != "" {
		t, err := time.Parse(time.RFC3339Nano, job.FinishedAt)
		if err == nil {
			finishedAt = &t
		}
	}

	_, err := r.pool.Exec(ctx,
		`UPDATE jobs SET status = $2, processed_items = $3, success_items = $4,
		  failed_items = $5, progress = $6, started_at = $7, finished_at = $8
		 WHERE id = $1`,
		job.JobID, job.Status, job.ProcessedItems, job.SuccessItems,
		job.FailedItems, job.Progress, startedAt, finishedAt)
	if err != nil {
		return fmt.Errorf("update job: %w", err)
	}
	return nil
}

func (r *JobRepository) FindByID(ctx context.Context, jobID string) (model.JobData, error) {
	var job model.JobData
	var createdAt time.Time
	var startedAt, finishedAt *time.Time

	err := r.pool.QueryRow(ctx,
		`SELECT id, operation, status, conflict_policy,
		        total_items, processed_items, success_items, failed_items, progress,
		        created_at, started_at, finished_at
		 FROM jobs WHERE id = $1`, jobID).
		Scan(&job.JobID, &job.Operation, &job.Status, &job.ConflictPolicy,
			&job.TotalItems, &job.ProcessedItems, &job.SuccessItems,
			&job.FailedItems, &job.Progress,
			&createdAt, &startedAt, &finishedAt)

	if errors.Is(err, pgx.ErrNoRows) {
		return model.JobData{}, model.ErrJobNotFound
	}
	if err != nil {
		return model.JobData{}, fmt.Errorf("find job: %w", err)
	}

	job.CreatedAt = createdAt.Format(time.RFC3339Nano)
	if startedAt != nil {
		job.StartedAt = startedAt.Format(time.RFC3339Nano)
	}
	if finishedAt != nil {
		job.FinishedAt = finishedAt.Format(time.RFC3339Nano)
	}

	return job, nil
}

func (r *JobRepository) SaveItems(ctx context.Context, jobID string, items []model.JobItemResult) error {
	if len(items) == 0 {
		return nil
	}

	batch := &pgx.Batch{}
	for _, item := range items {
		batch.Queue(
			`INSERT INTO job_items (job_id, source, dest, path, status, reason)
			 VALUES ($1, $2, $3, $4, $5, $6)`,
			jobID, item.From, item.To, item.Path, item.Status, item.Reason)
	}

	br := r.pool.SendBatch(ctx, batch)
	defer br.Close()

	for range items {
		if _, err := br.Exec(); err != nil {
			return fmt.Errorf("save job item: %w", err)
		}
	}

	return nil
}

func (r *JobRepository) GetItems(ctx context.Context, jobID string, page int, limit int) ([]model.JobItemResult, model.Meta, error) {
	if page < 1 {
		page = 1
	}
	if limit <= 0 {
		limit = 100
	}
	if limit > 500 {
		limit = 500
	}

	var total int
	err := r.pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM job_items WHERE job_id = $1`, jobID).Scan(&total)
	if err != nil {
		return nil, model.Meta{}, fmt.Errorf("count job items: %w", err)
	}

	totalPages := 0
	if total > 0 {
		totalPages = (total + limit - 1) / limit
	}

	offset := (page - 1) * limit
	rows, err := r.pool.Query(ctx,
		`SELECT source, dest, path, status, reason
		 FROM job_items WHERE job_id = $1
		 ORDER BY id
		 LIMIT $2 OFFSET $3`, jobID, limit, offset)
	if err != nil {
		return nil, model.Meta{}, fmt.Errorf("query job items: %w", err)
	}
	defer rows.Close()

	items := make([]model.JobItemResult, 0)
	for rows.Next() {
		var item model.JobItemResult
		if err := rows.Scan(&item.From, &item.To, &item.Path, &item.Status, &item.Reason); err != nil {
			return nil, model.Meta{}, fmt.Errorf("scan job item: %w", err)
		}
		items = append(items, item)
	}

	meta := model.Meta{Page: page, Limit: limit, Total: total, TotalPages: totalPages}
	return items, meta, rows.Err()
}
