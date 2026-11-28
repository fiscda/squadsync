package app

import (
	"context"
	"fmt"
	"slices"
	"time"

	"squadsync-be/internal/domain"
	"squadsync-be/internal/gateway"
	"squadsync-be/internal/store"
)

// An Emlekezteto is a DM before the confirmed start of a Veglegesitve
// Esemeny, to everyone with igen or talan on the confirmed Idopont. Each
// Csoport picks its offsets from EmlekeztetoValasztek; an Admin changes them.
//
// # When an Emlekezteto is sent
//
// Each offset's Emlekezteto is scheduled at the start minus the offset when
// the Idopont is confirmed (or the offsets change). One whose moment has
// already passed by then is skipped. When confirming, and only then, the
// shortest offset's is the exception: it is sent at once if the start is
// still ahead, so confirming shortly before the start still reminds
// everyone once. (With the default 24 hours and 1 hour, confirming 30
// minutes before sends the 1-hour one at once and skips the 24-hour one.)
// Changing the offsets never sends one at once: an Admin's edit shouldn't
// surprise everyone with a DM.
//
// An Emlekezteto that runs late, after downtime, is still sent, unless the
// Idopont has already started or a shorter offset's moment has passed too
// (then only that one is sent, so nobody gets two at once). The same offset
// is never sent twice for the same Idopont, even if it is confirmed again.

// EmlekeztetoValasztek are the offsets a Csoport can choose from.
var EmlekeztetoValasztek = []time.Duration{
	7 * 24 * time.Hour,
	3 * 24 * time.Hour,
	24 * time.Hour,
	3 * time.Hour,
	time.Hour,
	15 * time.Minute,
}

// emlekeztetoPayload identifies an Emlekezteto job: the confirmed Idopont
// and the offset, in minutes.
type emlekeztetoPayload struct {
	IdopontID string `json:"idopont"`
	Perc      int    `json:"perc"`
}

// SetEmlekeztetok sets the Csoport's Emlekezteto offsets: any subset of
// EmlekeztetoValasztek, none included; duplicates count once. Only an Admin
// may: another Tag gets ErrNotAdmin, anyone else ErrNotFound. An offset not
// on the list gives ErrInvalidInput. The future Emlekeztetok of the
// Csoport's Veglegesitve Esemenyek are rescheduled to the new offsets.
func (a *App) SetEmlekeztetok(ctx context.Context, felhasznaloID, csoportID string, offsets []time.Duration) error {
	percek := []int{}
	for _, o := range offsets {
		if !slices.Contains(EmlekeztetoValasztek, o) {
			return fmt.Errorf("%w: %s is not an emlekezteto offset", ErrInvalidInput, o)
		}
		if p := int(o / time.Minute); !slices.Contains(percek, p) {
			percek = append(percek, p)
		}
	}
	slices.SortFunc(percek, func(x, y int) int { return y - x })

	return a.store.InTx(ctx, func(q store.Queries) error {
		if err := requireAdmin(ctx, q, csoportID, felhasznaloID); err != nil {
			return err
		}
		if err := q.SetEmlekeztetoPercek(ctx, csoportID, percek); err != nil {
			return notFound(err)
		}
		esemenyek, err := q.ListEsemenyekOfCsoport(ctx, csoportID)
		if err != nil {
			return err
		}
		for _, e := range esemenyek {
			confirmed, ok := confirmedIdopont(e.Esemeny)
			if !ok || e.Allapot != domain.Veglegesitve {
				continue
			}
			i, err := q.FindIdopont(ctx, confirmed)
			if err != nil {
				return err
			}
			if err := cancelJobs(ctx, q, e.ID, jobEmlekezteto); err != nil {
				return err
			}
			if err := a.scheduleEmlekeztetok(ctx, q, e.Esemeny, i, false); err != nil {
				return err
			}
		}
		return nil
	})
}

func emlekeztetok(ctx context.Context, q store.Queries, csoportID string) ([]time.Duration, error) {
	percek, err := q.FindEmlekeztetoPercek(ctx, csoportID)
	if err != nil {
		return nil, notFound(err)
	}
	out := make([]time.Duration, len(percek))
	for n, p := range percek {
		out[n] = time.Duration(p) * time.Minute
	}
	return out, nil
}

// scheduleEmlekeztetok schedules the Emlekeztetok of the confirmed Idopont
// by the Csoport's offsets (see "When an Emlekezteto is sent"). catchUp,
// set only when confirming, sends the shortest offset's at once if its
// moment has passed but the start hasn't.
func (a *App) scheduleEmlekeztetok(ctx context.Context, q store.Queries, e domain.Esemeny, i domain.Idopont, catchUp bool) error {
	percek, err := q.FindEmlekeztetoPercek(ctx, e.CsoportID)
	if err != nil || len(percek) == 0 {
		return err
	}
	now := a.clock.Now()
	shortest := slices.Min(percek)
	for _, p := range percek {
		at := i.Start.Add(-time.Duration(p) * time.Minute)
		if at.Before(now) {
			if !catchUp || p != shortest || !now.Before(i.Start) {
				continue
			}
			at = now
		}
		if err := a.scheduleJob(ctx, q, jobEmlekezteto, e.ID, at, emlekeztetoPayload{IdopontID: i.ID, Perc: p}); err != nil {
			return err
		}
	}
	return nil
}

// runEmlekezteto sends an Emlekezteto, if the Esemeny is still Veglegesitve
// on the job's Idopont and it hasn't started yet.
func (a *App) runEmlekezteto(ctx context.Context, q store.Queries, j store.Job) (func(context.Context), error) {
	var p emlekeztetoPayload
	if err := jobPayload(j, &p); err != nil {
		return nil, err
	}
	e, err := q.FindEsemeny(ctx, j.EsemenyID)
	if err != nil {
		return nil, err
	}
	if e.Allapot != domain.Veglegesitve || e.ConfirmedIdopontID != p.IdopontID {
		return nil, nil
	}
	i, err := q.FindIdopont(ctx, p.IdopontID)
	if err != nil {
		return nil, err
	}
	now := a.clock.Now()
	if !now.Before(i.Start) {
		return nil, nil // too late to remind anyone
	}
	percek, err := q.FindEmlekeztetoPercek(ctx, e.CsoportID)
	if err != nil {
		return nil, err
	}
	for _, shorter := range percek {
		if shorter < p.Perc && !now.Before(i.Start.Add(-time.Duration(shorter)*time.Minute)) {
			return nil, nil // late: the shorter one, also due, reminds instead
		}
	}
	visszajelzesek, err := q.ListVisszajelzesekOfEsemeny(ctx, e.ID)
	if err != nil {
		return nil, err
	}
	cimzettek := emlekeztetoCimzettek(i, visszajelzesek)
	if len(cimzettek) == 0 {
		return nil, nil
	}
	csoport, err := q.FindCsoport(ctx, e.CsoportID)
	if err != nil {
		return nil, err
	}
	msg := gateway.Emlekezteto{
		CsoportName: csoport.Name,
		Title:       e.Title,
		Idopont:     idopontLabel(i, now),
		Link:        a.esemenyURL(e.ID),
	}
	return func(ctx context.Context) {
		for _, f := range cimzettek {
			a.sendEmlekeztetoDM(ctx, e.CsoportID, f, msg)
		}
	}, nil
}

// emlekeztetoCimzettek are the Felhasznalok to remind of the confirmed
// Idopont: those with igen or talan on it who count (see
// countsAsResztvevo), Tags and Vendegek who are Felhasznalok. A name-only
// Vendeg has no Felhasznalo to DM.
func emlekeztetoCimzettek(i domain.Idopont, visszajelzesek []domain.VisszajelzesOfEsemeny) []string {
	var out []string
	for _, v := range visszajelzesek {
		if v.IdopontID != i.ID || v.FelhasznaloID == "" || !countsAsResztvevo(v) {
			continue
		}
		if v.Valasz == domain.Igen || v.Valasz == domain.Talan {
			out = append(out, v.FelhasznaloID)
		}
	}
	return out
}
