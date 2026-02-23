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

type TokenRepository struct {
	pool *pgxpool.Pool
}

func NewTokenRepository(pool *pgxpool.Pool) *TokenRepository {
	return &TokenRepository{pool: pool}
}

func (r *TokenRepository) Store(ctx context.Context, token string, userID string, expiresAt time.Time) error {
	_, err := r.pool.Exec(ctx,
		`INSERT INTO refresh_tokens (token, user_id, created_at, expires_at)
		 VALUES ($1, $2, $3, $4)`,
		token, userID, time.Now().UTC(), expiresAt)
	if err != nil {
		return fmt.Errorf("store refresh token: %w", err)
	}
	return nil
}

func (r *TokenRepository) Validate(ctx context.Context, token string) (string, error) {
	var userID string
	err := r.pool.QueryRow(ctx,
		`SELECT user_id FROM refresh_tokens
		 WHERE token = $1 AND expires_at > now()`, token).Scan(&userID)

	if errors.Is(err, pgx.ErrNoRows) {
		return "", model.ErrTokenNotFound
	}
	if err != nil {
		return "", fmt.Errorf("validate refresh token: %w", err)
	}
	return userID, nil
}

func (r *TokenRepository) Revoke(ctx context.Context, token string) error {
	_, err := r.pool.Exec(ctx, `DELETE FROM refresh_tokens WHERE token = $1`, token)
	if err != nil {
		return fmt.Errorf("revoke refresh token: %w", err)
	}
	return nil
}

func (r *TokenRepository) RevokeAllForUser(ctx context.Context, userID string) error {
	_, err := r.pool.Exec(ctx, `DELETE FROM refresh_tokens WHERE user_id = $1`, userID)
	if err != nil {
		return fmt.Errorf("revoke all refresh tokens: %w", err)
	}
	return nil
}

func (r *TokenRepository) CleanExpired(ctx context.Context) (int64, error) {
	tag, err := r.pool.Exec(ctx, `DELETE FROM refresh_tokens WHERE expires_at <= now()`)
	if err != nil {
		return 0, fmt.Errorf("clean expired tokens: %w", err)
	}
	return tag.RowsAffected(), nil
}
