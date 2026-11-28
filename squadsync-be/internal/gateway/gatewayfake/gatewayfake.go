// Package gatewayfake has recording fakes of the outbound gateways, for
// tests. Each fake remembers every call so a test can assert on what was
// sent to whom. They never talk to a real service.
package gatewayfake

import (
	"context"
	"fmt"
	"slices"
	"strings"
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
	// Szervezo is set for the DM to a Tag who became an Esemeny's Szervezo.
	Szervezo *gateway.SzervezoDM
	// Emlekezteto is set for a reminder of a confirmed Esemeny.
	Emlekezteto *gateway.Emlekezteto
	// Hatarido is set for the Szervezo's DM when the voting Hatarido passed.
	Hatarido *gateway.HataridoDM
	// Auto is set for a passenger's DM that their Auto is gone.
	Auto *gateway.AutoDM
	// ElszamolasEmlekezteto is set for the reminder to confirm Kiadasok.
	ElszamolasEmlekezteto *gateway.ElszamolasEmlekezteto
	// Elszamolas is set for a Felhasznalo's result of an Elszamolas.
	Elszamolas *gateway.ElszamolasDM
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

// SendSzervezoDM records the message.
func (d *Discord) SendSzervezoDM(_ context.Context, discordID string, s gateway.SzervezoDM) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.fail != nil {
		return d.fail
	}
	d.dms = append(d.dms, DM{DiscordID: discordID, Szervezo: &s})
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

// SendElszamolasEmlekeztetoDM records the message.
func (d *Discord) SendElszamolasEmlekeztetoDM(_ context.Context, discordID string, e gateway.ElszamolasEmlekezteto) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.fail != nil {
		return d.fail
	}
	d.dms = append(d.dms, DM{DiscordID: discordID, ElszamolasEmlekezteto: &e})
	return nil
}

// SendElszamolasDM records the message.
func (d *Discord) SendElszamolasDM(_ context.Context, discordID string, e gateway.ElszamolasDM) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.fail != nil {
		return d.fail
	}
	d.dms = append(d.dms, DM{DiscordID: discordID, Elszamolas: &e})
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

// CalendarCall is one recorded GoogleCalendar call (except the code
// exchange, see CalendarExchanges).
type CalendarCall struct {
	// Op is "create_calendar", "upsert_event", "delete_event" or "revoke".
	Op           string
	RefreshToken string
	CalendarID   string
	// EventID is set for upsert_event and delete_event.
	EventID string
	// Event is set for upsert_event.
	Event gateway.CalendarEvent
}

// FakeCalendar is a calendar the fake Google holds.
type FakeCalendar struct {
	ID           string
	RefreshToken string
	Name         string
	TimeZone     string
	// Deleted is set by DeleteCalendar, as if the owner deleted it.
	Deleted bool
}

// GoogleCalendar keeps calendars and events in memory instead of calling
// Google, and records every call. Codes are answered from a table set up
// with Authorize; an unknown one is refused with
// gateway.ErrGoogleCodeRejected. Calendars get predictable ids:
// "calendar-1", "calendar-2", ...
type GoogleCalendar struct {
	mu        sync.Mutex
	codes     map[string]string
	exchanges []CodeExchange
	calls     []CalendarCall
	calendars []FakeCalendar
	events    map[string]map[string]gateway.CalendarEvent
	revoked   map[string]bool
	failNext  error
	during    func()
}

var _ gateway.GoogleCalendar = (*GoogleCalendar)(nil)

// Authorize makes code stand for a Google account that just granted access
// with the given refresh token, and returns the code.
func (g *GoogleCalendar) Authorize(code, refreshToken string) string {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.codes == nil {
		g.codes = map[string]string{}
	}
	g.codes[code] = refreshToken
	return code
}

// RevokeAccess makes Google refuse the refresh token from now on, as if
// its owner revoked the app's access in their Google account.
func (g *GoogleCalendar) RevokeAccess(refreshToken string) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.revoked == nil {
		g.revoked = map[string]bool{}
	}
	g.revoked[refreshToken] = true
}

// FailNext makes the next call (other than an exchange) fail with err,
// without doing anything. It is still recorded.
func (g *GoogleCalendar) FailNext(err error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.failNext = err
}

// DuringNext runs fn during the next call to create a calendar, upsert or
// delete an event, before it takes effect: as if something happened while
// Google was being called.
func (g *GoogleCalendar) DuringNext(fn func()) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.during = fn
}

func (g *GoogleCalendar) runDuring() {
	g.mu.Lock()
	fn := g.during
	g.during = nil
	g.mu.Unlock()
	if fn != nil {
		fn()
	}
}

// DeleteCalendar deletes a calendar as its owner would in Google.
func (g *GoogleCalendar) DeleteCalendar(calendarID string) {
	g.mu.Lock()
	defer g.mu.Unlock()
	for n := range g.calendars {
		if g.calendars[n].ID == calendarID {
			g.calendars[n].Deleted = true
		}
	}
	delete(g.events, calendarID)
}

// ExchangeCalendarCode records the exchange and answers from the table.
func (g *GoogleCalendar) ExchangeCalendarCode(_ context.Context, code, redirectURI string) (string, error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.exchanges = append(g.exchanges, CodeExchange{Code: code, RedirectURI: redirectURI})
	token, ok := g.codes[code]
	if !ok {
		return "", gateway.ErrGoogleCodeRejected
	}
	return token, nil
}

// start records a call and tells whether it may go ahead (the caller holds
// the lock).
func (g *GoogleCalendar) start(c CalendarCall) error {
	g.calls = append(g.calls, c)
	if err := g.failNext; err != nil {
		g.failNext = nil
		return err
	}
	if g.revoked[c.RefreshToken] {
		return gateway.ErrGoogleTokenRevoked
	}
	return nil
}

func (g *GoogleCalendar) calendar(id string) (FakeCalendar, bool) {
	for _, c := range g.calendars {
		if c.ID == id && !c.Deleted {
			return c, true
		}
	}
	return FakeCalendar{}, false
}

// CreateCalendar records the call and creates the calendar.
func (g *GoogleCalendar) CreateCalendar(_ context.Context, refreshToken, name, timeZone string) (string, error) {
	g.runDuring()
	g.mu.Lock()
	defer g.mu.Unlock()
	if err := g.start(CalendarCall{Op: "create_calendar", RefreshToken: refreshToken}); err != nil {
		return "", err
	}
	id := fmt.Sprintf("calendar-%d", len(g.calendars)+1)
	g.calendars = append(g.calendars, FakeCalendar{ID: id, RefreshToken: refreshToken, Name: name, TimeZone: timeZone})
	g.calls[len(g.calls)-1].CalendarID = id
	return id, nil
}

// UpsertEvent records the call and stores the event.
func (g *GoogleCalendar) UpsertEvent(_ context.Context, refreshToken, calendarID string, event gateway.CalendarEvent) error {
	g.runDuring()
	g.mu.Lock()
	defer g.mu.Unlock()
	if err := g.start(CalendarCall{Op: "upsert_event", RefreshToken: refreshToken, CalendarID: calendarID, EventID: event.ID, Event: event}); err != nil {
		return err
	}
	c, ok := g.calendar(calendarID)
	if !ok || c.RefreshToken != refreshToken {
		return gateway.ErrGoogleCalendarGone
	}
	if g.events == nil {
		g.events = map[string]map[string]gateway.CalendarEvent{}
	}
	if g.events[calendarID] == nil {
		g.events[calendarID] = map[string]gateway.CalendarEvent{}
	}
	g.events[calendarID][event.ID] = event
	return nil
}

// DeleteEvent records the call and deletes the event.
func (g *GoogleCalendar) DeleteEvent(_ context.Context, refreshToken, calendarID, eventID string) error {
	g.runDuring()
	g.mu.Lock()
	defer g.mu.Unlock()
	if err := g.start(CalendarCall{Op: "delete_event", RefreshToken: refreshToken, CalendarID: calendarID, EventID: eventID}); err != nil {
		return err
	}
	delete(g.events[calendarID], eventID)
	return nil
}

// RevokeCalendarToken records the call and makes the token refused.
func (g *GoogleCalendar) RevokeCalendarToken(_ context.Context, refreshToken string) error {
	g.mu.Lock()
	defer g.mu.Unlock()
	if err := g.start(CalendarCall{Op: "revoke", RefreshToken: refreshToken}); err != nil {
		return err
	}
	if g.revoked == nil {
		g.revoked = map[string]bool{}
	}
	g.revoked[refreshToken] = true
	return nil
}

// CalendarExchanges returns every recorded code exchange, in order.
func (g *GoogleCalendar) CalendarExchanges() []CodeExchange {
	g.mu.Lock()
	defer g.mu.Unlock()
	return append([]CodeExchange(nil), g.exchanges...)
}

// Calls returns every recorded call of the given ops (all with none), in
// order.
func (g *GoogleCalendar) Calls(ops ...string) []CalendarCall {
	g.mu.Lock()
	defer g.mu.Unlock()
	out := []CalendarCall{}
	for _, c := range g.calls {
		if len(ops) == 0 || slices.Contains(ops, c.Op) {
			out = append(out, c)
		}
	}
	return out
}

// Calendars returns every calendar created, in order.
func (g *GoogleCalendar) Calendars() []FakeCalendar {
	g.mu.Lock()
	defer g.mu.Unlock()
	return append([]FakeCalendar(nil), g.calendars...)
}

// Events returns the events now in the calendar, ordered by id.
func (g *GoogleCalendar) Events(calendarID string) []gateway.CalendarEvent {
	g.mu.Lock()
	defer g.mu.Unlock()
	out := []gateway.CalendarEvent{}
	for _, e := range g.events[calendarID] {
		out = append(out, e)
	}
	slices.SortFunc(out, func(a, b gateway.CalendarEvent) int { return strings.Compare(a.ID, b.ID) })
	return out
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
