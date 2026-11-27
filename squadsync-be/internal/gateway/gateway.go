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
	// PostEsemenyMessage posts an Esemeny's message in a channel.
	PostEsemenyMessage(ctx context.Context, channelID string, m EsemenyMessage) (MessageRef, error)
	// SendVoteRequestDM asks one Discord account, by direct message, to
	// answer a new Esemeny.
	SendVoteRequestDM(ctx context.Context, discordID string, r VoteRequest) error
	// SendEsemenyDM sends an Esemeny's message to one Discord account by
	// direct message, with the vote request as text above it.
	SendEsemenyDM(ctx context.Context, discordID string, r VoteRequest, m EsemenyMessage) (MessageRef, error)
	// EditEsemenyMessage replaces an Esemeny's message posted earlier
	// (in a channel or a DM) with m, in place and without notifying anyone.
	EditEsemenyMessage(ctx context.Context, ref MessageRef, m EsemenyMessage) error
}

// VoteRequest asks a Tag to answer a new Esemeny. Its fields are plain
// text; the bot words and escapes them.
type VoteRequest struct {
	CsoportName string
	Title       string
	// ChannelMessageURL links to the Esemeny's post in the Csatorna, "" for
	// none.
	ChannelMessageURL string
	// WebURL links to the Esemeny's page on the web, "" for none.
	WebURL string
}

// MessageRef identifies one message the bot posted.
type MessageRef struct {
	ChannelID string
	MessageID string
}

// EsemenyMessage is what an Esemeny's Discord message says. The application
// core decides its content; the bot only lays it out (embed, buttons) and
// escapes the plain-text fields for Discord's markdown.
type EsemenyMessage struct {
	EsemenyID string
	Title     string
	// URL is the Esemeny's page on the web, set only when Discord accepts
	// it as a link (not for localhost); then the Note shows it instead.
	URL         string
	Description string
	CsoportName string
	// Allapot is the Esemeny's state, as shown (e.g. "Egyeztetés").
	Allapot      string
	SzervezoName string
	// HelyszinName and HelyszinURL (a Maps link) are both optional.
	HelyszinName string
	HelyszinURL  string
	// Idopontok are in chronological order. At most MaxAnswerRows of them
	// are Answerable.
	Idopontok []IdopontLine
	// Note is an extra line under the Idopontok, "" for none.
	Note string
	// WebButton asks for a link button to URL, for when the answer buttons
	// don't fit. It is never set without URL.
	WebButton bool
}

// MaxAnswerRows is how many Idopontok can have answer buttons: Discord
// allows 5 rows of components on a message.
const MaxAnswerRows = 5

// IdopontLine is one Idopont in an EsemenyMessage.
type IdopontLine struct {
	ID string
	// Label is its Budapest time, ready to show.
	Label            string
	Igen, Talan, Nem int
	// Helyezes is 0 when nobody has answered it.
	Helyezes int
	// Answerable asks for igen/talán/nem buttons on it.
	Answerable bool
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

// PostEsemenyMessage implements DiscordMessenger.
func (Disabled) PostEsemenyMessage(context.Context, string, EsemenyMessage) (MessageRef, error) {
	return MessageRef{}, ErrNotConfigured
}

// SendVoteRequestDM implements DiscordMessenger.
func (Disabled) SendVoteRequestDM(context.Context, string, VoteRequest) error {
	return ErrNotConfigured
}

// SendEsemenyDM implements DiscordMessenger.
func (Disabled) SendEsemenyDM(context.Context, string, VoteRequest, EsemenyMessage) (MessageRef, error) {
	return MessageRef{}, ErrNotConfigured
}

// EditEsemenyMessage implements DiscordMessenger.
func (Disabled) EditEsemenyMessage(context.Context, MessageRef, EsemenyMessage) error {
	return ErrNotConfigured
}

// UpsertEvent implements GoogleCalendar.
func (Disabled) UpsertEvent(context.Context, string, CalendarEvent) error {
	return ErrNotConfigured
}

// CreatePlaylist implements Spotify.
func (Disabled) CreatePlaylist(context.Context, string) (Playlist, error) {
	return Playlist{}, ErrNotConfigured
}
