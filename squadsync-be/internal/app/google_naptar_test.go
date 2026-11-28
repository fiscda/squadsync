package app_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"squadsync-be/internal/app"
	"squadsync-be/internal/apptest"
	"squadsync-be/internal/domain"
	"squadsync-be/internal/gateway"
	"squadsync-be/internal/gateway/gatewayfake"
)

// connectGoogle connects the Felhasznalo's Google Calendar with the refresh
// token "refresh-<id>".
func connectGoogle(t *testing.T, h *apptest.Harness, felhasznaloID string) {
	t.Helper()
	code := h.Calendar.Authorize("code-"+felhasznaloID, "refresh-"+felhasznaloID)
	if err := h.App.ConnectGoogleNaptar(context.Background(), felhasznaloID, code); err != nil {
		t.Fatalf("ConnectGoogleNaptar(%s): %v", felhasznaloID, err)
	}
}

// runSync runs the due jobs until none is left (creating a calendar
// schedules the event's write as a follow-up).
func runSync(t *testing.T, h *apptest.Harness) {
	t.Helper()
	ctx := context.Background()
	for range 5 {
		if err := h.App.RunDueJobs(ctx); err != nil {
			t.Fatalf("RunDueJobs: %v", err)
		}
		ids, err := h.Store.ListDueJobIDs(ctx, h.Clock.Now(), 100)
		if err != nil {
			t.Fatalf("ListDueJobIDs: %v", err)
		}
		if len(ids) == 0 {
			return
		}
	}
	t.Fatalf("jobs still due after 5 runs")
}

// eventsOf returns the events in the Felhasznalo's live calendars.
func eventsOf(h *apptest.Harness, felhasznaloID string) []gateway.CalendarEvent {
	out := []gateway.CalendarEvent{}
	for _, c := range h.Calendar.Calendars() {
		if c.RefreshToken == "refresh-"+felhasznaloID && !c.Deleted {
			out = append(out, h.Calendar.Events(c.ID)...)
		}
	}
	return out
}

func expectEvents(t *testing.T, h *apptest.Harness, felhasznaloID string, titles ...string) []gateway.CalendarEvent {
	t.Helper()
	events := eventsOf(h, felhasznaloID)
	got := []string{}
	for _, e := range events {
		got = append(got, e.Title)
	}
	if strings.Join(got, "|") != strings.Join(titles, "|") {
		t.Fatalf("%s's events = %q, want %q", felhasznaloID, got, titles)
	}
	return events
}

// callsWith returns the recorded calls of the ops made with the
// Felhasznalo's token.
func callsWith(h *apptest.Harness, felhasznaloID string, ops ...string) []gatewayfake.CalendarCall {
	out := []gatewayfake.CalendarCall{}
	for _, c := range h.Calendar.Calls(ops...) {
		if c.RefreshToken == "refresh-"+felhasznaloID {
			out = append(out, c)
		}
	}
	return out
}

func googleState(t *testing.T, h *apptest.Harness, felhasznaloID string) app.GoogleNaptarState {
	t.Helper()
	st, err := h.App.GoogleNaptar(context.Background(), felhasznaloID)
	if err != nil {
		t.Fatalf("GoogleNaptar(%s): %v", felhasznaloID, err)
	}
	return st.State
}

func editTitle(t *testing.T, h *apptest.Harness, esemenyID, title string) {
	t.Helper()
	if err := h.App.EditEsemeny(context.Background(), "anna", esemenyID, app.EsemenyDetailsInput{Title: title}); err != nil {
		t.Fatalf("EditEsemeny: %v", err)
	}
}

// naptarTrip has anna (the Szervezo), bela and cecil answer igen, talan
// and nem to the first of two Idopontok, then confirms it. Everyone named
// in connected connects Google Calendar first.
func naptarTrip(t *testing.T, connected ...string) (*apptest.Harness, app.EsemenyDetail) {
	t.Helper()
	h := apptest.New(t)
	c := csoportOf(t, h, "bela", "cecil", "dani")
	for _, f := range connected {
		connectGoogle(t, h, f)
	}
	e := propose(t, h, "anna", c.ID, 2)
	d := detail(t, h, "anna", e.ID)
	answer(t, h, "anna", d, 0, domain.Igen)
	answer(t, h, "bela", d, 0, domain.Igen)
	answer(t, h, "cecil", d, 0, domain.Talan)
	answer(t, h, "dani", d, 0, domain.Igen)
	confirm(t, h, "anna", d, 0)
	return h, d
}

func TestConnectingStoresTheTokenCreatesTheCalendarAndBackfills(t *testing.T) {
	h, d := naptarTrip(t)
	if got := googleState(t, h, "anna"); got != app.GoogleNaptarDisconnected {
		t.Fatalf("state = %s, want disconnected", got)
	}
	runSync(t, h)
	if calls := h.Calendar.Calls(); len(calls) != 0 {
		t.Fatalf("calls before connecting = %+v, want none", calls)
	}

	connectGoogle(t, h, "anna")
	if got := googleState(t, h, "anna"); got != app.GoogleNaptarConnected {
		t.Fatalf("state = %s, want connected", got)
	}
	if ex := h.Calendar.CalendarExchanges(); len(ex) != 1 || ex[0].RedirectURI != apptest.WebURL+"/fiok/google/callback" {
		t.Fatalf("exchanges = %+v, want one with Go's redirect URI", ex)
	}
	runSync(t, h)

	cals := h.Calendar.Calendars()
	if len(cals) != 1 || cals[0].Name != "Squad Sync" || cals[0].TimeZone != "Europe/Budapest" || cals[0].RefreshToken != "refresh-anna" {
		t.Fatalf("calendars = %+v, want one Squad Sync calendar of anna's", cals)
	}
	events := expectEvents(t, h, "anna", "Társasjáték")
	if !strings.Contains(events[0].Description, apptest.WebURL+"/esemenyek/"+d.ID) {
		t.Fatalf("description = %q, want the link to the web page", events[0].Description)
	}
	// Every call went to the app's own calendar.
	for _, c := range h.Calendar.Calls("upsert_event", "delete_event") {
		if c.CalendarID != cals[0].ID {
			t.Fatalf("call %+v outside the Squad Sync calendar", c)
		}
	}
}

func TestConnectingWithARefusedCodeOrWithoutGoogleFails(t *testing.T) {
	h := apptest.New(t)
	ctx := context.Background()
	addFelhasznalo(t, h, "anna", "Anna")
	if err := h.App.ConnectGoogleNaptar(ctx, "anna", "unknown"); !errors.Is(err, app.ErrGoogleCodeRejected) {
		t.Fatalf("err = %v, want ErrGoogleCodeRejected", err)
	}
	if got := googleState(t, h, "anna"); got != app.GoogleNaptarDisconnected {
		t.Fatalf("state = %s, want disconnected", got)
	}

	off := app.New(app.Deps{Store: h.Store, Clock: h.Clock, Discord: h.Discord, Calendar: gateway.Disabled{}, Spotify: h.Spotify, WebURL: apptest.WebURL})
	if err := off.ConnectGoogleNaptar(ctx, "anna", "code"); !errors.Is(err, gateway.ErrNotConfigured) {
		t.Fatalf("err = %v, want ErrNotConfigured", err)
	}
	if st, _ := off.GoogleNaptar(ctx, "anna"); st.State != app.GoogleNaptarNotConfigured {
		t.Fatalf("state = %s, want not_configured", st.State)
	}
}

func TestConfirmingCreatesEventsForConnectedResztvevokOnly(t *testing.T) {
	h, d := naptarTrip(t, "anna", "bela", "cecil")
	runSync(t, h)

	idopont := d.Idopontok[0]
	events := expectEvents(t, h, "anna", "Társasjáték")
	if e := events[0]; !e.Start.Equal(idopont.Start) || !e.End.Equal(*idopont.End) || e.AllDay || e.Link != apptest.WebURL+"/esemenyek/"+d.ID {
		t.Fatalf("event = %+v, want the confirmed Idopont's time and the link", e)
	}
	expectEvents(t, h, "bela", "Társasjáték")
	// cecil said talan: no Resztvevo, so not even a calendar.
	if calls := callsWith(h, "cecil"); len(calls) != 0 {
		t.Fatalf("cecil's calls = %+v, want none", calls)
	}
	// dani is a Resztvevo but not connected.
	if calls := callsWith(h, "dani"); len(calls) != 0 {
		t.Fatalf("dani's calls = %+v, want none", calls)
	}
	if anna, bela := eventsOf(h, "anna")[0].ID, eventsOf(h, "bela")[0].ID; anna == bela {
		t.Fatalf("anna and bela share event id %s", anna)
	}
}

func TestEditingUpdatesTheEvent(t *testing.T) {
	h, d := naptarTrip(t, "anna")
	runSync(t, h)
	before := eventsOf(h, "anna")[0]

	if err := h.App.EditEsemeny(context.Background(), "anna", d.ID, app.EsemenyDetailsInput{
		Title:       "Bográcsozás",
		Description: "Hozz kanalat.",
		Helyszin:    domain.Helyszin{Name: "Kert", URL: "https://maps.google.com/?q=kert"},
	}); err != nil {
		t.Fatalf("EditEsemeny: %v", err)
	}
	runSync(t, h)

	after := expectEvents(t, h, "anna", "Bográcsozás")[0]
	if after.ID != before.ID || after.Location != "Kert" ||
		!strings.Contains(after.Description, "Hozz kanalat.") || !strings.Contains(after.Description, "https://maps.google.com/?q=kert") {
		t.Fatalf("event = %+v, want the same event with the new details", after)
	}
	if n := len(callsWith(h, "anna", "upsert_event")); n != 2 {
		t.Fatalf("upserts = %d, want 2", n)
	}
}

func TestReopeningAndCancellingDeleteTheEvents(t *testing.T) {
	h, d := naptarTrip(t, "anna", "bela")
	runSync(t, h)

	reopen(t, h, "anna", d.ID)
	runSync(t, h)
	expectEvents(t, h, "anna")
	expectEvents(t, h, "bela")

	confirm(t, h, "anna", d, 0)
	runSync(t, h)
	expectEvents(t, h, "bela", "Társasjáték")

	cancelEsemeny(t, h, "anna", d.ID)
	runSync(t, h)
	expectEvents(t, h, "anna")
	expectEvents(t, h, "bela")
	if n := len(callsWith(h, "bela", "delete_event")); n != 2 {
		t.Fatalf("bela's deletes = %d, want 2", n)
	}
}

func TestADropOutDeletesTheirEventAndJoiningCreatesOne(t *testing.T) {
	h, d := naptarTrip(t, "anna", "bela", "cecil")
	runSync(t, h)

	answer(t, h, "bela", d, 0, domain.Nem)
	answer(t, h, "cecil", d, 0, domain.Igen)
	runSync(t, h)

	expectEvents(t, h, "anna", "Társasjáték")
	expectEvents(t, h, "bela")
	expectEvents(t, h, "cecil", "Társasjáték")
	if n := len(callsWith(h, "anna", "upsert_event", "delete_event")); n != 1 {
		t.Fatalf("anna's event writes = %d, want only the first", n)
	}
}

func TestATagLeavingDeletesTheirEvents(t *testing.T) {
	h, d := naptarTrip(t, "anna", "bela")
	runSync(t, h)

	if err := h.App.LeaveCsoport(context.Background(), "bela", d.CsoportID); err != nil {
		t.Fatalf("LeaveCsoport: %v", err)
	}
	runSync(t, h)
	expectEvents(t, h, "bela")
	expectEvents(t, h, "anna", "Társasjáték")
}

func TestALezajlottEsemenyKeepsItsEvents(t *testing.T) {
	h, d := naptarTrip(t, "anna")
	runSync(t, h)

	h.Clock.Advance(d.Idopontok[0].End.Sub(h.Clock.Now()) + time.Hour)
	runSync(t, h)
	if got := detail(t, h, "anna", d.ID).Allapot; got != domain.Lezajlott {
		t.Fatalf("allapot = %s, want lezajlott", got)
	}
	expectEvents(t, h, "anna", "Társasjáték")
	if n := len(callsWith(h, "anna", "delete_event")); n != 0 {
		t.Fatalf("deletes = %d, want none", n)
	}
}

func TestAWholeDayIdopontBecomesAnAllDayEvent(t *testing.T) {
	h := apptest.New(t)
	ctx := context.Background()
	c := csoportOf(t, h)
	connectGoogle(t, h, "anna")
	e, err := h.App.ProposeEsemeny(ctx, "anna", c.ID, app.EsemenyInput{
		Title:     "Balaton",
		Idopontok: []app.IdopontInput{{Start: "2026-12-05", End: "2026-12-07", EgeszNapos: true}},
	})
	if err != nil {
		t.Fatalf("ProposeEsemeny: %v", err)
	}
	d := detail(t, h, "anna", e.ID)
	answer(t, h, "anna", d, 0, domain.Igen)
	confirm(t, h, "anna", d, 0)
	runSync(t, h)

	ev := expectEvents(t, h, "anna", "Balaton")[0]
	budapest, _ := time.LoadLocation("Europe/Budapest")
	if !ev.AllDay || !ev.Start.Equal(time.Date(2026, 12, 5, 0, 0, 0, 0, budapest)) || !ev.End.Equal(time.Date(2026, 12, 8, 0, 0, 0, 0, budapest)) {
		t.Fatalf("event = %+v, want all day from 12.05 to (exclusive) 12.08", ev)
	}
}

func TestRepeatedTriggersAreCoalesced(t *testing.T) {
	h, d := naptarTrip(t, "anna")
	editTitle(t, h, d.ID, "Egy")
	editTitle(t, h, d.ID, "Kettő")
	editTitle(t, h, d.ID, "Három")

	ctx := context.Background()
	ids, err := h.Store.ListDueJobIDs(ctx, h.Clock.Now(), 100)
	if err != nil {
		t.Fatalf("ListDueJobIDs: %v", err)
	}
	pending := 0
	for _, id := range ids {
		j, err := h.Store.FindJob(ctx, id)
		if err != nil {
			t.Fatalf("FindJob: %v", err)
		}
		if j.Kind == "gcal_sync" {
			pending++
		}
	}
	if pending != 1 {
		t.Fatalf("pending gcal_sync jobs = %d, want 1", pending)
	}

	runSync(t, h)
	expectEvents(t, h, "anna", "Három")
	if n := len(callsWith(h, "anna", "upsert_event")); n != 1 {
		t.Fatalf("upserts = %d, want 1", n)
	}
}

func TestARevokedTokenMarksTheConnectionBroken(t *testing.T) {
	h, d := naptarTrip(t, "anna")
	runSync(t, h)

	h.Calendar.RevokeAccess("refresh-anna")
	editTitle(t, h, d.ID, "Új cím")
	runSync(t, h)
	if got := googleState(t, h, "anna"); got != app.GoogleNaptarBroken {
		t.Fatalf("state = %s, want broken", got)
	}

	// Further changes are dropped, not retried.
	calls := len(h.Calendar.Calls())
	editTitle(t, h, d.ID, "Még újabb")
	h.Clock.Advance(3 * time.Hour)
	runSync(t, h)
	if got := len(h.Calendar.Calls()); got != calls {
		t.Fatalf("calls = %d, want still %d", got, calls)
	}
}

func TestConnectingAgainAfterBrokenSyncsWithTheNewToken(t *testing.T) {
	h, d := naptarTrip(t, "anna")
	runSync(t, h)
	h.Calendar.RevokeAccess("refresh-anna")
	editTitle(t, h, d.ID, "Új cím")
	runSync(t, h)

	// Connecting again, with another Google account: the old calendar is
	// out of the new token's reach, so a new one is created and filled.
	h.Calendar.Authorize("again", "refresh-anna-new")
	if err := h.App.ConnectGoogleNaptar(context.Background(), "anna", "again"); err != nil {
		t.Fatalf("ConnectGoogleNaptar: %v", err)
	}
	runSync(t, h)
	if got := googleState(t, h, "anna"); got != app.GoogleNaptarConnected {
		t.Fatalf("state = %s, want connected", got)
	}
	var newCal string
	for _, c := range h.Calendar.Calendars() {
		if c.RefreshToken == "refresh-anna-new" {
			newCal = c.ID
		}
	}
	if events := h.Calendar.Events(newCal); len(events) != 1 || events[0].Title != "Új cím" {
		t.Fatalf("events in the new calendar = %+v, want the current event", events)
	}
}

func TestDisconnectingStopsSyncing(t *testing.T) {
	h, d := naptarTrip(t, "anna")
	runSync(t, h)

	if err := h.App.DisconnectGoogleNaptar(context.Background(), "anna"); err != nil {
		t.Fatalf("DisconnectGoogleNaptar: %v", err)
	}
	if got := googleState(t, h, "anna"); got != app.GoogleNaptarDisconnected {
		t.Fatalf("state = %s, want disconnected", got)
	}
	if n := len(callsWith(h, "anna", "revoke")); n != 1 {
		t.Fatalf("revokes = %d, want 1", n)
	}
	writes := len(callsWith(h, "anna", "upsert_event", "delete_event", "create_calendar"))
	editTitle(t, h, d.ID, "Új cím")
	cancelEsemeny(t, h, "anna", d.ID)
	runSync(t, h)
	if got := len(callsWith(h, "anna", "upsert_event", "delete_event", "create_calendar")); got != writes {
		t.Fatalf("writes = %d, want still %d", got, writes)
	}
	// The calendar stays in their Google account, as it was.
	expectEvents(t, h, "anna", "Társasjáték")
}

func TestSyncingIsIdempotent(t *testing.T) {
	h, d := naptarTrip(t, "anna")
	// The first write fails and is retried after the backoff.
	runSync(t, h) // creates the calendar
	h.Calendar.FailNext(errors.New("google is down"))
	editTitle(t, h, d.ID, "Új cím")
	runSync(t, h)
	h.Clock.Advance(2 * time.Minute)
	runSync(t, h)
	expectEvents(t, h, "anna", "Új cím")

	// Asking again with nothing changed calls Google no more.
	calls := len(h.Calendar.Calls())
	connectGoogle(t, h, "anna") // backfills every Esemeny
	runSync(t, h)
	if got := len(h.Calendar.Calls()); got != calls {
		t.Fatalf("calls = %d, want still %d", got, calls)
	}
	expectEvents(t, h, "anna", "Új cím")
}

func TestADeletedCalendarIsCreatedAgain(t *testing.T) {
	h, d := naptarTrip(t, "anna")
	runSync(t, h)
	h.Calendar.DeleteCalendar(h.Calendar.Calendars()[0].ID)

	editTitle(t, h, d.ID, "Új cím")
	runSync(t, h)
	if n := len(h.Calendar.Calendars()); n != 2 {
		t.Fatalf("calendars = %d, want a new one", n)
	}
	expectEvents(t, h, "anna", "Új cím")
}

func TestAChangeWhileGoogleIsCalledIsNotLost(t *testing.T) {
	h, d := naptarTrip(t, "anna")
	runSync(t, h) // creates the calendar and the event

	editTitle(t, h, d.ID, "Egy")
	// While the job writes "Egy", the title changes again.
	h.Calendar.DuringNext(func() { editTitle(t, h, d.ID, "Kettő") })
	runSync(t, h)
	expectEvents(t, h, "anna", "Kettő")
}
