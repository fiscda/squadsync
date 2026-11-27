package app

import (
	"context"

	"squadsync-be/internal/store"
)

// Test seams for package app_test. They exist only in test builds.

// TestDurableCall is a durable job's outside call as tests write it: finish
// receives the store transaction that marks the job done.
type TestDurableCall func(ctx context.Context) (finish func(ctx context.Context, q store.Queries) error, err error)

// RegisterTestDurableJob registers a durable job kind whose handler returns
// call for every job of that kind (see "Durable mode" in job.go).
func (a *App) RegisterTestDurableJob(kind string, call TestDurableCall) {
	if a.testDurableJobs == nil {
		a.testDurableJobs = map[jobKind]durableJobHandler{}
	}
	a.testDurableJobs[jobKind(kind)] = func(context.Context, store.Queries, store.Job) (durableCall, error) {
		return durableCall(call), nil
	}
}
