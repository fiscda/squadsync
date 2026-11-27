package app

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"
	_ "time/tzdata" // Europe/Budapest must load even without system zone data
	"unicode/utf8"

	"squadsync-be/internal/domain"
	"squadsync-be/internal/gateway"
	"squadsync-be/internal/store"
)

// Limits on what an Esemeny holds. They also keep its Discord message
// within Discord's 6000-character total for an embed: a 100-character title,
// a 2000-character description, a 200-character Helyszin and 20 Idopont
// fields of about 100 characters each stay well under it.
const (
	MaxEsemenyTitleLength       = 100
	MaxEsemenyDescriptionLength = 2000
	MaxHelyszinNameLength       = 200
	MaxHelyszinURLLength        = 2000
	MaxIdopontok                = 20
)

// LocalTimeLayout is how Idopont times arrive from the entry points: a wall
// clock time in Europe/Budapest, to the minute.
const LocalTimeLayout = "2006-01-02T15:04"

// LocalDateLayout is how a whole-day Idopont's days arrive: a Budapest date.
const LocalDateLayout = "2006-01-02"

// Budapest is the one time zone of the app. Times are stored in UTC and
// shown in Budapest time; there are no per-user time zones.
var Budapest = mustLoadLocation("Europe/Budapest")

// ErrIdopontNotAnswerable means the Idopont can't be answered in the
// Esemeny's current state (see answerable).
var ErrIdopontNotAnswerable = errors.New("idopont is not answerable")

// ErrNotSzervezo means the acting Felhasznalo is a Tag of the Esemeny's
// Csoport but not its Szervezo, and the action is the Szervezo's alone.
var ErrNotSzervezo = errors.New("not the szervezo of the esemeny")

// ErrInvalidTransition means the Esemeny's state doesn't allow the change,
// such as confirming an Elmaradt Esemeny.
var ErrInvalidTransition = errors.New("invalid esemeny state transition")

// ErrNonexistentLocalTime means a wall clock time that never happens in
// Budapest, because the clocks jump forward over it (the spring DST change).
// It wraps ErrInvalidInput.
var ErrNonexistentLocalTime = fmt.Errorf("%w: nem létező időpont az óraátállítás miatt", ErrInvalidInput)

// EsemenyInput is a proposed Esemeny.
type EsemenyInput struct {
	Title       string
	Description string
	Helyszin    domain.Helyszin
	Idopontok   []IdopontInput
}

// IdopontInput is one candidate time.
//
// Normally Start and End are Budapest wall clock times in LocalTimeLayout.
// End is optional ("" for none) and must be after Start; it may be days
// later.
//
// With EgeszNapos, Start and End are Budapest dates in LocalDateLayout: the
// first and the last day, both included. End is optional ("" for a single
// day) and must not be before Start.
type IdopontInput struct {
	Start      string
	End        string
	EgeszNapos bool
}

// EsemenyDetail is an Esemeny as one of its Csoport's Tags sees it.
type EsemenyDetail struct {
	domain.Esemeny
	CsoportName  string
	SzervezoName string
	// Idopontok are in chronological order.
	Idopontok []IdopontAllas
	// ConfirmedIdopont is the confirmed Idopont's id while the Esemeny has
	// Resztvevok (Veglegesitve, Lezajlott), "" otherwise. Use it rather
	// than the stored Esemeny.ConfirmedIdopontID.
	ConfirmedIdopont string
	// Resztvevok are ordered by name; empty unless the Esemeny is
	// Veglegesitve or Lezajlott (see resztvevokOf).
	Resztvevok []domain.Resztvevo
}

// IdopontAllas is an Idopont with how it stands: its counts and Helyezes
// (both counting only Tags' Visszajelzesek), the viewer's own answer, and
// every Visszajelzes on it.
type IdopontAllas struct {
	domain.Idopont
	Igen, Talan, Nem int
	// Helyezes is 0 when no Tag has answered the Idopont.
	Helyezes Helyezes
	// Sajat is the viewer's own Visszajelzes, "" if they haven't answered.
	Sajat domain.Valasz
	// Visszajelzesek are ordered by name.
	Visszajelzesek []domain.VisszajelzesOfEsemeny
	// Answerable tells whether it takes Visszajelzesek now (see answerable).
	Answerable bool
	// Confirmed marks the Esemeny's confirmed Idopont while it is
	// Veglegesitve or Lezajlott (not once Elmaradt).
	Confirmed bool
}

// ProposeEsemeny creates an Esemeny in the Csoport with its Idopontok. The
// proposer becomes its Szervezo and it starts in Egyeztetes. Only Tags may
// propose; anyone else gets ErrNotFound. Malformed input gives
// ErrInvalidInput.
func (a *App) ProposeEsemeny(ctx context.Context, felhasznaloID, csoportID string, in EsemenyInput) (domain.Esemeny, error) {
	e, idopontok, err := a.newEsemeny(felhasznaloID, csoportID, in)
	if err != nil {
		return domain.Esemeny{}, err
	}
	err = a.store.InTx(ctx, func(q store.Queries) error {
		if _, err := requireTag(ctx, q, csoportID, felhasznaloID); err != nil {
			return err
		}
		if err := q.InsertEsemeny(ctx, e); err != nil {
			return err
		}
		for _, i := range idopontok {
			if err := q.InsertIdopont(ctx, i); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return domain.Esemeny{}, err
	}
	a.afterCommit(func(ctx context.Context) { a.announceEsemeny(ctx, e.ID) })
	return e, nil
}

// newEsemeny validates a proposal and builds its rows.
func (a *App) newEsemeny(felhasznaloID, csoportID string, in EsemenyInput) (domain.Esemeny, []domain.Idopont, error) {
	title := strings.TrimSpace(in.Title)
	if title == "" {
		return domain.Esemeny{}, nil, fmt.Errorf("%w: an esemeny needs a title", ErrInvalidInput)
	}
	if utf8.RuneCountInString(title) > MaxEsemenyTitleLength {
		return domain.Esemeny{}, nil, fmt.Errorf("%w: a title is at most %d characters", ErrInvalidInput, MaxEsemenyTitleLength)
	}
	description := strings.TrimSpace(in.Description)
	if utf8.RuneCountInString(description) > MaxEsemenyDescriptionLength {
		return domain.Esemeny{}, nil, fmt.Errorf("%w: a description is at most %d characters", ErrInvalidInput, MaxEsemenyDescriptionLength)
	}
	helyszin, err := validHelyszin(in.Helyszin)
	if err != nil {
		return domain.Esemeny{}, nil, err
	}
	if len(in.Idopontok) == 0 {
		return domain.Esemeny{}, nil, fmt.Errorf("%w: an esemeny needs at least one idopont", ErrInvalidInput)
	}
	if len(in.Idopontok) > MaxIdopontok {
		return domain.Esemeny{}, nil, fmt.Errorf("%w: an esemeny has at most %d idopontok", ErrInvalidInput, MaxIdopontok)
	}

	now := a.clock.Now()
	e := domain.Esemeny{
		ID:          randomID(),
		CsoportID:   csoportID,
		Title:       title,
		Description: description,
		Helyszin:    helyszin,
		SzervezoID:  felhasznaloID,
		Allapot:     domain.Egyeztetes,
		CreatedAt:   now,
	}
	idopontok := make([]domain.Idopont, 0, len(in.Idopontok))
	for n, input := range in.Idopontok {
		i, err := newIdopont(input)
		if err != nil {
			return domain.Esemeny{}, nil, fmt.Errorf("idopont %d: %w", n+1, err)
		}
		i.ID = randomID()
		i.EsemenyID = e.ID
		i.AddedBy = felhasznaloID
		i.CreatedAt = now
		idopontok = append(idopontok, i)
	}
	return e, idopontok, nil
}

func validHelyszin(h domain.Helyszin) (domain.Helyszin, error) {
	h.Name = strings.TrimSpace(h.Name)
	h.URL = strings.TrimSpace(h.URL)
	if utf8.RuneCountInString(h.Name) > MaxHelyszinNameLength {
		return domain.Helyszin{}, fmt.Errorf("%w: a helyszin name is at most %d characters", ErrInvalidInput, MaxHelyszinNameLength)
	}
	if h.URL == "" {
		return h, nil
	}
	// Only http and https: the URL becomes a link on the web, and other
	// schemes such as javascript: would run in the viewer's browser.
	if len(h.URL) > MaxHelyszinURLLength {
		return domain.Helyszin{}, fmt.Errorf("%w: a helyszin URL is at most %d characters", ErrInvalidInput, MaxHelyszinURLLength)
	}
	u, err := url.Parse(h.URL)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return domain.Helyszin{}, fmt.Errorf("%w: the helyszin URL must be an absolute http or https URL", ErrInvalidInput)
	}
	return h, nil
}

// newIdopont converts an Idopont's Budapest wall clock times to UTC.
func newIdopont(in IdopontInput) (domain.Idopont, error) {
	if in.EgeszNapos {
		return newEgeszNaposIdopont(in)
	}
	start, err := parseLocalTime(in.Start)
	if err != nil {
		return domain.Idopont{}, err
	}
	i := domain.Idopont{Start: start}
	if strings.TrimSpace(in.End) == "" {
		return i, nil
	}
	end, err := parseLocalTime(in.End)
	if err != nil {
		return domain.Idopont{}, err
	}
	if !end.After(start) {
		return domain.Idopont{}, fmt.Errorf("%w: the end must be after the start", ErrInvalidInput)
	}
	i.End = &end
	return i, nil
}

// newEgeszNaposIdopont turns a whole-day Idopont's first and last day into
// Budapest midnight of the first day and Budapest midnight after the last.
func newEgeszNaposIdopont(in IdopontInput) (domain.Idopont, error) {
	first, err := parseLocalDate(in.Start)
	if err != nil {
		return domain.Idopont{}, err
	}
	last := first
	if strings.TrimSpace(in.End) != "" {
		if last, err = parseLocalDate(in.End); err != nil {
			return domain.Idopont{}, err
		}
	}
	if last.Before(first) {
		return domain.Idopont{}, fmt.Errorf("%w: the last day must not be before the first", ErrInvalidInput)
	}
	// AddDate on the Budapest date keeps midnight across a DST change.
	end := last.AddDate(0, 0, 1).UTC()
	return domain.Idopont{Start: first.UTC(), End: &end, EgeszNapos: true}, nil
}

// parseLocalTime reads a Budapest wall clock time in LocalTimeLayout and
// returns it in UTC. A time skipped by the spring DST change is refused; a
// time that happens twice in autumn means its first occurrence.
func parseLocalTime(s string) (time.Time, error) {
	s = strings.TrimSpace(s)
	t, err := time.ParseInLocation(LocalTimeLayout, s, Budapest)
	if err != nil {
		return time.Time{}, fmt.Errorf("%w: a time must look like 2026-12-05T20:00", ErrInvalidInput)
	}
	if t.Format(LocalTimeLayout) != s {
		return time.Time{}, fmt.Errorf("%w: %s", ErrNonexistentLocalTime, s)
	}
	// Go resolves an autumn overlap to its second occurrence; take the first
	// (still summer time), one DST shift earlier, when it reads the same.
	if earlier := t.Add(-time.Hour); earlier.In(Budapest).Format(LocalTimeLayout) == s {
		t = earlier
	}
	return t.UTC(), nil
}

// parseLocalDate reads a date in LocalDateLayout and returns Budapest
// midnight of that day, in Budapest time.
func parseLocalDate(s string) (time.Time, error) {
	t, err := time.ParseInLocation(LocalDateLayout, strings.TrimSpace(s), Budapest)
	if err != nil {
		return time.Time{}, fmt.Errorf("%w: a day must look like 2026-12-05", ErrInvalidInput)
	}
	return t, nil
}

// EsemenyekOfCsoport lists a Csoport's Esemenyek, newest first. Only Tags
// may see them; anyone else gets ErrNotFound.
func (a *App) EsemenyekOfCsoport(ctx context.Context, felhasznaloID, csoportID string) ([]domain.EsemenySummary, error) {
	if _, err := requireTag(ctx, a.store.Queries, csoportID, felhasznaloID); err != nil {
		return nil, err
	}
	return a.store.ListEsemenyekOfCsoport(ctx, csoportID)
}

// Esemeny returns an Esemeny with its Idopontok, their counts and Helyezes,
// and the viewer's own Visszajelzesek. Only Tags of its Csoport may see it;
// anyone else gets ErrNotFound.
func (a *App) Esemeny(ctx context.Context, felhasznaloID, esemenyID string) (EsemenyDetail, error) {
	q := a.store.Queries
	e, err := requireEsemenyTag(ctx, q, esemenyID, felhasznaloID)
	if err != nil {
		return EsemenyDetail{}, err
	}
	return esemenyDetail(ctx, q, e, felhasznaloID)
}

// esemenyDetail reads what the Esemeny's page shows to viewerID ("" for
// nobody in particular).
func esemenyDetail(ctx context.Context, q store.Queries, e domain.Esemeny, viewerID string) (EsemenyDetail, error) {
	csoport, err := q.FindCsoport(ctx, e.CsoportID)
	if err != nil {
		return EsemenyDetail{}, err
	}
	szervezo, err := q.FindFelhasznalo(ctx, e.SzervezoID)
	if err != nil {
		return EsemenyDetail{}, err
	}
	idopontok, err := q.ListIdopontok(ctx, e.ID)
	if err != nil {
		return EsemenyDetail{}, err
	}
	visszajelzesek, err := q.ListVisszajelzesekOfEsemeny(ctx, e.ID)
	if err != nil {
		return EsemenyDetail{}, err
	}
	confirmed, _ := confirmedIdopont(e)
	return EsemenyDetail{
		Esemeny:          e,
		CsoportName:      csoport.Name,
		SzervezoName:     szervezo.Name,
		Idopontok:        idopontAllasok(e, idopontok, visszajelzesek, viewerID),
		ConfirmedIdopont: confirmed,
		Resztvevok:       resztvevokOf(e, visszajelzesek),
	}, nil
}

// idopontAllasok counts each Idopont's Visszajelzesek and ranks them.
func idopontAllasok(e domain.Esemeny, idopontok []domain.Idopont, visszajelzesek []domain.VisszajelzesOfEsemeny, viewerID string) []IdopontAllas {
	out := make([]IdopontAllas, len(idopontok))
	index := make(map[string]int, len(idopontok))
	confirmed, ok := confirmedIdopont(e)
	for n, i := range idopontok {
		out[n] = IdopontAllas{
			Idopont:        i,
			Visszajelzesek: []domain.VisszajelzesOfEsemeny{},
			Answerable:     answerable(e, i),
			Confirmed:      ok && i.ID == confirmed,
		}
		index[i.ID] = n
	}
	for _, v := range visszajelzesek {
		n, ok := index[v.IdopontID]
		if !ok {
			continue
		}
		allas := &out[n]
		allas.Visszajelzesek = append(allas.Visszajelzesek, v)
		if v.FelhasznaloID == viewerID {
			allas.Sajat = v.Valasz
		}
		if !v.IsTag {
			continue
		}
		switch v.Valasz {
		case domain.Igen:
			allas.Igen++
		case domain.Talan:
			allas.Talan++
		case domain.Nem:
			allas.Nem++
		}
	}

	pontszamok := make([]Pontszam, len(out))
	for n, allas := range out {
		if allas.Igen+allas.Talan+allas.Nem > 0 {
			pontszamok[n] = Pontszam{allas.Igen, allas.Talan, -allas.Nem}
		}
	}
	for n, h := range Helyezesek(pontszamok) {
		out[n].Helyezes = h
	}
	return out
}

// SetVisszajelzes records the Felhasznalo's answer to one Idopont of an
// Esemeny, replacing any earlier answer. Only Tags of the Esemeny's Csoport
// may answer; anyone else, and an Idopont of another Esemeny, gets
// ErrNotFound. An Idopont that isn't answerable in the Esemeny's state
// (see answerable) gives ErrIdopontNotAnswerable.
//
// A Resztvevo who drops from igen to talan or nem while the Esemeny is
// Veglegesitve is reported to the Szervezo by DM.
func (a *App) SetVisszajelzes(ctx context.Context, felhasznaloID, esemenyID, idopontID string, valasz domain.Valasz) error {
	if !valasz.Valid() {
		return fmt.Errorf("%w: a visszajelzes is igen, talan or nem", ErrInvalidInput)
	}
	var dropOut *gateway.DropOut
	var szervezoID, csoportID string
	err := a.store.InTx(ctx, func(q store.Queries) error {
		e, err := requireEsemenyTag(ctx, q, esemenyID, felhasznaloID)
		if err != nil {
			return err
		}
		i, err := q.FindIdopont(ctx, idopontID)
		if err != nil {
			return notFound(err)
		}
		if i.EsemenyID != e.ID {
			return ErrNotFound
		}
		if !answerable(e, i) {
			return ErrIdopontNotAnswerable
		}
		before, err := q.FindValasz(ctx, i.ID, felhasznaloID)
		if err != nil {
			return err
		}
		if err := q.UpsertVisszajelzes(ctx, domain.Visszajelzes{
			IdopontID:     i.ID,
			FelhasznaloID: felhasznaloID,
			Valasz:        valasz,
			UpdatedAt:     a.clock.Now(),
		}); err != nil {
			return err
		}
		droppedOut := makesResztvevo(e, i, before) && !makesResztvevo(e, i, valasz)
		if !droppedOut {
			return nil
		}
		// #16 releases the drop-out's claims here, in this transaction, the
		// Szervezo's own included, and #24 removes their calendar event.
		// (Joining, for #24, is makesResztvevo(e, i, valasz) &&
		// !makesResztvevo(e, i, before).)

		// The Szervezo dropping out needs no DM to themselves.
		if felhasznaloID != e.SzervezoID {
			d, err := a.dropOut(ctx, q, e, felhasznaloID)
			if err != nil {
				return err
			}
			dropOut, szervezoID, csoportID = &d, e.SzervezoID, e.CsoportID
		}
		return nil
	})
	if err != nil {
		return err
	}
	a.afterCommit(func(ctx context.Context) {
		a.refreshEsemenyMessages(ctx, esemenyID)
		if dropOut != nil {
			a.sendDropOutDM(ctx, csoportID, szervezoID, *dropOut)
		}
	})
	return nil
}

// answerable tells whether the Idopont of the Esemeny takes Visszajelzesek
// now. It is the one seam for this rule: SetVisszajelzes refuses the others,
// the web shows answer buttons on exactly these (IdopontAllas.Answerable),
// and so does the Discord message.
//
//   - Egyeztetes: every Idopont.
//   - Veglegesitve: only the confirmed one, so late answers and drop-outs
//     are still possible.
//   - Elmaradt and Lezajlott: none.
func answerable(e domain.Esemeny, i domain.Idopont) bool {
	switch e.Allapot {
	case domain.Egyeztetes:
		return true
	case domain.Veglegesitve:
		confirmed, ok := confirmedIdopont(e)
		return ok && i.ID == confirmed
	default:
		return false
	}
}

// requireEsemenyTag returns the Esemeny if the Felhasznalo is a Tag of its
// Csoport. Anyone else gets ErrNotFound, whether or not the Esemeny exists.
func requireEsemenyTag(ctx context.Context, q store.Queries, esemenyID, felhasznaloID string) (domain.Esemeny, error) {
	e, err := q.FindEsemeny(ctx, esemenyID)
	if err != nil {
		return domain.Esemeny{}, notFound(err)
	}
	if _, err := requireTag(ctx, q, e.CsoportID, felhasznaloID); err != nil {
		return domain.Esemeny{}, err
	}
	return e, nil
}

func mustLoadLocation(name string) *time.Location {
	loc, err := time.LoadLocation(name)
	if err != nil {
		panic(fmt.Sprintf("load time zone %s: %v", name, err))
	}
	return loc
}
