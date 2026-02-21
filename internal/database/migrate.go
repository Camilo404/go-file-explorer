package database

import (
	"context"
	_ "embed"
	"fmt"
	"log/slog"
)

//go:embed migrations/001_initial.up.sql
var initialMigrationSQL string

//go:embed migrations/002_security_hardening.up.sql
var securityHardeningSQL string

var requiredTables = []string{
	"users",
	"refresh_tokens",
	"audit_entries",
	"shares",
	"trash_records",
	"jobs",
	"job_items",
}

func (db *DB) EnsureSchema(ctx context.Context) error {
	if db == nil || db.Pool == nil {
		return fmt.Errorf("database pool is not initialized")
	}

	exists, err := db.hasAllRequiredTables(ctx)
	if err != nil {
		return fmt.Errorf("check existing tables: %w", err)
	}

	if !exists {
		slog.Info("database schema missing tables; applying initial migration")
		if _, err := db.Pool.Exec(ctx, initialMigrationSQL); err != nil {
			return fmt.Errorf("apply initial migration: %w", err)
		}

		exists, err = db.hasAllRequiredTables(ctx)
		if err != nil {
			return fmt.Errorf("re-check tables after migration: %w", err)
		}

		if !exists {
			return fmt.Errorf("schema initialization incomplete: required tables are still missing")
		}
	}

	// ── Incremental migrations ───────────────────────────────────
	// 002: security hardening (add columns if missing).
	if err := db.applySecurityHardening(ctx); err != nil {
		return fmt.Errorf("apply security hardening migration: %w", err)
	}

	slog.Info("database schema ensured")
	return nil
}

// applySecurityHardening runs migration 002 idempotently.
// The SQL uses IF NOT EXISTS so it is safe to re-run.
func (db *DB) applySecurityHardening(ctx context.Context) error {
	var hasColumn bool
	err := db.Pool.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM information_schema.columns
			WHERE table_schema = 'public'
			  AND table_name = 'users'
			  AND column_name = 'force_password_change'
		)
	`).Scan(&hasColumn)
	if err != nil {
		return fmt.Errorf("check force_password_change column: %w", err)
	}

	if !hasColumn {
		slog.Info("applying security hardening migration (002)")
		if _, err := db.Pool.Exec(ctx, securityHardeningSQL); err != nil {
			return fmt.Errorf("exec security hardening SQL: %w", err)
		}
		slog.Info("security hardening migration applied")
	}

	return nil
}

func (db *DB) hasAllRequiredTables(ctx context.Context) (bool, error) {
	var count int
	err := db.Pool.QueryRow(ctx, `
		SELECT COUNT(*)
		FROM information_schema.tables
		WHERE table_schema = 'public'
		  AND table_name = ANY($1)
	`, requiredTables).Scan(&count)
	if err != nil {
		return false, err
	}

	return count == len(requiredTables), nil
}
