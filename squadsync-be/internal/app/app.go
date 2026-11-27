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

// Generic errors returned by use cases. Entry points map them to responses.
// Errors specific to one area live in that area's file and wrap one of these
// when the entry points should treat them alike.
var (
	// ErrNotFound means the requested thing doesn't exist.
	ErrNotFound = errors.New("not found")
	// ErrInvalidInput means the caller sent something malformed.
	ErrInvalidInput = errors.New("invalid input")
)

// Deps are everything the application core needs from the outside.
type Deps struct {
	Store    *store.Store
	Clock    clock.Clock
	Discord  gateway.DiscordMessenger
	Calendar gateway.GoogleCalendar
	Spotify  gateway.Spotify
	// DiscordOAuth identifies Discord accounts being linked (ADR-0002).
	DiscordOAuth gateway.DiscordOAuth
	// PublicURL is the web app's public URL (BETTER_AUTH_URL), which the
	// linking flow's redirect URI is built from. Empty disables linking.
	PublicURL string
}

// App runs the use cases.
type App struct {
	store    *store.Store
	clock    clock.Clock
	discord  gateway.DiscordMessenger
	calendar gateway.GoogleCalendar
	spotify  gateway.Spotify

	discordOAuth gateway.DiscordOAuth
	publicURL    string
}

// New builds the application core.
func New(d Deps) *App {
	return &App{
		store:    d.Store,
		clock:    d.Clock,
		discord:  d.Discord,
		calendar: d.Calendar,
		spotify:  d.Spotify,

		discordOAuth: d.DiscordOAuth,
		publicURL:    d.PublicURL,
	}
}
