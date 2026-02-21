package database

import (
	"context"
	_ "embed"
	"fmt"
	"log/slog"
)

//go:embed migrations/001_initial.up.sql
var initialMigrationSQL string

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

	if exists {
		slog.Info("database schema already exists")
		return nil
	}

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

	slog.Info("database schema ensured")
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
