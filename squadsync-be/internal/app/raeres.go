package app

import (
	"context"
	"fmt"
	"time"

	"squadsync-be/internal/domain"
	"squadsync-be/internal/store"
)

// A Felhasznalo's Raeres says, for each Budapest calendar day, whether they
// are szabad or foglalt; an unmarked day is unknown and isn't stored. Only a
// rolling window can be marked: from today in Budapest through RaeresWindowDays
// later. Raeres belongs to the Felhasznalo and is shared by all their
// Csoportok; it is visible only to fellow Tags, and then only as counts
// (CsoportHeatmap, EsemenyRaeres), so nothing here returns another
// Felhasznalo's days. A Vendeg gets no route to any of it.

// RaeresWindowDays is how many days after today can still be marked.
const RaeresWindowDays = 365

// RaeresDay is one day of a Csoport's heatmap.
type RaeresDay struct {
	// Day is a Budapest date, like "2026-12-05".
	Day string
	// Szabad is how many Tags are szabad that day, Osszes how many Tags
	// the Csoport has.
	Szabad, Osszes int
}

// IdopontRaeres is how the Raeres of the Csoport's Tags and of the viewer
// stands on the days one Idopont covers.
type IdopontRaeres struct {
	IdopontID string
	// Szabad is how many Tags are szabad on every covered day, Osszes how
	// many Tags the Csoport has.
	Szabad, Osszes int
	// Sajat is the viewer's own Raeres over the covered days: szabad if
	// they are szabad on every one, foglalt if foglalt on any, otherwise
	// "" (unknown or mixed).
	Sajat domain.RaeresAllapot
}

// SetRaeres marks every day from..to (Budapest dates in LocalDateLayout,
// both included) as szabad or foglalt, or, with allapot "", makes them
// unknown again. Both days must lie within the window, today through
// RaeresWindowDays later; anything else gives ErrInvalidInput.
func (a *App) SetRaeres(ctx context.Context, felhasznaloID, from, to string, allapot domain.RaeresAllapot) error {
	if allapot != "" && !allapot.Valid() {
		return fmt.Errorf("%w: a raeres is szabad or foglalt, or empty to clear", ErrInvalidInput)
	}
	first, last, err := parseDayRange(from, to)
	if err != nil {
		return err
	}
	windowFrom, windowTo := a.raeresWindow()
	if first.Before(windowFrom) || last.After(windowTo) {
		return fmt.Errorf("%w: only days from %s to %s can be marked", ErrInvalidInput, formatDay(windowFrom), formatDay(windowTo))
	}
	return a.store.InTx(ctx, func(q store.Queries) error {
		if err := requireFelhasznalo(ctx, q, felhasznaloID); err != nil {
			return err
		}
		if allapot == "" {
			return q.DeleteRaeresRange(ctx, felhasznaloID, formatDay(first), formatDay(last))
		}
		for d := first; !d.After(last); d = d.AddDate(0, 0, 1) {
			if err := q.UpsertRaeres(ctx, felhasznaloID, formatDay(d), allapot); err != nil {
				return err
			}
		}
		return nil
	})
}

// Raeres returns the Felhasznalo's own marked days from..to (Budapest dates,
// both included), oldest first. Past days and days beyond the window aren't
// shown: the range is cut to the window.
func (a *App) Raeres(ctx context.Context, felhasznaloID, from, to string) ([]domain.RaeresDay, error) {
	first, last, err := a.clampedRange(from, to)
	if err != nil {
		return nil, err
	}
	if last.Before(first) {
		return []domain.RaeresDay{}, nil
	}
	days, err := a.store.ListRaeres(ctx, felhasznaloID, formatDay(first), formatDay(last))
	if err != nil {
		return nil, err
	}
	if days == nil {
		days = []domain.RaeresDay{}
	}
	return days, nil
}

// CsoportHeatmap counts, for each day from..to (Budapest dates, both
// included, cut to the window), how many of the Csoport's Tags are szabad.
// Only Tags may see it; anyone else gets ErrNotFound.
func (a *App) CsoportHeatmap(ctx context.Context, felhasznaloID, csoportID, from, to string) ([]RaeresDay, error) {
	q := a.store.Queries
	if _, err := requireTag(ctx, q, csoportID, felhasznaloID); err != nil {
		return nil, err
	}
	first, last, err := a.clampedRange(from, to)
	if err != nil {
		return nil, err
	}
	out := []RaeresDay{}
	if last.Before(first) {
		return out, nil
	}
	osszes, err := q.CountTags(ctx, csoportID)
	if err != nil {
		return nil, err
	}
	szabad, err := q.CountTagsSzabadPerDay(ctx, csoportID, formatDay(first), formatDay(last))
	if err != nil {
		return nil, err
	}
	for d := first; !d.After(last); d = d.AddDate(0, 0, 1) {
		day := formatDay(d)
		out = append(out, RaeresDay{Day: day, Szabad: szabad[day], Osszes: osszes})
	}
	return out, nil
}

// EsemenyRaeres tells, for each Idopont of the Esemeny in chronological
// order, how many Tags are szabad on all the days it covers and what the
// viewer's own Raeres says. Only Tags of the Esemeny's Csoport may ask;
// anyone else gets ErrNotFound.
func (a *App) EsemenyRaeres(ctx context.Context, felhasznaloID, esemenyID string) ([]IdopontRaeres, error) {
	q := a.store.Queries
	e, err := requireEsemenyTag(ctx, q, esemenyID, felhasznaloID)
	if err != nil {
		return nil, err
	}
	idopontok, err := q.ListIdopontok(ctx, e.ID)
	if err != nil {
		return nil, err
	}
	out := make([]IdopontRaeres, len(idopontok))
	for n, i := range idopontok {
		szabad, osszes, err := szabadAznap(ctx, q, e.CsoportID, i)
		if err != nil {
			return nil, err
		}
		sajat, err := sajatRaeres(ctx, q, felhasznaloID, i)
		if err != nil {
			return nil, err
		}
		out[n] = IdopontRaeres{IdopontID: i.ID, Szabad: szabad, Osszes: osszes, Sajat: sajat}
	}
	return out, nil
}

// szabadAznap counts the Csoport's Tags who are szabad on every day the
// Idopont covers, and the Tags in all.
func szabadAznap(ctx context.Context, q store.Queries, csoportID string, i domain.Idopont) (szabad, osszes int, err error) {
	if osszes, err = q.CountTags(ctx, csoportID); err != nil {
		return 0, 0, err
	}
	first, last := idopontDays(i)
	szabad, err = q.CountTagsSzabadOnEveryDay(ctx, csoportID, formatDay(first), formatDay(last), dayCount(first, last))
	return szabad, osszes, err
}

// sajatRaeres summarises the Felhasznalo's own Raeres over the days the
// Idopont covers.
func sajatRaeres(ctx context.Context, q store.Queries, felhasznaloID string, i domain.Idopont) (domain.RaeresAllapot, error) {
	first, last := idopontDays(i)
	days, err := q.ListRaeres(ctx, felhasznaloID, formatDay(first), formatDay(last))
	if err != nil {
		return "", err
	}
	szabad := 0
	for _, d := range days {
		switch d.Allapot {
		case domain.Foglalt:
			return domain.Foglalt, nil
		case domain.Szabad:
			szabad++
		}
	}
	if szabad == dayCount(first, last) {
		return domain.Szabad, nil
	}
	return "", nil
}

// idopontDays is the first and last Budapest day the Idopont covers. A
// whole-day Idopont ends at the Budapest midnight after its last day, and a
// timed one that ends exactly at midnight likewise doesn't reach into the
// new day; an Idopont without an end covers only its start day.
func idopontDays(i domain.Idopont) (first, last time.Time) {
	first = civilDay(i.Start)
	last = first
	if i.End != nil && i.End.After(i.Start) {
		last = civilDay(i.End.Add(-time.Nanosecond))
	}
	return first, last
}

// raeresWindow is the first and last day that can be marked now.
func (a *App) raeresWindow() (from, to time.Time) {
	from = civilDay(a.clock.Now())
	return from, from.AddDate(0, 0, RaeresWindowDays)
}

// clampedRange parses a requested range and cuts it to the window. The
// result is empty (last before first) when nothing of it is in the window.
func (a *App) clampedRange(from, to string) (first, last time.Time, err error) {
	first, last, err = parseDayRange(from, to)
	if err != nil {
		return time.Time{}, time.Time{}, err
	}
	windowFrom, windowTo := a.raeresWindow()
	if first.Before(windowFrom) {
		first = windowFrom
	}
	if last.After(windowTo) {
		last = windowTo
	}
	return first, last, nil
}

// Days are handled as midnight UTC of their calendar date, so adding days
// never meets a DST change.

func parseDayRange(from, to string) (first, last time.Time, err error) {
	if first, err = parseDay(from); err != nil {
		return time.Time{}, time.Time{}, err
	}
	if last, err = parseDay(to); err != nil {
		return time.Time{}, time.Time{}, err
	}
	if last.Before(first) {
		return time.Time{}, time.Time{}, fmt.Errorf("%w: the last day must not be before the first", ErrInvalidInput)
	}
	return first, last, nil
}

func parseDay(s string) (time.Time, error) {
	d, err := time.Parse(LocalDateLayout, s)
	if err != nil {
		return time.Time{}, fmt.Errorf("%w: a day must look like 2026-12-05", ErrInvalidInput)
	}
	return d, nil
}

func formatDay(d time.Time) string { return d.Format(LocalDateLayout) }

// civilDay is the Budapest calendar date of the moment.
func civilDay(t time.Time) time.Time {
	b := t.In(Budapest)
	return time.Date(b.Year(), b.Month(), b.Day(), 0, 0, 0, 0, time.UTC)
}

// dayCount is how many days first..last span, both included.
func dayCount(first, last time.Time) int {
	return int(last.Sub(first)/(24*time.Hour)) + 1
}
