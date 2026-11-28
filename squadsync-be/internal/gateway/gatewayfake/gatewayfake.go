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
	// Content is a plain DM's text.
	Content string
	// VoteRequest is set for a vote request, alone or above an Esemeny.
	VoteRequest *gateway.VoteRequest
	// Esemeny is the Esemeny message sent, nil if none.
	Esemeny *gateway.EsemenyMessage
	// Ref is the sent message, set only when Esemeny is.
	Ref gateway.MessageRef
	// Announcement is set for an announcement sent by DM.
	Announcement *gateway.Announcement
	// DropOut is set for a drop-out DM to the Szervezo.
	DropOut *gateway.DropOut
	// Emlekezteto is set for a reminder of a confirmed Esemeny.
	Emlekezteto *gateway.Emlekezteto
	// Hatarido is set for the Szervezo's DM when the voting Hatarido passed.
	Hatarido *gateway.HataridoDM
	// Auto is set for a passenger's DM that their Auto is gone.
	Auto *gateway.AutoDM
	// Szallas is set for a sleeper's DM that their Szoba is gone.
	Szallas *gateway.SzallasDM
}

// AnnouncementPost is one announcement posted in a channel.
type AnnouncementPost struct {
	Ref          gateway.MessageRef
	Announcement gateway.Announcement
}

// Post is one Esemeny message posted in a channel.
type Post struct {
	Ref     gateway.MessageRef
	Message gateway.EsemenyMessage
}

// Edit is one edit of a posted Esemeny message.
type Edit struct {
	Ref     gateway.MessageRef
	Message gateway.EsemenyMessage
}

// Discord records messages instead of sending them. Messages get
// predictable ids: "msg-1", "msg-2", ...; a DM's channel id is "dm-" and the
// Discord id.
type Discord struct {
	mu    sync.Mutex
	fail  error
	next  int
	dms   []DM
	posts []Post
	edits []Edit

	announcements []AnnouncementPost
}

var _ gateway.DiscordMessenger = (*Discord)(nil)

// Fail makes every later call fail with err, as when Discord is down. Calls
// that fail aren't recorded. Fail(nil) makes calls succeed again.
func (d *Discord) Fail(err error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.fail = err
}

func (d *Discord) newRef(channelID string) gateway.MessageRef {
	d.next++
	return gateway.MessageRef{ChannelID: channelID, MessageID: fmt.Sprintf("msg-%d", d.next)}
}

// SendDM records the message.
func (d *Discord) SendDM(_ context.Context, discordID string, content string) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.fail != nil {
		return d.fail
	}
	d.dms = append(d.dms, DM{DiscordID: discordID, Content: content})
	return nil
}

// SendVoteRequestDM records the message.
func (d *Discord) SendVoteRequestDM(_ context.Context, discordID string, r gateway.VoteRequest) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.fail != nil {
		return d.fail
	}
	d.dms = append(d.dms, DM{DiscordID: discordID, VoteRequest: &r})
	return nil
}

// SendEsemenyDM records the message.
func (d *Discord) SendEsemenyDM(_ context.Context, discordID string, r gateway.VoteRequest, m gateway.EsemenyMessage) (gateway.MessageRef, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.fail != nil {
		return gateway.MessageRef{}, d.fail
	}
	ref := d.newRef("dm-" + discordID)
	d.dms = append(d.dms, DM{DiscordID: discordID, VoteRequest: &r, Esemeny: &m, Ref: ref})
	return ref, nil
}

// PostEsemenyMessage records the post.
func (d *Discord) PostEsemenyMessage(_ context.Context, channelID string, m gateway.EsemenyMessage) (gateway.MessageRef, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.fail != nil {
		return gateway.MessageRef{}, d.fail
	}
	ref := d.newRef(channelID)
	d.posts = append(d.posts, Post{Ref: ref, Message: m})
	return ref, nil
}

// EditEsemenyMessage records the edit.
func (d *Discord) EditEsemenyMessage(_ context.Context, ref gateway.MessageRef, m gateway.EsemenyMessage) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.fail != nil {
		return d.fail
	}
	d.edits = append(d.edits, Edit{Ref: ref, Message: m})
	return nil
}

// PostAnnouncement records the post.
func (d *Discord) PostAnnouncement(_ context.Context, channelID string, a gateway.Announcement) (gateway.MessageRef, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.fail != nil {
		return gateway.MessageRef{}, d.fail
	}
	ref := d.newRef(channelID)
	d.announcements = append(d.announcements, AnnouncementPost{Ref: ref, Announcement: a})
	return ref, nil
}

// SendAnnouncementDM records the message.
func (d *Discord) SendAnnouncementDM(_ context.Context, discordID string, a gateway.Announcement) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.fail != nil {
		return d.fail
	}
	d.dms = append(d.dms, DM{DiscordID: discordID, Announcement: &a})
	return nil
}

// SendDropOutDM records the message.
func (d *Discord) SendDropOutDM(_ context.Context, discordID string, o gateway.DropOut) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.fail != nil {
		return d.fail
	}
	d.dms = append(d.dms, DM{DiscordID: discordID, DropOut: &o})
	return nil
}

// SendEmlekeztetoDM records the message.
func (d *Discord) SendEmlekeztetoDM(_ context.Context, discordID string, e gateway.Emlekezteto) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.fail != nil {
		return d.fail
	}
	d.dms = append(d.dms, DM{DiscordID: discordID, Emlekezteto: &e})
	return nil
}

// SendHataridoDM records the message.
func (d *Discord) SendHataridoDM(_ context.Context, discordID string, h gateway.HataridoDM) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.fail != nil {
		return d.fail
	}
	d.dms = append(d.dms, DM{DiscordID: discordID, Hatarido: &h})
	return nil
}

// SendAutoDM records the message.
func (d *Discord) SendAutoDM(_ context.Context, discordID string, a gateway.AutoDM) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.fail != nil {
		return d.fail
	}
	d.dms = append(d.dms, DM{DiscordID: discordID, Auto: &a})
	return nil
}

// SendSzallasDM records the message.
func (d *Discord) SendSzallasDM(_ context.Context, discordID string, z gateway.SzallasDM) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.fail != nil {
		return d.fail
	}
	d.dms = append(d.dms, DM{DiscordID: discordID, Szallas: &z})
	return nil
}

// Announcements returns every announcement posted in a channel, in order.
func (d *Discord) Announcements() []AnnouncementPost {
	d.mu.Lock()
	defer d.mu.Unlock()
	return append([]AnnouncementPost(nil), d.announcements...)
}

// DMs returns every direct message sent so far, in order.
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

// Posts returns every Esemeny message posted in a channel, in order.
func (d *Discord) Posts() []Post {
	d.mu.Lock()
	defer d.mu.Unlock()
	return append([]Post(nil), d.posts...)
}

// Edits returns every edit of a posted message, in order.
func (d *Discord) Edits() []Edit {
	d.mu.Lock()
	defer d.mu.Unlock()
	return append([]Edit(nil), d.edits...)
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
