package store

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

// GoogleNaptar is a Felhasznalo's connection to Google Calendar (ADR-0005).
type GoogleNaptar struct {
	FelhasznaloID string
	// KapcsolatID is new on every connect.
	KapcsolatID  string
	RefreshToken string
	// CalendarID is the "Squad Sync" calendar, "" until it is created.
	CalendarID string
	// MegszakadtAt is set once Google refused the token.
	MegszakadtAt *time.Time
	CreatedAt    time.Time
}

// GoogleNaptarEsemeny is what the sync knows of one (Felhasznalo, Esemeny)
// pair.
type GoogleNaptarEsemeny struct {
	FelhasznaloID string
	EsemenyID     string
	// EventID is the Google event, "" while there is none.
	EventID string
	// Tartalom is a hash of the event's content as last written.
	Tartalom string
	// Kerelem is the latest request to sync the pair.
	Kerelem string
}

// UpsertGoogleNaptar stores the Felhasznalo's connection, replacing any
// earlier one.
func (q Queries) UpsertGoogleNaptar(ctx context.Context, n GoogleNaptar) error {
	var calendar any
	if n.CalendarID != "" {
		calendar = n.CalendarID
	}
	_, err := q.db.ExecContext(ctx,
		`INSERT INTO google_naptar (felhasznalo_id, kapcsolat_id, refresh_token, calendar_id, megszakadt_at, created_at)
		 VALUES (?, ?, ?, ?, NULL, ?)
		 ON CONFLICT (felhasznalo_id) DO UPDATE SET
		   kapcsolat_id = excluded.kapcsolat_id, refresh_token = excluded.refresh_token,
		   calendar_id = excluded.calendar_id, megszakadt_at = NULL, created_at = excluded.created_at`,
		n.FelhasznaloID, n.KapcsolatID, n.RefreshToken, calendar, formatTime(n.CreatedAt))
	if err != nil {
		return fmt.Errorf("upsert google naptar: %w", err)
	}
	return nil
}

// FindGoogleNaptar returns the Felhasznalo's connection, or ErrNotFound.
func (q Queries) FindGoogleNaptar(ctx context.Context, felhasznaloID string) (GoogleNaptar, error) {
	var n GoogleNaptar
	var calendar, megszakadt sql.NullString
	var createdAt string
	err := q.db.QueryRowContext(ctx,
		`SELECT felhasznalo_id, kapcsolat_id, refresh_token, calendar_id, megszakadt_at, created_at
		 FROM google_naptar WHERE felhasznalo_id = ?`, felhasznaloID,
	).Scan(&n.FelhasznaloID, &n.KapcsolatID, &n.RefreshToken, &calendar, &megszakadt, &createdAt)
	if isNoRows(err) {
		return GoogleNaptar{}, ErrNotFound
	}
	if err != nil {
		return GoogleNaptar{}, fmt.Errorf("find google naptar: %w", err)
	}
	n.CalendarID = calendar.String
	if n.MegszakadtAt, err = parseNullTime(megszakadt); err != nil {
		return GoogleNaptar{}, err
	}
	if n.CreatedAt, err = parseTime(createdAt); err != nil {
		return GoogleNaptar{}, err
	}
	return n, nil
}

// DeleteGoogleNaptar removes the Felhasznalo's connection and everything
// the sync knew of their events.
func (q Queries) DeleteGoogleNaptar(ctx context.Context, felhasznaloID string) error {
	if _, err := q.db.ExecContext(ctx, `DELETE FROM google_naptar_esemeny WHERE felhasznalo_id = ?`, felhasznaloID); err != nil {
		return fmt.Errorf("delete google naptar esemenyek: %w", err)
	}
	if _, err := q.db.ExecContext(ctx, `DELETE FROM google_naptar WHERE felhasznalo_id = ?`, felhasznaloID); err != nil {
		return fmt.Errorf("delete google naptar: %w", err)
	}
	return nil
}

// SetGoogleNaptarCalendar stores the newly created calendar of the
// connection, if it is still the same connection and has none yet. It
// reports whether it did.
func (q Queries) SetGoogleNaptarCalendar(ctx context.Context, felhasznaloID, kapcsolatID, calendarID string) (bool, error) {
	res, err := q.db.ExecContext(ctx,
		`UPDATE google_naptar SET calendar_id = ?
		 WHERE felhasznalo_id = ? AND kapcsolat_id = ? AND calendar_id IS NULL`,
		calendarID, felhasznaloID, kapcsolatID)
	if err != nil {
		return false, fmt.Errorf("set google naptar calendar: %w", err)
	}
	n, err := res.RowsAffected()
	return n > 0, err
}

// MarkGoogleNaptarBroken records that Google refused the connection's token,
// if it is still the same connection.
func (q Queries) MarkGoogleNaptarBroken(ctx context.Context, felhasznaloID, kapcsolatID string, at time.Time) error {
	_, err := q.db.ExecContext(ctx,
		`UPDATE google_naptar SET megszakadt_at = ?
		 WHERE felhasznalo_id = ? AND kapcsolat_id = ? AND megszakadt_at IS NULL`,
		formatTime(at), felhasznaloID, kapcsolatID)
	if err != nil {
		return fmt.Errorf("mark google naptar broken: %w", err)
	}
	return nil
}

// ForgetGoogleNaptarCalendar clears the connection's calendar, which no
// longer exists in Google, and every event the sync knew in it. It reports
// whether it did (the connection and its calendar were still these).
func (q Queries) ForgetGoogleNaptarCalendar(ctx context.Context, felhasznaloID, kapcsolatID, calendarID string) (bool, error) {
	res, err := q.db.ExecContext(ctx,
		`UPDATE google_naptar SET calendar_id = NULL
		 WHERE felhasznalo_id = ? AND kapcsolat_id = ? AND calendar_id = ?`,
		felhasznaloID, kapcsolatID, calendarID)
	if err != nil {
		return false, fmt.Errorf("forget google naptar calendar: %w", err)
	}
	if n, err := res.RowsAffected(); err != nil || n == 0 {
		return false, err
	}
	if _, err := q.db.ExecContext(ctx,
		`UPDATE google_naptar_esemeny SET event_id = NULL, tartalom = '' WHERE felhasznalo_id = ?`, felhasznaloID); err != nil {
		return false, fmt.Errorf("forget google naptar events: %w", err)
	}
	return true, nil
}

// FindGoogleNaptarEsemeny returns what the sync knows of the pair, or
// ErrNotFound.
func (q Queries) FindGoogleNaptarEsemeny(ctx context.Context, felhasznaloID, esemenyID string) (GoogleNaptarEsemeny, error) {
	r := GoogleNaptarEsemeny{FelhasznaloID: felhasznaloID, EsemenyID: esemenyID}
	var eventID sql.NullString
	err := q.db.QueryRowContext(ctx,
		`SELECT event_id, tartalom, kerelem FROM google_naptar_esemeny WHERE felhasznalo_id = ? AND esemeny_id = ?`,
		felhasznaloID, esemenyID,
	).Scan(&eventID, &r.Tartalom, &r.Kerelem)
	if isNoRows(err) {
		return GoogleNaptarEsemeny{}, ErrNotFound
	}
	if err != nil {
		return GoogleNaptarEsemeny{}, fmt.Errorf("find google naptar esemeny: %w", err)
	}
	r.EventID = eventID.String
	return r, nil
}

// RequestGoogleNaptarSync records a new request to sync the pair.
func (q Queries) RequestGoogleNaptarSync(ctx context.Context, felhasznaloID, esemenyID, kerelem string) error {
	_, err := q.db.ExecContext(ctx,
		`INSERT INTO google_naptar_esemeny (felhasznalo_id, esemeny_id, kerelem) VALUES (?, ?, ?)
		 ON CONFLICT (felhasznalo_id, esemeny_id) DO UPDATE SET kerelem = excluded.kerelem`,
		felhasznaloID, esemenyID, kerelem)
	if err != nil {
		return fmt.Errorf("request google naptar sync: %w", err)
	}
	return nil
}

// SetGoogleNaptarEvent records the pair's event as written ("" eventID for
// none). A pair without a row gets one with the given kerelem.
func (q Queries) SetGoogleNaptarEvent(ctx context.Context, felhasznaloID, esemenyID, eventID, tartalom, kerelem string) error {
	var id any
	if eventID != "" {
		id = eventID
	}
	_, err := q.db.ExecContext(ctx,
		`INSERT INTO google_naptar_esemeny (felhasznalo_id, esemeny_id, event_id, tartalom, kerelem) VALUES (?, ?, ?, ?, ?)
		 ON CONFLICT (felhasznalo_id, esemeny_id) DO UPDATE SET event_id = excluded.event_id, tartalom = excluded.tartalom`,
		felhasznaloID, esemenyID, id, tartalom, kerelem)
	if err != nil {
		return fmt.Errorf("set google naptar event: %w", err)
	}
	return nil
}

// DeleteIdleGoogleNaptarEsemeny removes the pair's row if it has no event
// and its latest request is still kerelem.
func (q Queries) DeleteIdleGoogleNaptarEsemeny(ctx context.Context, felhasznaloID, esemenyID, kerelem string) error {
	_, err := q.db.ExecContext(ctx,
		`DELETE FROM google_naptar_esemeny
		 WHERE felhasznalo_id = ? AND esemeny_id = ? AND event_id IS NULL AND kerelem = ?`,
		felhasznaloID, esemenyID, kerelem)
	if err != nil {
		return fmt.Errorf("delete google naptar esemeny: %w", err)
	}
	return nil
}

// ListGoogleNaptarFelhasznalok returns the Felhasznalok the sync knows an
// event of, or a pending sync of, for the Esemeny.
func (q Queries) ListGoogleNaptarFelhasznalok(ctx context.Context, esemenyID string) ([]string, error) {
	return q.listStrings(ctx, "list google naptar felhasznalok",
		`SELECT felhasznalo_id FROM google_naptar_esemeny WHERE esemeny_id = ? ORDER BY felhasznalo_id`, esemenyID)
}

// ListGoogleNaptarEsemenyek returns the Esemenyek, of one Csoport or of
// every Csoport when csoportID is "", that the sync knows an event or a
// pending sync of for the Felhasznalo.
func (q Queries) ListGoogleNaptarEsemenyek(ctx context.Context, felhasznaloID, csoportID string) ([]string, error) {
	return q.listStrings(ctx, "list google naptar esemenyek",
		`SELECT g.esemeny_id FROM google_naptar_esemeny g JOIN esemeny e ON e.id = g.esemeny_id
		 WHERE g.felhasznalo_id = ? AND (? = '' OR e.csoport_id = ?) ORDER BY g.esemeny_id`,
		felhasznaloID, csoportID, csoportID)
}

// ListConfirmedIgenEsemenyek returns the Veglegesitve and Lezajlott
// Esemenyek where the Felhasznalo answered igen to the confirmed Idopont:
// those they may be a Resztvevo of.
func (q Queries) ListConfirmedIgenEsemenyek(ctx context.Context, felhasznaloID string) ([]string, error) {
	return q.listStrings(ctx, "list confirmed igen esemenyek",
		`SELECT e.id FROM esemeny e
		 JOIN visszajelzes v ON v.idopont_id = e.confirmed_idopont_id
		 WHERE v.felhasznalo_id = ? AND v.valasz = 'igen' AND e.allapot IN ('veglegesitve', 'lezajlott')
		 ORDER BY e.id`, felhasznaloID)
}

// HasPendingFelhasznaloJob tells whether the Esemeny has a pending job of
// the kind whose payload names the Felhasznalo (as "felhasznalo").
func (q Queries) HasPendingFelhasznaloJob(ctx context.Context, kind, esemenyID, felhasznaloID string) (bool, error) {
	var found bool
	err := q.db.QueryRowContext(ctx,
		`SELECT EXISTS (SELECT 1 FROM job WHERE kind = ? AND esemeny_id = ? AND done_at IS NULL
		   AND json_extract(payload, '$.felhasznalo') = ?)`,
		kind, esemenyID, felhasznaloID).Scan(&found)
	if err != nil {
		return false, fmt.Errorf("find pending job: %w", err)
	}
	return found, nil
}

func (q Queries) listStrings(ctx context.Context, what, query string, args ...any) ([]string, error) {
	rows, err := q.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", what, err)
	}
	defer rows.Close()
	out := []string{}
	for rows.Next() {
		var s string
		if err := rows.Scan(&s); err != nil {
			return nil, fmt.Errorf("%s: %w", what, err)
		}
		out = append(out, s)
	}
	return out, rows.Err()
}
