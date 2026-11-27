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
	"log/slog"
	"sync"

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

	// WebURL is the public URL of the web app, for links in Discord
	// messages.
	WebURL string
	// Log receives failures that don't fail a use case, such as Discord
	// being down. Optional.
	Log *slog.Logger
	// AsyncDiscord sends Discord messages in the background after a use
	// case returns, instead of before. The server sets it, so a slow Discord
	// never slows down the web or the bot; tests leave it off.
	AsyncDiscord bool
}

// App runs the use cases.
type App struct {
	store    *store.Store
	clock    clock.Clock
	discord  gateway.DiscordMessenger
	calendar gateway.GoogleCalendar
	spotify  gateway.Spotify

	webURL       string
	log          *slog.Logger
	asyncDiscord bool
	// discordMu serialises Discord work on Esemenyek, so a later message
	// edit never lands before an earlier one (see esemeny_discord.go).
	discordMu sync.Mutex
	// background counts Discord work still running in the background.
	background sync.WaitGroup
}

// New builds the application core.
func New(d Deps) *App {
	a := &App{
		store:        d.Store,
		clock:        d.Clock,
		discord:      d.Discord,
		calendar:     d.Calendar,
		spotify:      d.Spotify,
		webURL:       d.WebURL,
		log:          d.Log,
		asyncDiscord: d.AsyncDiscord,
	}
	if a.log == nil {
		a.log = slog.New(slog.DiscardHandler)
	}
	return a
}
