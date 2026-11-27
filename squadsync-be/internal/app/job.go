package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"squadsync-be/internal/store"
)

// Timed jobs are rows in the job table: work due at a moment, such as the
// Lezajlott transition (lezajlott.go), an Emlekezteto (emlekezteto.go) or
// the Hatarido DM (hatarido.go). The server calls RunDueJobs once a minute;
// tests advance the fake clock and call it themselves.
//
// # Adding a job kind
//
//  1. Add a jobKind constant and register its handler in jobHandlers.
//  2. Schedule it with scheduleJob inside the transaction of the use case
//     that makes it due, and cancel it with cancelJobs where it stops being
//     due.
//
// A handler runs inside the job's transaction, which also marks the job
// done, so a job's database changes happen exactly once. It must re-check
// that the job still applies (the Esemeny's state may have changed) and do
// nothing otherwise. Discord work goes in the returned func, which runs after
// the commit through afterCommit, best effort, like any use case's; it must
// never call a use case that itself calls afterCommit (see discordWork).
//
// A handler that returns an error is rolled back and retried with backoff,
// up to maxJobAttempts attempts in all; then the job is given up and logged.

// jobKind names what a job does.
type jobKind string

// The job kinds.
const (
	jobLezajlott   jobKind = "lezajlott"
	jobEmlekezteto jobKind = "emlekezteto"
	jobHatarido    jobKind = "hatarido"
)

// jobHandler runs one due job in its transaction. after, if not nil, runs
// once the transaction commits.
type jobHandler func(ctx context.Context, q store.Queries, j store.Job) (after func(context.Context), err error)

// jobHandlers is the one place job kinds are registered.
func (a *App) jobHandlers() map[jobKind]jobHandler {
	return map[jobKind]jobHandler{
		jobLezajlott:   a.runLezajlott,
		jobEmlekezteto: a.runEmlekezteto,
		jobHatarido:    a.runHatarido,
	}
}

const (
	// maxJobAttempts is how many times a failing job is tried in all.
	maxJobAttempts = 5
	// firstJobRetry is the wait after the first failure; each later one
	// waits jobRetryFactor times longer (1, 5, 25, 125 minutes).
	firstJobRetry  = time.Minute
	jobRetryFactor = 5
	// dueJobBatch bounds the jobs one RunDueJobs call runs; the rest run on
	// the next call.
	dueJobBatch = 500
)

// RunDueJobs runs every pending job whose time has come (by the app's
// clock), including ones missed while the server was down. It is idempotent
// and safe to call concurrently: each job runs in its own transaction that
// first checks it's still pending, so a job runs once. It returns an error
// only when it can't read the due jobs; a failing job is recorded and
// retried later.
func (a *App) RunDueJobs(ctx context.Context) error {
	now := a.clock.Now()
	ids, err := a.store.ListDueJobIDs(ctx, now, dueJobBatch)
	if err != nil {
		return err
	}
	for _, id := range ids {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		a.runJob(ctx, id, now)
	}
	return nil
}

// runJob runs one job, if it is still pending and due.
func (a *App) runJob(ctx context.Context, id string, now time.Time) {
	var after func(context.Context)
	var kind jobKind
	err := a.store.InTx(ctx, func(q store.Queries) error {
		j, err := q.FindJob(ctx, id)
		if errors.Is(err, store.ErrNotFound) {
			return nil // cancelled meanwhile
		}
		if err != nil {
			return err
		}
		if j.DoneAt != nil || j.RunAt.After(now) {
			return nil // another runner got it first, or it was retried
		}
		kind = jobKind(j.Kind)
		h, ok := a.jobHandlers()[kind]
		if !ok {
			return fmt.Errorf("unknown job kind %q", j.Kind)
		}
		if after, err = h(ctx, q, j); err != nil {
			return err
		}
		return q.FinishJob(ctx, j.ID, now)
	})
	if err != nil {
		after = nil
		a.failJob(ctx, id, now, err)
		return
	}
	if after != nil {
		a.afterCommit(after)
	}
}

// failJob records a failed attempt and schedules the next one, or gives up.
func (a *App) failJob(ctx context.Context, id string, now time.Time, cause error) {
	err := a.store.InTx(ctx, func(q store.Queries) error {
		j, err := q.FindJob(ctx, id)
		if err != nil || j.DoneAt != nil {
			return err
		}
		attempts := j.Attempts + 1
		if attempts >= maxJobAttempts {
			a.log.Error("timed job failed; giving up", "job", id, "kind", j.Kind, "esemeny", j.EsemenyID, "attempts", attempts, "err", cause)
			return q.FailJob(ctx, id, now, nil, cause.Error())
		}
		wait := firstJobRetry
		for range attempts - 1 {
			wait *= jobRetryFactor
		}
		retryAt := now.Add(wait)
		a.log.Warn("timed job failed; will retry", "job", id, "kind", j.Kind, "esemeny", j.EsemenyID, "attempts", attempts, "retry_at", retryAt, "err", cause)
		return q.FailJob(ctx, id, now, &retryAt, cause.Error())
	})
	if err != nil {
		a.log.Error("record timed job failure", "job", id, "err", err, "cause", cause)
	}
}

// scheduleJob schedules a job of the Esemeny at runAt (in the caller's
// transaction). The payload is marshalled to JSON and, with the kind and the
// Esemeny, identifies the job: scheduling one that exists, pending or done,
// changes nothing, so a job never runs twice.
func (a *App) scheduleJob(ctx context.Context, q store.Queries, kind jobKind, esemenyID string, runAt time.Time, payload any) error {
	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal %s job payload: %w", kind, err)
	}
	return q.InsertJob(ctx, store.Job{
		ID:        randomID(),
		Kind:      string(kind),
		EsemenyID: esemenyID,
		Payload:   string(body),
		RunAt:     runAt,
		CreatedAt: a.clock.Now(),
	})
}

// cancelJobs cancels the Esemeny's pending jobs of the kinds (in the
// caller's transaction).
func cancelJobs(ctx context.Context, q store.Queries, esemenyID string, kinds ...jobKind) error {
	names := make([]string, len(kinds))
	for n, k := range kinds {
		names[n] = string(k)
	}
	return q.DeletePendingJobs(ctx, esemenyID, names...)
}

// jobPayload reads a job's payload into v.
func jobPayload(j store.Job, v any) error {
	if err := json.Unmarshal([]byte(j.Payload), v); err != nil {
		return fmt.Errorf("%s job %s: bad payload %q: %w", j.Kind, j.ID, j.Payload, err)
	}
	return nil
}
