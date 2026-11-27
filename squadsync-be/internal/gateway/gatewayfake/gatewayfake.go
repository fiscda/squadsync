// Package gatewayfake has recording fakes of the outbound gateways, for
// tests. Each fake remembers every call so a test can assert on what was
// sent to whom. They never talk to a real service.
package gatewayfake

import (
	"context"
	"fmt"
	"sync"

	"squadsync-be/internal/gateway"
)

// DM is one direct message sent through the fake Discord.
type DM struct {
	DiscordID string
	Content   string
}

// Discord records direct messages instead of sending them.
type Discord struct {
	mu  sync.Mutex
	dms []DM
}

var _ gateway.DiscordMessenger = (*Discord)(nil)

// SendDM records the message.
func (d *Discord) SendDM(_ context.Context, discordID string, content string) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.dms = append(d.dms, DM{DiscordID: discordID, Content: content})
	return nil
}

// DMs returns every message sent so far, in order.
func (d *Discord) DMs() []DM {
	d.mu.Lock()
	defer d.mu.Unlock()
	return append([]DM(nil), d.dms...)
}

// DMsTo returns the messages sent to one Discord account, in order.
func (d *Discord) DMsTo(discordID string) []DM {
	var out []DM
	for _, dm := range d.DMs() {
		if dm.DiscordID == discordID {
			out = append(out, dm)
		}
	}
	return out
}

// CalendarUpsert is one recorded GoogleCalendar.UpsertEvent call.
type CalendarUpsert struct {
	FelhasznaloID string
	Event         gateway.CalendarEvent
}

// GoogleCalendar records calendar writes instead of calling Google.
type GoogleCalendar struct {
	mu      sync.Mutex
	upserts []CalendarUpsert
}

var _ gateway.GoogleCalendar = (*GoogleCalendar)(nil)

// UpsertEvent records the call.
func (g *GoogleCalendar) UpsertEvent(_ context.Context, felhasznaloID string, event gateway.CalendarEvent) error {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.upserts = append(g.upserts, CalendarUpsert{FelhasznaloID: felhasznaloID, Event: event})
	return nil
}

// Upserts returns every recorded upsert, in order.
func (g *GoogleCalendar) Upserts() []CalendarUpsert {
	g.mu.Lock()
	defer g.mu.Unlock()
	return append([]CalendarUpsert(nil), g.upserts...)
}

// Spotify records playlist operations instead of calling Spotify. Created
// playlists get predictable ids: "playlist-1", "playlist-2", ...
type Spotify struct {
	mu        sync.Mutex
	playlists []string
}

var _ gateway.Spotify = (*Spotify)(nil)

// CreatePlaylist records the playlist name and returns a fake playlist.
func (s *Spotify) CreatePlaylist(_ context.Context, name string) (gateway.Playlist, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.playlists = append(s.playlists, name)
	id := fmt.Sprintf("playlist-%d", len(s.playlists))
	return gateway.Playlist{ID: id, URL: "https://open.spotify.com/playlist/" + id}, nil
}

// CreatedPlaylists returns the names of every created playlist, in order.
func (s *Spotify) CreatedPlaylists() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]string(nil), s.playlists...)
}

// CodeExchange is one recorded DiscordOAuth.ExchangeCode call.
type CodeExchange struct {
	Code        string
	RedirectURI string
}

// DiscordOAuth answers authorization codes from a table set up by the test
// instead of calling Discord, and records every exchange. An unknown code is
// refused with gateway.ErrDiscordCodeRejected.
type DiscordOAuth struct {
	mu        sync.Mutex
	codes     map[string]gateway.DiscordIdentity
	exchanges []CodeExchange
	err       error
}

var _ gateway.DiscordOAuth = (*DiscordOAuth)(nil)

// Authorize makes code stand for the given Discord account, as if that
// account had just authorized the app, and returns the code.
func (d *DiscordOAuth) Authorize(code string, identity gateway.DiscordIdentity) string {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.codes == nil {
		d.codes = map[string]gateway.DiscordIdentity{}
	}
	d.codes[code] = identity
	return code
}

// ExchangeCode records the call and answers from the table.
func (d *DiscordOAuth) ExchangeCode(_ context.Context, code, redirectURI string) (gateway.DiscordIdentity, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.exchanges = append(d.exchanges, CodeExchange{Code: code, RedirectURI: redirectURI})
	if d.err != nil {
		return gateway.DiscordIdentity{}, d.err
	}
	identity, ok := d.codes[code]
	if !ok {
		return gateway.DiscordIdentity{}, gateway.ErrDiscordCodeRejected
	}
	return identity, nil
}

// Exchanges returns every recorded exchange, in order.
func (d *DiscordOAuth) Exchanges() []CodeExchange {
	d.mu.Lock()
	defer d.mu.Unlock()
	return append([]CodeExchange(nil), d.exchanges...)
}

// Fail makes every later ExchangeCode fail with err (nil restores normal
// answers).
func (d *DiscordOAuth) Fail(err error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.err = err
}
