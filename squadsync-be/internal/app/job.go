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
//  1. Add a jobKind constant and register its handler in jobHandlers (the
//     normal mode) or in durableJobHandlers (the durable mode, below).
//  2. Schedule it with scheduleJob inside the transaction of the use case
//     that makes it due, and cancel it with cancelJobs where it stops being
//     due.
//
// Either way, a handler must re-check that the job still applies (the
// Esemeny's state may have changed) and do nothing otherwise. A handler
// that returns an error is rolled back and retried with backoff, up to
// maxJobAttempts attempts in all; then the job is given up and logged.
//
// # Normal mode: jobHandler
//
// The handler runs inside the job's transaction, which also marks the job
// done, so its database changes happen exactly once. Discord work goes in
// the returned after func, which runs after the commit through afterCommit.
// That work is best effort and is never retried: a failed DM is only logged,
// as for any use case. It must never call a use case that itself calls
// afterCommit (see discordWork). Use this mode for DMs and message edits.
//
// # Durable mode: durableJobHandler
//
// For an outside call that must succeed, such as creating a Spotify
// playlist (#23), whose failure should be retried:
//
//  1. The handler reads what it needs in a short transaction and returns
//     the call (nil when there is nothing to do; the job is then done).
//  2. The call runs outside any transaction, so a slow service never holds
//     SQLite's write lock. It bounds itself with its own timeout.
//  3. On success it returns a finish func, which runs in the transaction
//     that marks the job done and stores what the call returned (such as a
//     playlist id). On failure the job is retried with the usual backoff.
//
// While the call runs the job is leased for durableJobLease, so another
// runner leaves it alone. The call is at least once: if the process dies
// during it, or finish fails, it is made again, so it should be safe to
// repeat (or check first whether it already happened).

// jobKind names what a job does.
type jobKind string

// The job kinds.
const (
	jobLezajlott   jobKind = "lezajlott"
	jobEmlekezteto jobKind = "emlekezteto"
	jobHatarido    jobKind = "hatarido"
	// jobElszamolasEmlekezteto reminds the unconfirmed, 3 days after
	// Lezajlott, to confirm their Kiadasok (elszamolas.go).
	jobElszamolasEmlekezteto jobKind = "elszamolas_emlekezteto"
)

// jobHandler runs one due job in its transaction. after, if not nil, runs
// once the transaction commits (best effort, never retried).
type jobHandler func(ctx context.Context, q store.Queries, j store.Job) (after func(context.Context), err error)

// durableJobHandler reads what a durable job needs in a transaction and
// returns its outside call, nil for nothing to do.
type durableJobHandler func(ctx context.Context, q store.Queries, j store.Job) (call durableCall, err error)

// durableCall is a durable job's outside call, made outside any
// transaction. finish, if not nil, stores its results in the transaction
// that marks the job done.
type durableCall func(ctx context.Context) (finish func(ctx context.Context, q store.Queries) error, err error)

// jobHandlers is the one place normal-mode job kinds are registered.
func (a *App) jobHandlers() map[jobKind]jobHandler {
	return map[jobKind]jobHandler{
		jobLezajlott:   a.runLezajlott,
		jobEmlekezteto: a.runEmlekezteto,
		jobHatarido:    a.runHatarido,

		jobElszamolasEmlekezteto: a.runElszamolasEmlekezteto,
	}
}

// durableJobHandlers is the one place durable-mode job kinds are
// registered. #23 adds the Spotify playlist here.
func (a *App) durableJobHandlers() map[jobKind]durableJobHandler {
	handlers := map[jobKind]durableJobHandler{}
	for k, h := range a.testDurableJobs {
		handlers[k] = h
	}
	return handlers
}

const (
	// maxJobAttempts is how many times a failing job is tried in all.
	maxJobAttempts = 5
	// firstJobRetry is the wait after the first failure; each later one
	// waits jobRetryFactor times longer (1, 5, 25, 125 minutes).
	firstJobRetry  = time.Minute
	jobRetryFactor = 5
	// durableJobLease is how long a durable job's outside call may take
	// before another runner may try it again.
	durableJobLease = 10 * time.Minute
	// dueJobBatch bounds the jobs one RunDueJobs call runs; the rest run on
	// the next call.
	dueJobBatch = 500
)

// RunDueJobs runs every pending job whose time has come (by the app's
// clock), including ones missed while the server was down. It is idempotent
// and safe to call concurrently: each job runs in its own transaction that
// first checks it's still pending, so a job runs once. Once ctx is
// cancelled it stops between jobs; the job in progress finishes. It returns
// an error only when it can't read the due jobs (or ctx ended); a failing
// job is recorded and retried later.
func (a *App) RunDueJobs(ctx context.Context) error {
	now := a.clock.Now()
	ids, err := a.store.ListDueJobIDs(ctx, now, dueJobBatch)
	if err != nil {
		return err
	}
	// A job in progress isn't interrupted, so it never stops halfway.
	jobCtx := context.WithoutCancel(ctx)
	for _, id := range ids {
		if err := ctx.Err(); err != nil {
			return err
		}
		a.runJob(jobCtx, id, now)
	}
	return nil
}

// dueJob finds the job if it is still pending and due at now.
func dueJob(ctx context.Context, q store.Queries, id string, now time.Time) (store.Job, bool, error) {
	j, err := q.FindJob(ctx, id)
	if errors.Is(err, store.ErrNotFound) {
		return store.Job{}, false, nil // cancelled meanwhile
	}
	if err != nil {
		return store.Job{}, false, err
	}
	// Done by another runner, or retried or leased for later.
	return j, j.DoneAt == nil && !j.RunAt.After(now), nil
}

// runJob runs one job, if it is still pending and due.
func (a *App) runJob(ctx context.Context, id string, now time.Time) {
	var after func(context.Context)
	var durable durableCall
	err := a.store.InTx(ctx, func(q store.Queries) error {
		j, due, err := dueJob(ctx, q, id, now)
		if err != nil || !due {
			return err
		}
		if h, ok := a.durableJobHandlers()[jobKind(j.Kind)]; ok {
			if durable, err = h(ctx, q, j); err != nil {
				return err
			}
			if durable != nil {
				return q.LeaseJob(ctx, j.ID, now.Add(durableJobLease))
			}
			return q.FinishJob(ctx, j.ID, now)
		}
		h, ok := a.jobHandlers()[jobKind(j.Kind)]
		if !ok {
			return fmt.Errorf("unknown job kind %q", j.Kind)
		}
		if after, err = h(ctx, q, j); err != nil {
			return err
		}
		return q.FinishJob(ctx, j.ID, now)
	})
	if err != nil {
		a.failJob(ctx, id, now, err)
		return
	}
	if durable != nil {
		a.finishDurableJob(ctx, id, now, durable)
		return
	}
	if after != nil {
		a.afterCommit(after)
	}
}

// finishDurableJob makes a leased durable job's outside call, outside any
// transaction, then marks the job done with its results.
func (a *App) finishDurableJob(ctx context.Context, id string, now time.Time, call durableCall) {
	finish, err := call(ctx)
	if err == nil {
		err = a.store.InTx(ctx, func(q store.Queries) error {
			j, err := q.FindJob(ctx, id)
			if err != nil {
				return err
			}
			if j.DoneAt != nil {
				return nil
			}
			if finish != nil {
				if err := finish(ctx, q); err != nil {
					return err
				}
			}
			return q.FinishJob(ctx, id, now)
		})
	}
	if err != nil {
		a.failJob(ctx, id, now, err)
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
