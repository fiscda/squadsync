package app

import (
	"context"
	"crypto/sha256"
	"encoding/base32"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"squadsync-be/internal/domain"
	"squadsync-be/internal/gateway"
	"squadsync-be/internal/store"
)

// Google Calendar sync (ADR-0005). A Felhasznalo connects Google Calendar
// from /fiok, separately from login. The app then writes only to one
// "Squad Sync" calendar it creates in their account (scope
// calendar.app.created).
//
// # The rule
//
// A Google event exists for a (Felhasznalo, Esemeny) pair exactly while the
// Esemeny is Veglegesitve or Lezajlott, the Felhasznalo is one of its
// Resztvevok (resztvevokOf), and they are connected (not broken).
//
// # Reconciling
//
// Every change that may affect the rule asks for the pair to be synced
// (requestNaptarSync): confirming, reopening, cancelling, editing, a
// Resztvevo joining or dropping out, a Tag leaving, Lezajlott, and
// connecting (which backfills). A request stamps the pair with a new random
// kerelem and schedules a durable gcal_sync job, unless one is already
// pending for the pair: repeated requests coalesce.
//
// The job (runGcalSync) reads the rule's answer and what it wrote before in
// a short transaction, then makes one Google call outside any transaction:
// create the calendar if there is none yet, upsert the event, or delete it.
// Its finish stores the result. If the pair's kerelem changed while Google
// was being called, finish schedules another job, so no change is lost. An
// event's id is derived from the pair, so a repeated call never duplicates.
//
// Google refusing the token (invalid_grant) marks the connection broken:
// /fiok asks to connect again, and jobs do nothing until then. Other
// failures are retried with the job backoff.

// Errors of the Google Naptar area.
var (
	// ErrGoogleCodeRejected means Google refused the authorization code of
	// the connecting flow.
	ErrGoogleCodeRejected = fmt.Errorf("%w: google rejected the authorization code", ErrInvalidInput)
)

const (
	// GoogleNaptarCallbackPath is the Next.js route Google redirects to at
	// the end of connecting. It must be registered as a redirect URI of the
	// Google OAuth client (README.md).
	GoogleNaptarCallbackPath = "/fiok/google/callback"

	// jobGcalSync syncs one (Felhasznalo, Esemeny) pair (durable mode).
	jobGcalSync jobKind = "gcal_sync"

	googleNaptarName     = "Squad Sync"
	googleNaptarTimeZone = "Europe/Budapest"
	// googleCallTimeout bounds one call to Google.
	googleCallTimeout = 30 * time.Second
	// openEndedLength is how long the event of an Idopont with only a start
	// lasts in the calendar.
	openEndedLength = time.Hour
)

// GoogleNaptarState is where a Felhasznalo's Google Calendar connection is.
type GoogleNaptarState string

// The states of a connection.
const (
	// GoogleNaptarNotConfigured means the server has no Google credentials.
	GoogleNaptarNotConfigured GoogleNaptarState = "not_configured"
	GoogleNaptarDisconnected  GoogleNaptarState = "disconnected"
	GoogleNaptarConnected     GoogleNaptarState = "connected"
	// GoogleNaptarBroken means Google refused the token: connect again.
	GoogleNaptarBroken GoogleNaptarState = "broken"
)

// GoogleNaptarStatus is a Felhasznalo's Google Calendar connection.
type GoogleNaptarStatus struct {
	State GoogleNaptarState
	// ConnectedAt is when they connected, nil when disconnected.
	ConnectedAt *time.Time
}

// googleNaptarConfigured tells whether the server can connect Google
// Calendar: it has the Google client and the web's public URL.
func (a *App) googleNaptarConfigured() bool {
	if _, disabled := a.calendar.(gateway.Disabled); disabled || a.calendar == nil {
		return false
	}
	return strings.TrimSpace(a.webURL) != ""
}

// GoogleNaptarRedirectURI is the redirect URI of the connecting flow: the
// web app's public URL plus GoogleNaptarCallbackPath. Go owns it, so a
// caller can't make Go exchange a code for another redirect URI. It fails
// with gateway.ErrNotConfigured when the public URL isn't set.
func (a *App) GoogleNaptarRedirectURI() (string, error) {
	base := strings.TrimRight(strings.TrimSpace(a.webURL), "/")
	if base == "" {
		return "", fmt.Errorf("web url for the google redirect: %w", gateway.ErrNotConfigured)
	}
	return base + GoogleNaptarCallbackPath, nil
}

// GoogleNaptar returns the Felhasznalo's Google Calendar connection, or
// ErrNotFound for an unknown Felhasznalo.
func (a *App) GoogleNaptar(ctx context.Context, felhasznaloID string) (GoogleNaptarStatus, error) {
	if _, err := a.Felhasznalo(ctx, felhasznaloID); err != nil {
		return GoogleNaptarStatus{}, err
	}
	if !a.googleNaptarConfigured() {
		return GoogleNaptarStatus{State: GoogleNaptarNotConfigured}, nil
	}
	n, err := a.store.FindGoogleNaptar(ctx, felhasznaloID)
	if errors.Is(err, store.ErrNotFound) {
		return GoogleNaptarStatus{State: GoogleNaptarDisconnected}, nil
	}
	if err != nil {
		return GoogleNaptarStatus{}, err
	}
	at := n.CreatedAt
	if n.MegszakadtAt != nil {
		return GoogleNaptarStatus{State: GoogleNaptarBroken, ConnectedAt: &at}, nil
	}
	return GoogleNaptarStatus{State: GoogleNaptarConnected, ConnectedAt: &at}, nil
}

// ConnectGoogleNaptar connects the Felhasznalo's Google Calendar. code comes
// from the Google OAuth flow Next.js ran, whose redirect URI is
// GoogleNaptarRedirectURI; Go exchanges it for a refresh token and stores
// it. Every Esemeny they are a Resztvevo of is then synced (the first sync
// creates the "Squad Sync" calendar).
//
// Connecting again (say, after the connection broke) replaces the token and
// keeps the calendar; if the new token can't reach it, the sync creates a
// new one.
//
// It fails with ErrNotFound for an unknown Felhasznalo,
// gateway.ErrNotConfigured without Google credentials, and
// ErrGoogleCodeRejected when Google refuses the code.
func (a *App) ConnectGoogleNaptar(ctx context.Context, felhasznaloID, code string) error {
	code = strings.TrimSpace(code)
	if code == "" {
		return fmt.Errorf("%w: code is required", ErrInvalidInput)
	}
	if _, err := a.Felhasznalo(ctx, felhasznaloID); err != nil {
		return err
	}
	if !a.googleNaptarConfigured() {
		return fmt.Errorf("google calendar: %w", gateway.ErrNotConfigured)
	}
	redirectURI, err := a.GoogleNaptarRedirectURI()
	if err != nil {
		return err
	}
	cctx, cancel := context.WithTimeout(ctx, googleCallTimeout)
	token, err := a.calendar.ExchangeCalendarCode(cctx, code, redirectURI)
	cancel()
	if errors.Is(err, gateway.ErrGoogleCodeRejected) {
		return fmt.Errorf("%w (%v)", ErrGoogleCodeRejected, err)
	}
	if err != nil {
		return fmt.Errorf("exchange google code: %w", err)
	}

	return a.store.InTx(ctx, func(q store.Queries) error {
		if err := requireFelhasznalo(ctx, q, felhasznaloID); err != nil {
			return err
		}
		calendarID := ""
		old, err := q.FindGoogleNaptar(ctx, felhasznaloID)
		switch {
		case err == nil:
			calendarID = old.CalendarID
		case !errors.Is(err, store.ErrNotFound):
			return err
		}
		if err := q.UpsertGoogleNaptar(ctx, store.GoogleNaptar{
			FelhasznaloID: felhasznaloID,
			KapcsolatID:   randomID(),
			RefreshToken:  token,
			CalendarID:    calendarID,
			CreatedAt:     a.clock.Now(),
		}); err != nil {
			return err
		}
		return a.backfillNaptar(ctx, q, felhasznaloID)
	})
}

// DisconnectGoogleNaptar stops syncing the Felhasznalo's Google Calendar:
// the token and the calendar's id are forgotten, and the token is revoked
// at Google (best effort). The "Squad Sync" calendar stays in their Google
// account, with its events; they can delete it. Disconnecting when not
// connected changes nothing.
func (a *App) DisconnectGoogleNaptar(ctx context.Context, felhasznaloID string) error {
	var token string
	err := a.store.InTx(ctx, func(q store.Queries) error {
		n, err := q.FindGoogleNaptar(ctx, felhasznaloID)
		if errors.Is(err, store.ErrNotFound) {
			return nil
		}
		if err != nil {
			return err
		}
		token = n.RefreshToken
		return q.DeleteGoogleNaptar(ctx, felhasznaloID)
	})
	if err != nil || token == "" {
		return err
	}
	cctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), googleCallTimeout)
	defer cancel()
	if err := a.calendar.RevokeCalendarToken(cctx, token); err != nil {
		a.log.Warn("revoke google token", "felhasznalo", felhasznaloID, "err", err)
	}
	return nil
}

// --- Requesting a sync ---

// gcalSyncPayload identifies a gcal_sync job: the pair's Felhasznalo (the
// Esemeny is the job's) and the kerelem it was scheduled for, which makes
// each job distinct.
type gcalSyncPayload struct {
	Felhasznalo string `json:"felhasznalo"`
	Kerelem     string `json:"kerelem"`
}

// requestNaptarSync asks for the Esemeny to be synced into the calendars of
// the Felhasznalok, in the caller's transaction. Those who aren't connected
// (or whose connection is broken) are skipped. A pair with a pending job
// gets no second one.
func (a *App) requestNaptarSync(ctx context.Context, q store.Queries, esemenyID string, felhasznaloIDs ...string) error {
	seen := map[string]bool{}
	for _, f := range felhasznaloIDs {
		if f == "" || seen[f] {
			continue
		}
		seen[f] = true
		n, err := q.FindGoogleNaptar(ctx, f)
		if errors.Is(err, store.ErrNotFound) {
			continue
		}
		if err != nil {
			return err
		}
		if n.MegszakadtAt != nil {
			continue
		}
		kerelem := randomID()
		if err := q.RequestGoogleNaptarSync(ctx, f, esemenyID, kerelem); err != nil {
			return err
		}
		pending, err := q.HasPendingFelhasznaloJob(ctx, string(jobGcalSync), esemenyID, f)
		if err != nil {
			return err
		}
		if pending {
			continue
		}
		if err := a.scheduleGcalSync(ctx, q, esemenyID, f, kerelem); err != nil {
			return err
		}
	}
	return nil
}

func (a *App) scheduleGcalSync(ctx context.Context, q store.Queries, esemenyID, felhasznaloID, kerelem string) error {
	return a.scheduleJob(ctx, q, jobGcalSync, esemenyID, a.clock.Now(), gcalSyncPayload{Felhasznalo: felhasznaloID, Kerelem: kerelem})
}

// syncNaptarOfEsemeny asks for the Esemeny to be synced for everyone it may
// concern: its Resztvevok, and whoever has its event (or a pending sync).
// Call it in the transaction of a change of its state or details, after
// the change.
func (a *App) syncNaptarOfEsemeny(ctx context.Context, q store.Queries, esemenyID string) error {
	e, err := q.FindEsemeny(ctx, esemenyID)
	if err != nil {
		return err
	}
	visszajelzesek, err := q.ListVisszajelzesekOfEsemeny(ctx, esemenyID)
	if err != nil {
		return err
	}
	ids, err := q.ListGoogleNaptarFelhasznalok(ctx, esemenyID)
	if err != nil {
		return err
	}
	for _, r := range resztvevokOf(e, visszajelzesek) {
		ids = append(ids, r.FelhasznaloID)
	}
	return a.requestNaptarSync(ctx, q, esemenyID, ids...)
}

// syncNaptarOfTagLeft asks for the Csoport's Esemenyek to be synced for a
// Tag who left it (or was removed), after the Tag is gone.
func (a *App) syncNaptarOfTagLeft(ctx context.Context, q store.Queries, csoportID, felhasznaloID string) error {
	ids, err := q.ListGoogleNaptarEsemenyek(ctx, felhasznaloID, csoportID)
	if err != nil {
		return err
	}
	for _, id := range ids {
		if err := a.requestNaptarSync(ctx, q, id, felhasznaloID); err != nil {
			return err
		}
	}
	return nil
}

// backfillNaptar asks for every Esemeny that may concern the Felhasznalo to
// be synced: those they answered igen to the confirmed Idopont of, and
// those they have an event of.
func (a *App) backfillNaptar(ctx context.Context, q store.Queries, felhasznaloID string) error {
	ids, err := q.ListConfirmedIgenEsemenyek(ctx, felhasznaloID)
	if err != nil {
		return err
	}
	known, err := q.ListGoogleNaptarEsemenyek(ctx, felhasznaloID, "")
	if err != nil {
		return err
	}
	for _, id := range known {
		if !slices.Contains(ids, id) {
			ids = append(ids, id)
		}
	}
	for _, id := range ids {
		if err := a.requestNaptarSync(ctx, q, id, felhasznaloID); err != nil {
			return err
		}
	}
	return nil
}

// --- The sync job ---

// runGcalSync reconciles one pair: it decides, in the job's transaction,
// which Google call (if any) brings the calendar in line with the rule.
func (a *App) runGcalSync(ctx context.Context, q store.Queries, j store.Job) (durableCall, error) {
	var p gcalSyncPayload
	if err := jobPayload(j, &p); err != nil {
		return nil, err
	}
	f, esemenyID := p.Felhasznalo, j.EsemenyID
	conn, err := q.FindGoogleNaptar(ctx, f)
	if errors.Is(err, store.ErrNotFound) {
		return nil, nil // disconnected
	}
	if err != nil {
		return nil, err
	}
	if conn.MegszakadtAt != nil {
		return nil, nil // dropped until they connect again
	}
	row, err := q.FindGoogleNaptarEsemeny(ctx, f, esemenyID)
	if errors.Is(err, store.ErrNotFound) {
		row = store.GoogleNaptarEsemeny{FelhasznaloID: f, EsemenyID: esemenyID, Kerelem: randomID()}
		if err := q.RequestGoogleNaptarSync(ctx, f, esemenyID, row.Kerelem); err != nil {
			return nil, err
		}
	} else if err != nil {
		return nil, err
	}

	event, want, err := a.desiredCalendarEvent(ctx, q, esemenyID, f)
	if err != nil {
		return nil, err
	}
	run := gcalSync{a: a, conn: conn, esemenyID: esemenyID, kerelem: row.Kerelem}
	switch {
	case want && conn.CalendarID == "":
		return run.createCalendar, nil
	case want:
		tartalom := contentHash(event)
		if row.EventID == event.ID && row.Tartalom == tartalom {
			return nil, nil // already in the calendar as it should be
		}
		return run.upsert(event, tartalom), nil
	case row.EventID != "" && conn.CalendarID != "":
		return run.delete(row.EventID), nil
	default:
		// Nothing in Google, and nothing should be.
		if err := q.SetGoogleNaptarEvent(ctx, f, esemenyID, "", "", row.Kerelem); err != nil {
			return nil, err
		}
		return nil, q.DeleteIdleGoogleNaptarEsemeny(ctx, f, esemenyID, row.Kerelem)
	}
}

// gcalSync is one run of a gcal_sync job: the connection and the pair's
// kerelem as the job read them.
type gcalSync struct {
	a         *App
	conn      store.GoogleNaptar
	esemenyID string
	kerelem   string
}

// createCalendar creates the "Squad Sync" calendar; the job it schedules
// then writes the event.
func (s gcalSync) createCalendar(ctx context.Context) (func(context.Context, store.Queries) error, error) {
	cctx, cancel := context.WithTimeout(ctx, googleCallTimeout)
	id, err := s.a.calendar.CreateCalendar(cctx, s.conn.RefreshToken, googleNaptarName, googleNaptarTimeZone)
	cancel()
	if errors.Is(err, gateway.ErrGoogleTokenRevoked) {
		return s.markBroken, nil
	}
	if err != nil {
		return nil, err
	}
	return func(ctx context.Context, q store.Queries) error {
		stored, err := q.SetGoogleNaptarCalendar(ctx, s.conn.FelhasznaloID, s.conn.KapcsolatID, id)
		if err != nil {
			return err
		}
		if !stored {
			// Disconnected meanwhile, or another runner created one first.
			s.a.log.Warn("google calendar created but not used", "felhasznalo", s.conn.FelhasznaloID, "calendar", id)
			return s.followUp(ctx, q)
		}
		// A new kerelem, so the follow-up below writes the event.
		if err := q.RequestGoogleNaptarSync(ctx, s.conn.FelhasznaloID, s.esemenyID, randomID()); err != nil {
			return err
		}
		return s.followUp(ctx, q)
	}, nil
}

// upsert writes the event.
func (s gcalSync) upsert(event gateway.CalendarEvent, tartalom string) durableCall {
	return func(ctx context.Context) (func(context.Context, store.Queries) error, error) {
		cctx, cancel := context.WithTimeout(ctx, googleCallTimeout)
		err := s.a.calendar.UpsertEvent(cctx, s.conn.RefreshToken, s.conn.CalendarID, event)
		cancel()
		switch {
		case errors.Is(err, gateway.ErrGoogleTokenRevoked):
			return s.markBroken, nil
		case errors.Is(err, gateway.ErrGoogleCalendarGone):
			return s.calendarGone, nil
		case err != nil:
			return nil, err
		}
		return func(ctx context.Context, q store.Queries) error {
			same, err := s.sameConnection(ctx, q)
			if err != nil {
				return err
			}
			if same {
				if err := q.SetGoogleNaptarEvent(ctx, s.conn.FelhasznaloID, s.esemenyID, event.ID, tartalom, s.kerelem); err != nil {
					return err
				}
			}
			return s.followUp(ctx, q)
		}, nil
	}
}

// delete removes the event.
func (s gcalSync) delete(eventID string) durableCall {
	return func(ctx context.Context) (func(context.Context, store.Queries) error, error) {
		cctx, cancel := context.WithTimeout(ctx, googleCallTimeout)
		err := s.a.calendar.DeleteEvent(cctx, s.conn.RefreshToken, s.conn.CalendarID, eventID)
		cancel()
		switch {
		case errors.Is(err, gateway.ErrGoogleTokenRevoked):
			return s.markBroken, nil
		case errors.Is(err, gateway.ErrGoogleCalendarGone):
			// The event went with its calendar.
		case err != nil:
			return nil, err
		}
		return func(ctx context.Context, q store.Queries) error {
			same, err := s.sameConnection(ctx, q)
			if err != nil {
				return err
			}
			if !same {
				return s.followUp(ctx, q)
			}
			if err := q.SetGoogleNaptarEvent(ctx, s.conn.FelhasznaloID, s.esemenyID, "", "", s.kerelem); err != nil {
				return err
			}
			if err := s.followUp(ctx, q); err != nil {
				return err
			}
			return q.DeleteIdleGoogleNaptarEsemeny(ctx, s.conn.FelhasznaloID, s.esemenyID, s.kerelem)
		}, nil
	}
}

// markBroken records that Google refused the token. Pending jobs of the
// Felhasznalo then do nothing.
func (s gcalSync) markBroken(ctx context.Context, q store.Queries) error {
	s.a.log.Warn("google refused the token; the connection is broken", "felhasznalo", s.conn.FelhasznaloID)
	if err := q.MarkGoogleNaptarBroken(ctx, s.conn.FelhasznaloID, s.conn.KapcsolatID, s.a.clock.Now()); err != nil {
		return err
	}
	// Connected again meanwhile: the follow-up syncs under the new one.
	return s.followUp(ctx, q)
}

// calendarGone forgets the calendar, which was deleted in Google (or the
// token's account can't see it), with every event in it, and syncs
// everything again into a new one.
func (s gcalSync) calendarGone(ctx context.Context, q store.Queries) error {
	forgotten, err := q.ForgetGoogleNaptarCalendar(ctx, s.conn.FelhasznaloID, s.conn.KapcsolatID, s.conn.CalendarID)
	if err != nil {
		return err
	}
	if !forgotten {
		return s.followUp(ctx, q)
	}
	s.a.log.Warn("google calendar is gone; creating a new one", "felhasznalo", s.conn.FelhasznaloID, "calendar", s.conn.CalendarID)
	if err := s.a.backfillNaptar(ctx, q, s.conn.FelhasznaloID); err != nil {
		return err
	}
	return s.followUp(ctx, q)
}

// sameConnection tells whether the connection is still the one the job
// read, and working, so its result may be stored.
func (s gcalSync) sameConnection(ctx context.Context, q store.Queries) (bool, error) {
	n, err := q.FindGoogleNaptar(ctx, s.conn.FelhasznaloID)
	if errors.Is(err, store.ErrNotFound) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return n.KapcsolatID == s.conn.KapcsolatID && n.CalendarID == s.conn.CalendarID && n.MegszakadtAt == nil, nil
}

// followUp schedules another job for the pair if it was asked to be synced
// again while this job called Google.
func (s gcalSync) followUp(ctx context.Context, q store.Queries) error {
	row, err := q.FindGoogleNaptarEsemeny(ctx, s.conn.FelhasznaloID, s.esemenyID)
	if errors.Is(err, store.ErrNotFound) {
		return nil
	}
	if err != nil || row.Kerelem == s.kerelem {
		return err
	}
	return s.a.scheduleGcalSync(ctx, q, s.esemenyID, s.conn.FelhasznaloID, row.Kerelem)
}

// --- The event ---

// desiredCalendarEvent is the event the rule wants in the Felhasznalo's
// calendar for the Esemeny, if any.
func (a *App) desiredCalendarEvent(ctx context.Context, q store.Queries, esemenyID, felhasznaloID string) (gateway.CalendarEvent, bool, error) {
	e, err := q.FindEsemeny(ctx, esemenyID)
	if errors.Is(err, store.ErrNotFound) {
		return gateway.CalendarEvent{}, false, nil
	}
	if err != nil {
		return gateway.CalendarEvent{}, false, err
	}
	confirmedID, ok := confirmedIdopont(e)
	if !ok {
		return gateway.CalendarEvent{}, false, nil
	}
	visszajelzesek, err := q.ListVisszajelzesekOfEsemeny(ctx, esemenyID)
	if err != nil {
		return gateway.CalendarEvent{}, false, err
	}
	if !slices.ContainsFunc(resztvevokOf(e, visszajelzesek), func(r domain.Resztvevo) bool {
		return r.FelhasznaloID == felhasznaloID
	}) {
		return gateway.CalendarEvent{}, false, nil
	}
	i, err := q.FindIdopont(ctx, confirmedID)
	if err != nil {
		return gateway.CalendarEvent{}, false, err
	}
	return a.calendarEvent(e, i, felhasznaloID), true, nil
}

// calendarEvent is the Esemeny's event in the Felhasznalo's calendar.
func (a *App) calendarEvent(e domain.Esemeny, i domain.Idopont, felhasznaloID string) gateway.CalendarEvent {
	end := i.Start.Add(openEndedLength)
	if i.End != nil {
		end = *i.End
	}
	link := a.esemenyURL(e.ID)
	var description []string
	if e.Description != "" {
		description = append(description, e.Description)
	}
	if e.Helyszin.URL != "" {
		description = append(description, "Helyszín: "+e.Helyszin.URL)
	}
	description = append(description, "Squad Sync: "+link)
	location := e.Helyszin.Name
	if location == "" {
		location = e.Helyszin.URL
	}
	return gateway.CalendarEvent{
		ID:          calendarEventID(felhasznaloID, e.ID),
		Title:       e.Title,
		Description: strings.Join(description, "\n\n"),
		Location:    location,
		Start:       i.Start,
		End:         end,
		AllDay:      i.EgeszNapos,
		Link:        link,
	}
}

// calendarEventID derives the pair's Google event id: base32hex (Google's
// alphabet for event ids), lowercase.
func calendarEventID(felhasznaloID, esemenyID string) string {
	sum := sha256.Sum256([]byte(felhasznaloID + "\x00" + esemenyID))
	id := base32.HexEncoding.WithPadding(base32.NoPadding).EncodeToString(sum[:20])
	return "ss" + strings.ToLower(id)
}

// contentHash fingerprints an event's content.
func contentHash(e gateway.CalendarEvent) string {
	body, _ := json.Marshal(e) // a struct of strings, times and a bool
	sum := sha256.Sum256(body)
	return hex.EncodeToString(sum[:])
}
