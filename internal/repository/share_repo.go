package repository

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"go-file-explorer/internal/model"
	"go-file-explorer/pkg/apierror"
)

type ShareRepository struct {
	pool *pgxpool.Pool
}

func NewShareRepository(pool *pgxpool.Pool) *ShareRepository {
	return &ShareRepository{pool: pool}
}

func (r *ShareRepository) Create(ctx context.Context, record model.ShareRecord) error {
	_, err := r.pool.Exec(ctx,
		`INSERT INTO shares (id, token, path, created_by, created_at, expires_at)
		 VALUES ($1, $2::uuid, $3, $4, $5, $6)`,
		record.ID, record.Token, record.Path, record.CreatedBy,
		record.CreatedAt, record.ExpiresAt)
	if err != nil {
		return fmt.Errorf("create share: %w", err)
	}
	return nil
}

func (r *ShareRepository) ListByUser(ctx context.Context, userID string) ([]model.ShareRecord, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT id, token, path, created_by, created_at, expires_at
		 FROM shares
		 WHERE created_by = $1 AND expires_at > now()
		 ORDER BY created_at DESC`, userID)
	if err != nil {
		return nil, fmt.Errorf("list shares: %w", err)
	}
	defer rows.Close()

	records := make([]model.ShareRecord, 0)
	for rows.Next() {
		var s model.ShareRecord
		var createdAt, expiresAt time.Time
		if err := rows.Scan(&s.ID, &s.Token, &s.Path, &s.CreatedBy, &createdAt, &expiresAt); err != nil {
			return nil, fmt.Errorf("scan share: %w", err)
		}
		s.CreatedAt = createdAt.Format(time.RFC3339Nano)
		s.ExpiresAt = expiresAt.Format(time.RFC3339Nano)
		records = append(records, s)
	}
	return records, rows.Err()
}

func (r *ShareRepository) Revoke(ctx context.Context, shareID string, userID string) error {
	tag, err := r.pool.Exec(ctx,
		`DELETE FROM shares WHERE id = $1 AND created_by = $2`, shareID, userID)
	if err != nil {
		return fmt.Errorf("revoke share: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return apierror.New("NOT_FOUND", "share not found", shareID, http.StatusNotFound)
	}
	return nil
}

func (r *ShareRepository) ResolveToken(ctx context.Context, token string) (model.ShareRecord, error) {
	var s model.ShareRecord
	var createdAt, expiresAt time.Time
	err := r.pool.QueryRow(ctx,
		`SELECT id, token, path, created_by, created_at, expires_at
		 FROM shares WHERE token = $1::uuid`, token).
		Scan(&s.ID, &s.Token, &s.Path, &s.CreatedBy, &createdAt, &expiresAt)

	if errors.Is(err, pgx.ErrNoRows) {
		return model.ShareRecord{}, apierror.New("NOT_FOUND", "share not found", token, http.StatusNotFound)
	}
	if err != nil {
		return model.ShareRecord{}, fmt.Errorf("resolve share token: %w", err)
	}

	s.CreatedAt = createdAt.Format(time.RFC3339Nano)
	s.ExpiresAt = expiresAt.Format(time.RFC3339Nano)

	if time.Now().UTC().After(expiresAt) {
		return model.ShareRecord{}, apierror.New("GONE", "share link has expired", token, http.StatusGone)
	}

	return s, nil
}

func (r *ShareRepository) CleanExpired(ctx context.Context) (int64, error) {
	tag, err := r.pool.Exec(ctx, `DELETE FROM shares WHERE expires_at <= now()`)
	if err != nil {
		return 0, fmt.Errorf("clean expired shares: %w", err)
	}
	return tag.RowsAffected(), nil
}
