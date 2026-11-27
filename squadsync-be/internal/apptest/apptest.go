// Package apptest is the test harness for the application core. A test gets
// a real App wired to a fresh temporary SQLite file (built with the real
// migrations), a controllable clock and recording fakes for every outbound
// gateway:
//
//	h := apptest.New(t)
//	f, err := h.App.FirstLogin(ctx, app.FirstLoginInput{...})
//	h.Clock.Advance(24 * time.Hour)
//	dms := h.Discord.DMsTo("discord-id")
package apptest

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"squadsync-be/internal/app"
	"squadsync-be/internal/clock"
	"squadsync-be/internal/gateway/gatewayfake"
	"squadsync-be/internal/store"
)

// Start is where the fake clock starts: a fixed, ordinary moment in UTC.
var Start = time.Date(2026, time.March, 2, 9, 0, 0, 0, time.UTC)

// WebURL is the web app's public URL the core is given.
const WebURL = "https://squadsync.example"

// Harness is one isolated application core and its controllable outside world.
type Harness struct {
	App      *app.App
	Store    *store.Store
	Clock    *clock.Fake
	Discord  *gatewayfake.Discord
	Calendar *gatewayfake.GoogleCalendar
	Spotify  *gatewayfake.Spotify
	// DBPath is the temporary SQLite file, removed when the test ends.
	DBPath string
}

// New builds a Harness. Everything is cleaned up when the test ends.
func New(t testing.TB) *Harness {
	t.Helper()
	s, path := NewStore(t)
	h := &Harness{
		Store:    s,
		Clock:    clock.NewFake(Start),
		Discord:  &gatewayfake.Discord{},
		Calendar: &gatewayfake.GoogleCalendar{},
		Spotify:  &gatewayfake.Spotify{},
		DBPath:   path,
	}
	h.App = app.New(app.Deps{
		Store:    h.Store,
		Clock:    h.Clock,
		Discord:  h.Discord,
		Calendar: h.Calendar,
		Spotify:  h.Spotify,
		WebURL:   WebURL,
	})
	return h
}

// NewStore opens a fresh temporary SQLite file with the real migrations
// applied. It is closed and removed when the test ends.
func NewStore(t testing.TB) (*store.Store, string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "squadsync-test.db")
	s, err := store.Open(path)
	if err != nil {
		t.Fatalf("open test database: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })
	if err := s.Migrate(context.Background()); err != nil {
		t.Fatalf("migrate test database: %v", err)
	}
	return s, path
}
