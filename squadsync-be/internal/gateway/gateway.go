// Package gateway declares the outbound ports of the application core: the
// outside services it talks to. The core depends only on these interfaces.
// Tests use the recording fakes in gateway/gatewayfake; real implementations
// arrive with the tickets that need them.
//
// The interfaces are deliberately minimal. Grow them as tickets need more.
package gateway

import (
	"context"
	"errors"
	"time"
)

// ErrNotConfigured is returned by the Disabled gateways.
var ErrNotConfigured = errors.New("gateway: not configured")

// DiscordMessenger sends messages through the Discord bot.
type DiscordMessenger interface {
	// SendDM sends a direct message to one Discord account.
	SendDM(ctx context.Context, discordID string, content string) error
}

// CalendarEvent is one event in a Felhasznalo's dedicated "Squad Sync"
// Google calendar (ADR-0005).
type CalendarEvent struct {
	// Key identifies the event on our side, so it can be updated or removed.
	Key   string
	Title string
	Start time.Time
	End   time.Time
}

// GoogleCalendar writes to Felhasznalok's dedicated Google calendars.
type GoogleCalendar interface {
	// UpsertEvent creates or updates an event in the Felhasznalo's calendar.
	UpsertEvent(ctx context.Context, felhasznaloID string, event CalendarEvent) error
}

// Playlist is a Spotify playlist on the owner account (ADR-0006).
type Playlist struct {
	ID  string
	URL string
}

// Spotify manages playlists on the single owner account (ADR-0006).
type Spotify interface {
	// CreatePlaylist creates a public playlist with the given name.
	CreatePlaylist(ctx context.Context, name string) (Playlist, error)
}

// Disabled implements every gateway and fails with ErrNotConfigured. The
// server uses it until a real implementation exists.
type Disabled struct{}

// SendDM implements DiscordMessenger.
func (Disabled) SendDM(context.Context, string, string) error { return ErrNotConfigured }

// UpsertEvent implements GoogleCalendar.
func (Disabled) UpsertEvent(context.Context, string, CalendarEvent) error {
	return ErrNotConfigured
}

// CreatePlaylist implements Spotify.
func (Disabled) CreatePlaylist(context.Context, string) (Playlist, error) {
	return Playlist{}, ErrNotConfigured
}
