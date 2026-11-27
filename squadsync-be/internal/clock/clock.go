// Package clock gives the application core its notion of "now", so tests can
// control time instead of sleeping.
package clock

import (
	"sync"
	"time"
)

// Clock tells the current time. Use cases read time only through a Clock.
type Clock interface {
	Now() time.Time
}

// System is the real wall clock, in UTC.
type System struct{}

// Now returns the current time in UTC.
func (System) Now() time.Time { return time.Now().UTC() }

// Fake is a Clock that only moves when a test moves it. It is safe for
// concurrent use.
type Fake struct {
	mu  sync.Mutex
	now time.Time
}

// NewFake returns a Fake clock that starts at start (converted to UTC).
func NewFake(start time.Time) *Fake {
	return &Fake{now: start.UTC()}
}

// Now returns the fake current time.
func (f *Fake) Now() time.Time {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.now
}

// Advance moves the clock forward by d.
func (f *Fake) Advance(d time.Duration) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.now = f.now.Add(d)
}

// Set moves the clock to t (converted to UTC).
func (f *Fake) Set(t time.Time) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.now = t.UTC()
}
