package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"
)

// Job is one timed job (see the job table). The application core gives
// each kind its meaning.
type Job struct {
	ID        string
	Kind      string
	EsemenyID string
	// Payload is kind-specific JSON.
	Payload string
	RunAt   time.Time
	// DoneAt is nil while the job is pending.
	DoneAt    *time.Time
	Attempts  int
	LastError string
	CreatedAt time.Time
}

// InsertJob schedules a job. A job with the same Esemeny, kind and payload,
// pending or done, makes it a no-op, so scheduling is idempotent.
func (q Queries) InsertJob(ctx context.Context, j Job) error {
	_, err := q.db.ExecContext(ctx,
		`INSERT INTO job (id, kind, esemeny_id, payload, run_at, created_at)
		 VALUES (?, ?, ?, ?, ?, ?)
		 ON CONFLICT (esemeny_id, kind, payload) DO NOTHING`,
		j.ID, j.Kind, j.EsemenyID, j.Payload, formatTime(j.RunAt), formatTime(j.CreatedAt),
	)
	if err != nil {
		return fmt.Errorf("insert job: %w", err)
	}
	return nil
}

// DeletePendingJobs cancels the Esemeny's pending jobs of the given kinds.
// Jobs that are done stay.
func (q Queries) DeletePendingJobs(ctx context.Context, esemenyID string, kinds ...string) error {
	if len(kinds) == 0 {
		return nil
	}
	args := []any{esemenyID}
	for _, k := range kinds {
		args = append(args, k)
	}
	_, err := q.db.ExecContext(ctx,
		`DELETE FROM job WHERE esemeny_id = ? AND done_at IS NULL
		 AND kind IN (?`+strings.Repeat(", ?", len(kinds)-1)+`)`,
		args...,
	)
	if err != nil {
		return fmt.Errorf("delete pending jobs: %w", err)
	}
	return nil
}

// ListDueJobIDs returns up to limit pending jobs whose run_at is at or
// before now, oldest first.
func (q Queries) ListDueJobIDs(ctx context.Context, now time.Time, limit int) ([]string, error) {
	rows, err := q.db.QueryContext(ctx,
		`SELECT id FROM job WHERE done_at IS NULL AND run_at <= ? ORDER BY run_at, id LIMIT ?`,
		formatTime(now), limit,
	)
	if err != nil {
		return nil, fmt.Errorf("list due jobs: %w", err)
	}
	defer rows.Close()
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("list due jobs: %w", err)
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

// FindJob returns the job with the given id, or ErrNotFound.
func (q Queries) FindJob(ctx context.Context, id string) (Job, error) {
	var j Job
	var runAt, createdAt string
	var doneAt sql.NullString
	err := q.db.QueryRowContext(ctx,
		`SELECT id, kind, esemeny_id, payload, run_at, done_at, attempts, last_error, created_at FROM job WHERE id = ?`, id,
	).Scan(&j.ID, &j.Kind, &j.EsemenyID, &j.Payload, &runAt, &doneAt, &j.Attempts, &j.LastError, &createdAt)
	if errors.Is(err, sql.ErrNoRows) {
		return Job{}, ErrNotFound
	}
	if err != nil {
		return Job{}, fmt.Errorf("find job: %w", err)
	}
	if j.RunAt, err = parseTime(runAt); err != nil {
		return Job{}, err
	}
	if j.DoneAt, err = parseNullTime(doneAt); err != nil {
		return Job{}, err
	}
	if j.CreatedAt, err = parseTime(createdAt); err != nil {
		return Job{}, err
	}
	return j, nil
}

// FinishJob marks a job done after an attempt that succeeded.
func (q Queries) FinishJob(ctx context.Context, id string, at time.Time) error {
	res, err := q.db.ExecContext(ctx,
		`UPDATE job SET done_at = ?, attempts = attempts + 1, last_error = '' WHERE id = ?`,
		formatTime(at), id)
	if err != nil {
		return fmt.Errorf("finish job: %w", err)
	}
	return requireAffected(res, "finish job")
}

// LeaseJob keeps other runners off a pending job until the given time, by
// moving its run_at there.
func (q Queries) LeaseJob(ctx context.Context, id string, until time.Time) error {
	res, err := q.db.ExecContext(ctx, `UPDATE job SET run_at = ? WHERE id = ? AND done_at IS NULL`, formatTime(until), id)
	if err != nil {
		return fmt.Errorf("lease job: %w", err)
	}
	return requireAffected(res, "lease job")
}

// FailJob records a failed attempt: the job runs again at retryAt, or, when
// retryAt is nil, gives up and is done.
func (q Queries) FailJob(ctx context.Context, id string, at time.Time, retryAt *time.Time, lastError string) error {
	var res sql.Result
	var err error
	if retryAt == nil {
		res, err = q.db.ExecContext(ctx,
			`UPDATE job SET done_at = ?, attempts = attempts + 1, last_error = ? WHERE id = ?`,
			formatTime(at), lastError, id)
	} else {
		res, err = q.db.ExecContext(ctx,
			`UPDATE job SET run_at = ?, attempts = attempts + 1, last_error = ? WHERE id = ?`,
			formatTime(*retryAt), lastError, id)
	}
	if err != nil {
		return fmt.Errorf("fail job: %w", err)
	}
	return requireAffected(res, "fail job")
}

// FindEmlekeztetoPercek returns the Csoport's Emlekezteto offsets in
// minutes, largest first, or ErrNotFound.
func (q Queries) FindEmlekeztetoPercek(ctx context.Context, csoportID string) ([]int, error) {
	var list string
	err := q.db.QueryRowContext(ctx, `SELECT emlekezteto_percek FROM csoport WHERE id = ?`, csoportID).Scan(&list)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("find emlekezteto percek: %w", err)
	}
	percek := []int{}
	for _, s := range strings.Split(list, ",") {
		if s == "" {
			continue
		}
		n, err := strconv.Atoi(s)
		if err != nil {
			return nil, fmt.Errorf("find emlekezteto percek: stored %q: %w", list, err)
		}
		percek = append(percek, n)
	}
	return percek, nil
}

// SetEmlekeztetoPercek stores the Csoport's Emlekezteto offsets in minutes,
// or returns ErrNotFound.
func (q Queries) SetEmlekeztetoPercek(ctx context.Context, csoportID string, percek []int) error {
	parts := make([]string, len(percek))
	for n, p := range percek {
		parts[n] = strconv.Itoa(p)
	}
	res, err := q.db.ExecContext(ctx,
		`UPDATE csoport SET emlekezteto_percek = ? WHERE id = ?`, strings.Join(parts, ","), csoportID)
	if err != nil {
		return fmt.Errorf("set emlekezteto percek: %w", err)
	}
	return requireAffected(res, "set emlekezteto percek")
}

// FindHatarido returns the Esemeny's voting Hatarido, nil for none, or
// ErrNotFound.
func (q Queries) FindHatarido(ctx context.Context, esemenyID string) (*time.Time, error) {
	var at sql.NullString
	err := q.db.QueryRowContext(ctx, `SELECT hatarido FROM esemeny WHERE id = ?`, esemenyID).Scan(&at)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("find hatarido: %w", err)
	}
	return parseNullTime(at)
}

// SetHatarido sets (or, with nil, clears) the Esemeny's voting Hatarido,
// or returns ErrNotFound.
func (q Queries) SetHatarido(ctx context.Context, esemenyID string, at *time.Time) error {
	var v any
	if at != nil {
		v = formatTime(*at)
	}
	res, err := q.db.ExecContext(ctx, `UPDATE esemeny SET hatarido = ? WHERE id = ?`, v, esemenyID)
	if err != nil {
		return fmt.Errorf("set hatarido: %w", err)
	}
	return requireAffected(res, "set hatarido")
}
