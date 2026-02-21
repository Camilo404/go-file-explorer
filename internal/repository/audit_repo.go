package repository

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"go-file-explorer/internal/model"
)

type AuditRepository struct {
	pool *pgxpool.Pool
}

func NewAuditRepository(pool *pgxpool.Pool) *AuditRepository {
	return &AuditRepository{pool: pool}
}

func (r *AuditRepository) Log(ctx context.Context, entry model.AuditEntry) error {
	var beforeJSON, afterJSON []byte
	var err error

	if entry.Before != nil {
		beforeJSON, err = json.Marshal(entry.Before)
		if err != nil {
			return fmt.Errorf("marshal before data: %w", err)
		}
	}
	if entry.After != nil {
		afterJSON, err = json.Marshal(entry.After)
		if err != nil {
			return fmt.Errorf("marshal after data: %w", err)
		}
	}

	_, err = r.pool.Exec(ctx,
		`INSERT INTO audit_entries
		 (action, occurred_at, actor_user_id, actor_username, actor_role, actor_ip,
		  status, resource, before_data, after_data, error_text)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)`,
		entry.Action, entry.OccurredAt,
		entry.Actor.UserID, entry.Actor.Username, entry.Actor.Role, entry.Actor.IP,
		entry.Status, entry.Resource, beforeJSON, afterJSON, entry.Error)
	if err != nil {
		return fmt.Errorf("log audit entry: %w", err)
	}
	return nil
}

func (r *AuditRepository) Query(ctx context.Context, query model.AuditQuery) ([]model.AuditEntry, model.Meta, error) {
	if query.Page < 1 {
		query.Page = 1
	}
	if query.Limit <= 0 {
		query.Limit = 50
	}
	if query.Limit > 200 {
		query.Limit = 200
	}

	where := make([]string, 0)
	args := make([]any, 0)
	argIdx := 1

	if action := strings.TrimSpace(query.Action); action != "" {
		where = append(where, fmt.Sprintf("lower(action) = lower($%d)", argIdx))
		args = append(args, action)
		argIdx++
	}
	if actorID := strings.TrimSpace(query.ActorID); actorID != "" {
		where = append(where, fmt.Sprintf("actor_user_id = $%d", argIdx))
		args = append(args, actorID)
		argIdx++
	}
	if status := strings.TrimSpace(query.Status); status != "" {
		where = append(where, fmt.Sprintf("lower(status) = lower($%d)", argIdx))
		args = append(args, status)
		argIdx++
	}
	if path := strings.TrimSpace(query.Path); path != "" {
		where = append(where, fmt.Sprintf("lower(resource) LIKE lower($%d)", argIdx))
		args = append(args, "%"+path+"%")
		argIdx++
	}
	if from := strings.TrimSpace(query.From); from != "" {
		where = append(where, fmt.Sprintf("occurred_at >= $%d::timestamptz", argIdx))
		args = append(args, from)
		argIdx++
	}
	if to := strings.TrimSpace(query.To); to != "" {
		where = append(where, fmt.Sprintf("occurred_at <= $%d::timestamptz", argIdx))
		args = append(args, to)
		argIdx++
	}

	whereClause := ""
	if len(where) > 0 {
		whereClause = "WHERE " + strings.Join(where, " AND ")
	}

	// Count total
	countQuery := fmt.Sprintf("SELECT COUNT(*) FROM audit_entries %s", whereClause)
	var total int
	if err := r.pool.QueryRow(ctx, countQuery, args...).Scan(&total); err != nil {
		return nil, model.Meta{}, fmt.Errorf("count audit entries: %w", err)
	}

	totalPages := 0
	if total > 0 {
		totalPages = (total + query.Limit - 1) / query.Limit
	}
	meta := model.Meta{Page: query.Page, Limit: query.Limit, Total: total, TotalPages: totalPages}

	// Paginated query
	offset := (query.Page - 1) * query.Limit
	dataQuery := fmt.Sprintf(
		`SELECT action, occurred_at, actor_user_id, actor_username, actor_role, actor_ip,
		        status, resource, before_data, after_data, error_text
		 FROM audit_entries %s
		 ORDER BY occurred_at DESC
		 LIMIT $%d OFFSET $%d`, whereClause, argIdx, argIdx+1)
	args = append(args, query.Limit, offset)

	rows, err := r.pool.Query(ctx, dataQuery, args...)
	if err != nil {
		return nil, model.Meta{}, fmt.Errorf("query audit entries: %w", err)
	}
	defer rows.Close()

	entries := make([]model.AuditEntry, 0)
	for rows.Next() {
		var e model.AuditEntry
		var occurredAt time.Time
		var beforeJSON, afterJSON []byte

		if err := rows.Scan(
			&e.Action, &occurredAt,
			&e.Actor.UserID, &e.Actor.Username, &e.Actor.Role, &e.Actor.IP,
			&e.Status, &e.Resource, &beforeJSON, &afterJSON, &e.Error,
		); err != nil {
			return nil, model.Meta{}, fmt.Errorf("scan audit entry: %w", err)
		}

		e.OccurredAt = occurredAt.UTC().Format(time.RFC3339Nano)

		if len(beforeJSON) > 0 {
			var before any
			if jsonErr := json.Unmarshal(beforeJSON, &before); jsonErr == nil {
				e.Before = before
			}
		}
		if len(afterJSON) > 0 {
			var after any
			if jsonErr := json.Unmarshal(afterJSON, &after); jsonErr == nil {
				e.After = after
			}
		}

		entries = append(entries, e)
	}

	return entries, meta, rows.Err()
}
