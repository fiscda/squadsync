package store

import (
	"context"
	"embed"
	"fmt"
	"io/fs"
	"sort"
	"strconv"
	"strings"
	"time"
)

// migrationFiles are the Go-owned schema changes, applied in version order.
//
// Rules for migration files:
//   - Name them YYYYMMDDHHMM_short_description.sql, using the current UTC
//     time as the version. Timestamps keep parallel branches from picking the
//     same number; two files with the same version fail Migrate.
//   - Never edit or rename a file once it has been applied anywhere: Migrate
//     refuses a version that was recorded under a different file name.
//   - Each file runs in one transaction with foreign keys enforced. SQLite
//     can't switch foreign_keys off inside a transaction, so a migration that
//     rebuilds a table (create new, copy, drop old, rename) must start with
//     PRAGMA defer_foreign_keys = ON; the checks then run at commit.
//
//go:embed migrations/*.sql
var migrationFiles embed.FS

// Migrate applies every embedded migration not yet recorded in
// schema_migrations. It is safe to run on every start.
func (s *Store) Migrate(ctx context.Context) error {
	sub, err := fs.Sub(migrationFiles, "migrations")
	if err != nil {
		return err
	}
	return s.MigrateFS(ctx, sub)
}

type migration struct {
	version int64
	name    string
	body    string
}

// MigrateFS applies the *.sql migrations at the root of fsys that aren't yet
// recorded in schema_migrations, oldest version first. Each migration runs in
// its own transaction. Migrate uses it with the embedded files; tests can pass
// their own.
func (s *Store) MigrateFS(ctx context.Context, fsys fs.FS) error {
	if _, err := s.db.ExecContext(ctx, `
		CREATE TABLE IF NOT EXISTS schema_migrations (
			version    INTEGER PRIMARY KEY,
			name       TEXT NOT NULL,
			applied_at TEXT NOT NULL -- RFC 3339, UTC
		) STRICT`); err != nil {
		return fmt.Errorf("create schema_migrations: %w", err)
	}

	migrations, err := readMigrations(fsys)
	if err != nil {
		return err
	}
	for _, m := range migrations {
		if err := s.applyMigration(ctx, m); err != nil {
			return fmt.Errorf("migration %s: %w", m.name, err)
		}
	}
	return nil
}

func readMigrations(fsys fs.FS) ([]migration, error) {
	names, err := fs.Glob(fsys, "*.sql")
	if err != nil {
		return nil, err
	}

	byVersion := map[int64]string{}
	var migrations []migration
	for _, name := range names {
		prefix, _, ok := strings.Cut(name, "_")
		version, err := strconv.ParseInt(prefix, 10, 64)
		if !ok || err != nil {
			return nil, fmt.Errorf("migration %q: name must be YYYYMMDDHHMM_description.sql", name)
		}
		if other, dup := byVersion[version]; dup {
			return nil, fmt.Errorf("migrations %q and %q share version %d; give one a new timestamp", other, name, version)
		}
		byVersion[version] = name

		body, err := fs.ReadFile(fsys, name)
		if err != nil {
			return nil, err
		}
		migrations = append(migrations, migration{version: version, name: name, body: string(body)})
	}
	sort.Slice(migrations, func(i, j int) bool { return migrations[i].version < migrations[j].version })
	return migrations, nil
}

func (s *Store) applyMigration(ctx context.Context, m migration) error {
	tx, err := s.db.BeginTx(ctx, nil) // BEGIN IMMEDIATE: one migrator at a time
	if err != nil {
		return err
	}
	defer tx.Rollback()

	var appliedName string
	err = tx.QueryRowContext(ctx,
		`SELECT name FROM schema_migrations WHERE version = ?`, m.version,
	).Scan(&appliedName)
	switch {
	case err == nil && appliedName == m.name:
		return nil // already applied
	case err == nil:
		return fmt.Errorf("version %d applied as %s, file is %s", m.version, appliedName, m.name)
	case !isNoRows(err):
		return err
	}

	if _, err := tx.ExecContext(ctx, m.body); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx,
		`INSERT INTO schema_migrations (version, name, applied_at) VALUES (?, ?, ?)`,
		m.version, m.name, time.Now().UTC().Format(time.RFC3339),
	); err != nil {
		return err
	}
	return tx.Commit()
}
