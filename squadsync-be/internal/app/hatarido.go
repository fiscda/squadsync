package app

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"time"

	"squadsync-be/internal/domain"
	"squadsync-be/internal/gateway"
	"squadsync-be/internal/store"
)

// A Hatarido is an Esemeny's optional voting deadline, set by its Szervezo
// when proposing it or later, while it is in Egyeztetes. When it passes, the
// Szervezo gets a DM with every Idopont, its Helyezes and counts, and a
// confirm button for each (or the web link, when there are too many for
// Discord's buttons). Voting stays open. If the Esemeny is no longer in
// Egyeztetes by then, nothing is sent; reopening it before the Hatarido
// makes the DM count again.

// ErrHataridoNotFuture means a Hatarido that isn't in the future. It wraps
// ErrInvalidInput.
var ErrHataridoNotFuture = fmt.Errorf("%w: a határidő a jövőben legyen", ErrInvalidInput)

// hataridoPayload identifies a Hatarido job: the moment it was set for.
type hataridoPayload struct {
	At time.Time `json:"at"`
}

// SetHatarido sets, changes or (with "") clears the Esemeny's voting
// Hatarido, given as a Budapest wall clock time in LocalTimeLayout. It must
// be in the future (ErrHataridoNotFuture). Only the Szervezo may (see
// requireSzervezo), and only in Egyeztetes (ErrInvalidTransition).
func (a *App) SetHatarido(ctx context.Context, felhasznaloID, esemenyID, hatarido string) error {
	at, err := parseHatarido(hatarido, a.clock.Now())
	if err != nil {
		return err
	}
	return a.store.InTx(ctx, func(q store.Queries) error {
		e, err := requireSzervezo(ctx, q, esemenyID, felhasznaloID)
		if err != nil {
			return err
		}
		if e.Allapot != domain.Egyeztetes {
			return ErrInvalidTransition
		}
		return a.setHatarido(ctx, q, e.ID, at)
	})
}

// parseHatarido reads a Hatarido from an entry point: nil for "".
func parseHatarido(s string, now time.Time) (*time.Time, error) {
	if strings.TrimSpace(s) == "" {
		return nil, nil
	}
	at, err := parseLocalTime(s)
	if err != nil {
		return nil, err
	}
	if !at.After(now) {
		return nil, ErrHataridoNotFuture
	}
	return &at, nil
}

// setHatarido stores the Hatarido (nil clears it) and reschedules its job,
// in the caller's transaction.
func (a *App) setHatarido(ctx context.Context, q store.Queries, esemenyID string, at *time.Time) error {
	if err := q.SetHatarido(ctx, esemenyID, at); err != nil {
		return notFound(err)
	}
	if err := cancelJobs(ctx, q, esemenyID, jobHatarido); err != nil {
		return err
	}
	if at == nil {
		return nil
	}
	return a.scheduleJob(ctx, q, jobHatarido, esemenyID, *at, hataridoPayload{At: at.UTC()})
}

// runHatarido sends the Szervezo the Hatarido DM, if the Esemeny is still in
// Egyeztetes with this Hatarido.
func (a *App) runHatarido(ctx context.Context, q store.Queries, j store.Job) (func(context.Context), error) {
	var p hataridoPayload
	if err := jobPayload(j, &p); err != nil {
		return nil, err
	}
	e, err := q.FindEsemeny(ctx, j.EsemenyID)
	if err != nil {
		return nil, err
	}
	if e.Allapot != domain.Egyeztetes {
		return nil, nil
	}
	current, err := q.FindHatarido(ctx, e.ID)
	if err != nil {
		return nil, err
	}
	if current == nil || !current.Equal(p.At) {
		return nil, nil
	}
	d, err := esemenyDetail(ctx, q, e, "")
	if err != nil {
		return nil, err
	}
	now := a.clock.Now()
	lines := make([]gateway.IdopontLine, len(d.Idopontok))
	for n, allas := range d.Idopontok {
		lines[n] = gateway.IdopontLine{
			ID:       allas.ID,
			Label:    idopontLabel(allas.Idopont, now),
			Igen:     allas.Igen,
			Talan:    allas.Talan,
			Nem:      allas.Nem,
			Helyezes: int(allas.Helyezes),
		}
	}
	// Best Helyezes first, unranked last; ties stay chronological.
	slices.SortStableFunc(lines, func(x, y gateway.IdopontLine) int { return rank(x.Helyezes) - rank(y.Helyezes) })
	dm := gateway.HataridoDM{
		EsemenyID:      e.ID,
		CsoportName:    d.CsoportName,
		Title:          e.Title,
		Idopontok:      lines,
		ConfirmButtons: len(lines) <= gateway.MaxAnswerRows,
		Link:           a.esemenyURL(e.ID),
	}
	return func(ctx context.Context) { a.sendHataridoDM(ctx, e.CsoportID, e.SzervezoID, dm) }, nil
}

// rank orders Helyezesek with "none" (0) after every place.
func rank(h int) int {
	if h == 0 {
		return MaxIdopontok + 1
	}
	return h
}
