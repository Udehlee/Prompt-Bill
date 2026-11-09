package database

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/Udehlee/Prompt-Bill/migrations"
	"github.com/pressly/goose/v3"
)

// Migrate applies all pending SQL migrations to the database
func Migrate(ctx context.Context, db *sql.DB) error {
	goose.SetBaseFS(migrations.FS)
	if err := goose.SetDialect("postgres"); err != nil {
		return fmt.Errorf("failed to set goose dialect: %w", err)
	}

	if err := goose.UpContext(ctx, db, "."); err != nil {
		return fmt.Errorf("failed to run migrations up: %w", err)
	}

	return nil
}

// Rollback rolls back the most recently applied migration
func Rollback(ctx context.Context, db *sql.DB) error {
	goose.SetBaseFS(migrations.FS)
	if err := goose.SetDialect("postgres"); err != nil {
		return fmt.Errorf("failed to set goose dialect: %w", err)
	}

	if err := goose.DownContext(ctx, db, "."); err != nil {
		return fmt.Errorf("failed to rollback migration: %w", err)
	}

	return nil
}
