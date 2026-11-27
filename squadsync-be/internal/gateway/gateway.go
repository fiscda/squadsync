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
	// PostAnnouncement posts a short plain-text announcement of an
	// Esemeny's public moment (confirmed, reopened, cancelled) in a channel.
	PostAnnouncement(ctx context.Context, channelID string, a Announcement) (MessageRef, error)
	// SendAnnouncementDM sends the same announcement to one Discord account
	// by direct message, for a Csoport without a Csatorna.
	SendAnnouncementDM(ctx context.Context, discordID string, a Announcement) error
	// SendDropOutDM tells one Discord account of the Szervezo that a
	// Resztvevo dropped out.
	SendDropOutDM(ctx context.Context, discordID string, d DropOut) error
}

// Announcement is a short plain-text message about a public moment of an
// Esemeny. Its fields are plain text; the bot words and escapes them.
type Announcement struct {
	// Headline is fixed text from the core, e.g. "✅ Véglegesítve".
	Headline    string
	CsoportName string
	Title       string
	// Detail is extra text after the title, such as the confirmed
	// Idopont's time, "" for none.
	Detail string
	// Link is the Esemeny's message in the Csatorna, or its web page.
	// It may be a localhost URL, shown as text.
	Link string
}

// DropOut tells the Szervezo that someone dropped from igen on the
// confirmed Idopont. Its fields are plain text; the bot words and escapes
// them.
type DropOut struct {
	// Name is who dropped out.
	Name        string
	CsoportName string
	Title       string
	// Link is the Esemeny's web page.
	Link string
	// Felszabadult is what the drop-out's released claims freed, e.g.
	// "2 db sátor, 1 db gázpalack"; "" for nothing.
	Felszabadult string
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
	// Resztvevok are the names of the Resztvevok, set only while the
	// Esemeny has a confirmed Idopont.
	Resztvevok []string
	// KiegeszitoSummary is the Kiegeszitok' summary line, e.g.
	// "🎒 2 hiányzik", "" when none is on.
	KiegeszitoSummary string
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
	// Confirmed marks the Esemeny's confirmed Idopont.
	Confirmed bool
	// Szabad is how many Tags are szabad on every day it covers, out of
	// Osszes Tags (their Raeres).
	Szabad, Osszes int
	// NincsRaeres means none of its days can have a Raeres (they are past
	// or beyond the window): show "–" instead of the counts.
	NincsRaeres bool
}

// DiscordIdentity is the Discord account behind an OAuth authorization.
type DiscordIdentity struct {
	// ID is the Discord user id.
	ID string
	// Name is the account's display name (or its username if it has none).
	Name string
}

// ErrDiscordCodeRejected means Discord refused an OAuth authorization code:
// it was invalid, expired, already used, or issued for another redirect URI.
var ErrDiscordCodeRejected = errors.New("gateway: discord rejected the authorization code")

// DiscordOAuth finds out which Discord account authorized the app, for
// linking a further Discord account to a Felhasznalo (ADR-0002).
type DiscordOAuth interface {
	// ExchangeCode exchanges an OAuth authorization code (scope identify)
	// for the identity of the Discord account that granted it. redirectURI
	// must be the one the authorization was started with. It fails with
	// ErrDiscordCodeRejected when Discord refuses the code.
	ExchangeCode(ctx context.Context, code, redirectURI string) (DiscordIdentity, error)
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

// ExchangeCode implements DiscordOAuth.
func (Disabled) ExchangeCode(context.Context, string, string) (DiscordIdentity, error) {
	return DiscordIdentity{}, ErrNotConfigured
}

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

// PostAnnouncement implements DiscordMessenger.
func (Disabled) PostAnnouncement(context.Context, string, Announcement) (MessageRef, error) {
	return MessageRef{}, ErrNotConfigured
}

// SendAnnouncementDM implements DiscordMessenger.
func (Disabled) SendAnnouncementDM(context.Context, string, Announcement) error {
	return ErrNotConfigured
}

// SendDropOutDM implements DiscordMessenger.
func (Disabled) SendDropOutDM(context.Context, string, DropOut) error {
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
