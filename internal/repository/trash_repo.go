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

type TrashRepository struct {
	pool *pgxpool.Pool
}

func NewTrashRepository(pool *pgxpool.Pool) *TrashRepository {
	return &TrashRepository{pool: pool}
}

func (r *TrashRepository) Create(ctx context.Context, record model.TrashRecord) error {
	_, err := r.pool.Exec(ctx,
		`INSERT INTO trash_records
		 (id, original_path, trash_name, deleted_at,
		  deleted_by_user_id, deleted_by_username, deleted_by_role, deleted_by_ip)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`,
		record.ID, record.OriginalPath, record.TrashName, record.DeletedAt,
		record.DeletedBy.UserID, record.DeletedBy.Username,
		record.DeletedBy.Role, record.DeletedBy.IP)
	if err != nil {
		return fmt.Errorf("create trash record: %w", err)
	}
	return nil
}

func (r *TrashRepository) FindLatestByPath(ctx context.Context, originalPath string) (model.TrashRecord, error) {
	var rec model.TrashRecord
	var deletedAt time.Time
	err := r.pool.QueryRow(ctx,
		`SELECT id, original_path, trash_name, deleted_at,
		        deleted_by_user_id, deleted_by_username, deleted_by_role, deleted_by_ip
		 FROM trash_records
		 WHERE original_path = $1 AND restored_at IS NULL
		 ORDER BY deleted_at DESC LIMIT 1`, originalPath).
		Scan(&rec.ID, &rec.OriginalPath, &rec.TrashName, &deletedAt,
			&rec.DeletedBy.UserID, &rec.DeletedBy.Username,
			&rec.DeletedBy.Role, &rec.DeletedBy.IP)

	if errors.Is(err, pgx.ErrNoRows) {
		return model.TrashRecord{}, model.ErrTrashItemNotFound
	}
	if err != nil {
		return model.TrashRecord{}, fmt.Errorf("find trash by path: %w", err)
	}
	rec.DeletedAt = deletedAt.Format(time.RFC3339Nano)
	return rec, nil
}

func (r *TrashRepository) MarkRestored(ctx context.Context, id string, actor model.AuditActor) error {
	now := time.Now().UTC()
	tag, err := r.pool.Exec(ctx,
		`UPDATE trash_records
		 SET restored_at = $2,
		     restored_by_user_id = $3, restored_by_username = $4,
		     restored_by_role = $5, restored_by_ip = $6
		 WHERE id = $1 AND restored_at IS NULL`,
		id, now, actor.UserID, actor.Username, actor.Role, actor.IP)
	if err != nil {
		return fmt.Errorf("mark restored: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return model.ErrTrashItemNotFound
	}
	return nil
}

func (r *TrashRepository) List(ctx context.Context, includeRestored bool) ([]model.TrashRecord, error) {
	query := `SELECT id, original_path, trash_name, deleted_at,
	                 deleted_by_user_id, deleted_by_username, deleted_by_role, deleted_by_ip,
	                 restored_at, restored_by_user_id, restored_by_username,
	                 restored_by_role, restored_by_ip
	          FROM trash_records`
	if !includeRestored {
		query += ` WHERE restored_at IS NULL`
	}
	query += ` ORDER BY deleted_at DESC`

	rows, err := r.pool.Query(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("list trash: %w", err)
	}
	defer rows.Close()

	records := make([]model.TrashRecord, 0)
	for rows.Next() {
		var rec model.TrashRecord
		var deletedAt time.Time
		var restoredAt *time.Time
		var restoredByUserID, restoredByUsername, restoredByRole, restoredByIP string

		if err := rows.Scan(
			&rec.ID, &rec.OriginalPath, &rec.TrashName, &deletedAt,
			&rec.DeletedBy.UserID, &rec.DeletedBy.Username,
			&rec.DeletedBy.Role, &rec.DeletedBy.IP,
			&restoredAt, &restoredByUserID, &restoredByUsername,
			&restoredByRole, &restoredByIP,
		); err != nil {
			return nil, fmt.Errorf("scan trash record: %w", err)
		}

		rec.DeletedAt = deletedAt.Format(time.RFC3339Nano)
		if restoredAt != nil {
			rec.RestoredAt = restoredAt.Format(time.RFC3339Nano)
			rec.RestoredBy = model.AuditActor{
				UserID:   restoredByUserID,
				Username: restoredByUsername,
				Role:     restoredByRole,
				IP:       restoredByIP,
			}
		}

		records = append(records, rec)
	}
	return records, rows.Err()
}

func (r *TrashRepository) FindByID(ctx context.Context, id string) (model.TrashRecord, error) {
	var rec model.TrashRecord
	var deletedAt time.Time
	err := r.pool.QueryRow(ctx,
		`SELECT id, original_path, trash_name, deleted_at,
		        deleted_by_user_id, deleted_by_username, deleted_by_role, deleted_by_ip
		 FROM trash_records
		 WHERE id = $1 AND restored_at IS NULL`, id).
		Scan(&rec.ID, &rec.OriginalPath, &rec.TrashName, &deletedAt,
			&rec.DeletedBy.UserID, &rec.DeletedBy.Username,
			&rec.DeletedBy.Role, &rec.DeletedBy.IP)

	if errors.Is(err, pgx.ErrNoRows) {
		return model.TrashRecord{}, model.ErrTrashItemNotFound
	}
	if err != nil {
		return model.TrashRecord{}, fmt.Errorf("find trash by id: %w", err)
	}
	rec.DeletedAt = deletedAt.Format(time.RFC3339Nano)
	return rec, nil
}

func (r *TrashRepository) Delete(ctx context.Context, id string) error {
	tag, err := r.pool.Exec(ctx, `DELETE FROM trash_records WHERE id = $1`, id)
	if err != nil {
		return fmt.Errorf("delete trash record: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return model.ErrTrashItemNotFound
	}
	return nil
}

func (r *TrashRepository) DeleteAllNotRestored(ctx context.Context) ([]model.TrashRecord, error) {
	// First retrieve all non-restored records (we need trash_name for file deletion)
	records, err := r.List(ctx, false)
	if err != nil {
		return nil, err
	}

	_, err = r.pool.Exec(ctx, `DELETE FROM trash_records WHERE restored_at IS NULL`)
	if err != nil {
		return nil, fmt.Errorf("empty trash records: %w", err)
	}

	return records, nil
}
