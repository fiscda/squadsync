package app

import (
	"context"

	"squadsync-be/internal/domain"
	"squadsync-be/internal/store"
)

// VetoSwitchOff makes this App's Kiegeszito of the given kind refuse being
// switched off with err (nil allows it again), so tests can check the veto
// hook without a Kiegeszito that has one.
func (a *App) VetoSwitchOff(kind domain.KiegeszitoKind, err error) {
	k := a.kiegeszitok[kind]
	k.vetoSwitchOff = func(context.Context, *kiegeszitoTx) error { return err }
	if err == nil {
		k.vetoSwitchOff = nil
	}
	a.kiegeszitok[kind] = k
}

// SetSwitchable gives this App's Kiegeszito of the given kind its own rule
// for when it can be switched (nil restores the default).
func (a *App) SetSwitchable(kind domain.KiegeszitoKind, switchable func(domain.Esemeny) bool) {
	k := a.kiegeszitok[kind]
	k.switchable = switchable
	a.kiegeszitok[kind] = k
}

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
