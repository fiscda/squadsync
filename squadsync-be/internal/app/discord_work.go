package app

import (
	"context"
	"log/slog"
	"sync"
)

// discordQueueSize bounds the Discord work waiting for the background
// worker. A job that doesn't fit is dropped: every job re-reads the current
// state, so the next one about the same Esemeny catches up.
const discordQueueSize = 256

// discordWork runs Discord work after a use case commits, one job at a
// time and in order, so each job reads the latest state and message edits
// land in the order of the changes.
//
// In sync mode (tests) a job runs at once, in the caller. In async mode (the
// server) one worker goroutine reads a bounded FIFO queue. Jobs still
// queued when the process stops are lost; nothing is persisted, so a restart
// never resends them.
//
// A job must never call a use case that itself calls afterCommit: in sync
// mode it would deadlock on mu.
type discordWork struct {
	log   *slog.Logger
	async bool

	// base is every job's context. It is cancelled when Wait's deadline
	// passes, which abandons the job in flight and the ones still queued.
	base   context.Context
	cancel context.CancelFunc

	// mu serialises jobs in sync mode, and guards closed and jobs (sends
	// and close) in async mode.
	mu     sync.Mutex
	closed bool
	jobs   chan func(context.Context)
	done   chan struct{}
}

func newDiscordWork(async bool, log *slog.Logger) *discordWork {
	base, cancel := context.WithCancel(context.Background())
	w := &discordWork{log: log, async: async, base: base, cancel: cancel}
	if async {
		w.jobs = make(chan func(context.Context), discordQueueSize)
		w.done = make(chan struct{})
		go w.run()
	}
	return w
}

// add runs the job, or queues it for the worker in async mode.
func (w *discordWork) add(job func(context.Context)) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if !w.async {
		job(w.base)
		return
	}
	if w.closed {
		w.log.Warn("discord work after shutdown dropped")
		return
	}
	select {
	case w.jobs <- job:
	default:
		w.log.Warn("discord work queue full; job dropped", "size", discordQueueSize)
	}
}

func (w *discordWork) run() {
	defer close(w.done)
	abandoned := 0
	for job := range w.jobs {
		if w.base.Err() != nil {
			abandoned++
			continue
		}
		job(w.base)
	}
	if abandoned > 0 {
		w.log.Warn("discord work abandoned at shutdown", "jobs", abandoned)
	}
}

// wait stops accepting jobs and waits for the queued ones until ctx ends;
// then it cancels the rest, which are logged as abandoned.
func (w *discordWork) wait(ctx context.Context) {
	if !w.async {
		return
	}
	w.mu.Lock()
	if !w.closed {
		w.closed = true
		close(w.jobs)
	}
	w.mu.Unlock()

	select {
	case <-w.done:
	case <-ctx.Done():
		w.log.Warn("discord work didn't finish before the shutdown deadline; abandoning it")
		w.cancel()
		<-w.done
	}
	w.cancel()
}

// afterCommit runs Discord work once a use case has committed (see
// discordWork). The job gets a context without a deadline; it bounds each
// Discord call itself with discordCallTimeout.
func (a *App) afterCommit(job func(ctx context.Context)) {
	a.work.add(job)
}

// WaitDiscord stops taking Discord work and waits for what is queued, until
// ctx ends; then it abandons the rest and logs it. The server calls it while
// shutting down. The App must not be used afterwards.
func (a *App) WaitDiscord(ctx context.Context) {
	a.work.wait(ctx)
}
