// Package store is the SQLite storage of the Go-owned domain tables.
//
// The SQLite file is shared with Next.js (Better Auth). Go never reads or
// writes Better Auth's tables (user, session, account, verification), and
// migrates only its own tables (ADR-0004).
package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/url"
	"time"

	_ "modernc.org/sqlite" // pure-Go SQLite driver, registered as "sqlite"
)

// ErrNotFound is returned when a looked-up row doesn't exist.
var ErrNotFound = errors.New("store: not found")

// busyTimeout is how long a connection waits for another process (Next.js)
// to release its lock before giving up with SQLITE_BUSY.
const busyTimeout = 5 * time.Second

// timeFormat is how times are stored: RFC 3339 in UTC.
const timeFormat = time.RFC3339Nano

// Store gives access to the Go-owned tables. Its embedded Queries run outside
// a transaction; use InTx to group writes.
type Store struct {
	Queries
	db *sql.DB
}

// Open opens (creating if needed) the SQLite file at path, in WAL mode with a
// busy timeout so Go and Next.js can share it. Write transactions start with
// BEGIN IMMEDIATE so two writers queue up instead of failing mid-transaction.
// Open does not migrate; call Migrate.
func Open(path string) (*Store, error) {
	q := url.Values{}
	q.Add("_pragma", "journal_mode(WAL)")
	q.Add("_pragma", fmt.Sprintf("busy_timeout(%d)", busyTimeout.Milliseconds()))
	q.Add("_pragma", "foreign_keys(ON)")
	q.Add("_pragma", "synchronous(NORMAL)")
	q.Set("_txlock", "immediate")
	dsn := "file:" + path + "?" + q.Encode()

	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("open sqlite %q: %w", path, err)
	}
	if err := db.Ping(); err != nil {
		db.Close()
		return nil, fmt.Errorf("open sqlite %q: %w", path, err)
	}
	return &Store{Queries: Queries{db: db}, db: db}, nil
}

// Close closes the database.
func (s *Store) Close() error { return s.db.Close() }

// InTx runs fn in one transaction. It commits if fn returns nil and rolls
// back otherwise.
func (s *Store) InTx(ctx context.Context, fn func(q Queries) error) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}
	if err := fn(Queries{db: tx}); err != nil {
		_ = tx.Rollback()
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit transaction: %w", err)
	}
	return nil
}

// dbtx is what both *sql.DB and *sql.Tx provide.
type dbtx interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

// Queries are the reads and writes on the Go-owned tables. Each method runs
// either directly on the database or inside the transaction given by InTx.
type Queries struct {
	db dbtx
}

func isNoRows(err error) bool { return errors.Is(err, sql.ErrNoRows) }

func formatTime(t time.Time) string { return t.UTC().Format(timeFormat) }

func parseTime(s string) (time.Time, error) {
	t, err := time.Parse(timeFormat, s)
	if err != nil {
		return time.Time{}, fmt.Errorf("parse stored time %q: %w", s, err)
	}
	return t.UTC(), nil
}
