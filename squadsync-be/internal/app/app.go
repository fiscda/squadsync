// Package app is the application core: every use case of Squad Sync lives
// here as a method on App. The internal HTTP API and the Discord bot are thin
// entry points that translate their input into these calls.
//
// App is the main test seam. Tests build it with apptest.New, which wires a
// temporary SQLite file with the real migrations, a fake clock and recording
// fakes for the outbound gateways.
package app

import (
	"errors"

	"squadsync-be/internal/clock"
	"squadsync-be/internal/gateway"
	"squadsync-be/internal/store"
)

// Errors returned by use cases. Entry points map them to responses.
var (
	// ErrNotFound means the requested thing doesn't exist.
	ErrNotFound = errors.New("not found")
	// ErrInvalidInput means the caller sent something malformed.
	ErrInvalidInput = errors.New("invalid input")
	// ErrDiscordAccountInUse means the Discord account already belongs to
	// another Felhasznalo.
	ErrDiscordAccountInUse = errors.New("discord account already belongs to another felhasznalo")
)

// Deps are everything the application core needs from the outside.
type Deps struct {
	Store    *store.Store
	Clock    clock.Clock
	Discord  gateway.DiscordMessenger
	Calendar gateway.GoogleCalendar
	Spotify  gateway.Spotify
}

// App runs the use cases.
type App struct {
	store    *store.Store
	clock    clock.Clock
	discord  gateway.DiscordMessenger
	calendar gateway.GoogleCalendar
	spotify  gateway.Spotify
}

// New builds the application core.
func New(d Deps) *App {
	return &App{
		store:    d.Store,
		clock:    d.Clock,
		discord:  d.Discord,
		calendar: d.Calendar,
		spotify:  d.Spotify,
	}
}
