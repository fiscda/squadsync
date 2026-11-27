package store

import (
	"context"
	"embed"
	"fmt"
	"io/fs"
	"sort"
	"strconv"
	"strings"
)

// migrationFiles are the Go-owned schema changes, applied in file-name order.
// Name new files NNNN_short_description.sql and never edit an applied one.
//
//go:embed migrations/*.sql
var migrationFiles embed.FS

// Migrate applies every migration not yet recorded in schema_migrations. Each
// migration runs in its own transaction. It is safe to run on every start.
func (s *Store) Migrate(ctx context.Context) error {
	if _, err := s.db.ExecContext(ctx, `
		CREATE TABLE IF NOT EXISTS schema_migrations (
			version    INTEGER PRIMARY KEY,
			name       TEXT NOT NULL,
			applied_at TEXT NOT NULL
		) STRICT`); err != nil {
		return fmt.Errorf("create schema_migrations: %w", err)
	}

	names, err := fs.Glob(migrationFiles, "migrations/*.sql")
	if err != nil {
		return err
	}
	sort.Strings(names)

	for _, name := range names {
		base := strings.TrimPrefix(name, "migrations/")
		prefix, _, ok := strings.Cut(base, "_")
		version, err := strconv.Atoi(prefix)
		if !ok || err != nil {
			return fmt.Errorf("migration %q: name must start with a number and an underscore", base)
		}
		body, err := migrationFiles.ReadFile(name)
		if err != nil {
			return err
		}
		if err := s.applyMigration(ctx, version, base, string(body)); err != nil {
			return fmt.Errorf("migration %s: %w", base, err)
		}
	}
	return nil
}

func (s *Store) applyMigration(ctx context.Context, version int, name, body string) error {
	tx, err := s.db.BeginTx(ctx, nil) // BEGIN IMMEDIATE: one migrator at a time
	if err != nil {
		return err
	}
	defer tx.Rollback()

	var applied int
	if err := tx.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM schema_migrations WHERE version = ?`, version,
	).Scan(&applied); err != nil {
		return err
	}
	if applied > 0 {
		return nil
	}
	if _, err := tx.ExecContext(ctx, body); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx,
		`INSERT INTO schema_migrations (version, name, applied_at) VALUES (?, ?, datetime('now'))`,
		version, name,
	); err != nil {
		return err
	}
	return tx.Commit()
}
