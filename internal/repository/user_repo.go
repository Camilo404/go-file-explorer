package repository

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"go-file-explorer/internal/model"
	"go-file-explorer/pkg/apierror"
)

type UserRepository struct {
	pool *pgxpool.Pool
}

func NewUserRepository(pool *pgxpool.Pool) *UserRepository {
	return &UserRepository{pool: pool}
}

func (r *UserRepository) FindByID(ctx context.Context, id string) (model.User, error) {
	var u model.User
	err := r.pool.QueryRow(ctx,
		`SELECT id, username, password_hash, role, force_password_change,
		        failed_login_attempts, locked_until, created_at, updated_at
		 FROM users WHERE id = $1`, id).
		Scan(&u.ID, &u.Username, &u.PasswordHash, &u.Role, &u.ForcePasswordChange,
			&u.FailedLoginAttempts, &u.LockedUntil, &u.CreatedAt, &u.UpdatedAt)

	if errors.Is(err, pgx.ErrNoRows) {
		return model.User{}, apierror.New("NOT_FOUND", "user not found", id, http.StatusNotFound)
	}
	if err != nil {
		return model.User{}, fmt.Errorf("find user by id: %w", err)
	}
	return u, nil
}

func (r *UserRepository) FindByUsername(ctx context.Context, username string) (model.User, error) {
	var u model.User
	err := r.pool.QueryRow(ctx,
		`SELECT id, username, password_hash, role, force_password_change,
		        failed_login_attempts, locked_until, created_at, updated_at
		 FROM users WHERE lower(username) = lower($1)`, strings.TrimSpace(username)).
		Scan(&u.ID, &u.Username, &u.PasswordHash, &u.Role, &u.ForcePasswordChange,
			&u.FailedLoginAttempts, &u.LockedUntil, &u.CreatedAt, &u.UpdatedAt)

	if errors.Is(err, pgx.ErrNoRows) {
		return model.User{}, apierror.New("NOT_FOUND", "user not found", username, http.StatusNotFound)
	}
	if err != nil {
		return model.User{}, fmt.Errorf("find user by username: %w", err)
	}
	return u, nil
}

func (r *UserRepository) ExistsByUsername(ctx context.Context, username string) (bool, error) {
	var exists bool
	err := r.pool.QueryRow(ctx,
		`SELECT EXISTS(SELECT 1 FROM users WHERE lower(username) = lower($1))`,
		strings.TrimSpace(username)).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("check username exists: %w", err)
	}
	return exists, nil
}

func (r *UserRepository) Create(ctx context.Context, u model.User) error {
	_, err := r.pool.Exec(ctx,
		`INSERT INTO users (id, username, password_hash, role, force_password_change, created_at, updated_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7)`,
		u.ID, u.Username, u.PasswordHash, u.Role, u.ForcePasswordChange, u.CreatedAt, u.UpdatedAt)
	if err != nil {
		return fmt.Errorf("create user: %w", err)
	}
	return nil
}

func (r *UserRepository) Update(ctx context.Context, u model.User) error {
	tag, err := r.pool.Exec(ctx,
		`UPDATE users SET role = $2, updated_at = $3 WHERE id = $1`,
		u.ID, u.Role, u.UpdatedAt)
	if err != nil {
		return fmt.Errorf("update user: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return apierror.New("NOT_FOUND", "user not found", u.ID, http.StatusNotFound)
	}
	return nil
}

func (r *UserRepository) UpdatePassword(ctx context.Context, userID string, passwordHash string) error {
	tag, err := r.pool.Exec(ctx,
		`UPDATE users SET password_hash = $2, force_password_change = false, updated_at = $3 WHERE id = $1`,
		userID, passwordHash, time.Now().UTC())
	if err != nil {
		return fmt.Errorf("update password: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return apierror.New("NOT_FOUND", "user not found", userID, http.StatusNotFound)
	}
	return nil
}

func (r *UserRepository) IncrementFailedAttempts(ctx context.Context, userID string) error {
	_, err := r.pool.Exec(ctx,
		`UPDATE users SET failed_login_attempts = failed_login_attempts + 1, updated_at = $2 WHERE id = $1`,
		userID, time.Now().UTC())
	if err != nil {
		return fmt.Errorf("increment failed attempts: %w", err)
	}
	return nil
}

func (r *UserRepository) LockAccount(ctx context.Context, userID string, until time.Time) error {
	_, err := r.pool.Exec(ctx,
		`UPDATE users SET locked_until = $2, updated_at = $3 WHERE id = $1`,
		userID, until, time.Now().UTC())
	if err != nil {
		return fmt.Errorf("lock account: %w", err)
	}
	return nil
}

func (r *UserRepository) ResetFailedAttempts(ctx context.Context, userID string) error {
	_, err := r.pool.Exec(ctx,
		`UPDATE users SET failed_login_attempts = 0, locked_until = NULL, updated_at = $2 WHERE id = $1`,
		userID, time.Now().UTC())
	if err != nil {
		return fmt.Errorf("reset failed attempts: %w", err)
	}
	return nil
}

func (r *UserRepository) Delete(ctx context.Context, id string) error {
	tag, err := r.pool.Exec(ctx, `DELETE FROM users WHERE id = $1`, id)
	if err != nil {
		return fmt.Errorf("delete user: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return apierror.New("NOT_FOUND", "user not found", id, http.StatusNotFound)
	}
	return nil
}

func (r *UserRepository) List(ctx context.Context) ([]model.AuthUser, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT id, username, role, force_password_change FROM users ORDER BY username`)
	if err != nil {
		return nil, fmt.Errorf("list users: %w", err)
	}
	defer rows.Close()

	users := make([]model.AuthUser, 0)
	for rows.Next() {
		var u model.AuthUser
		if err := rows.Scan(&u.ID, &u.Username, &u.Role, &u.ForcePasswordChange); err != nil {
			return nil, fmt.Errorf("scan user: %w", err)
		}
		users = append(users, u)
	}
	return users, rows.Err()
}

func (r *UserRepository) Count(ctx context.Context) (int, error) {
	var count int
	err := r.pool.QueryRow(ctx, `SELECT COUNT(*) FROM users`).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("count users: %w", err)
	}
	return count, nil
}
