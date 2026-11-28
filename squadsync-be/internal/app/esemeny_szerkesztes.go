package app

import (
	"context"
	"errors"
	"fmt"
	"time"

	"squadsync-be/internal/domain"
	"squadsync-be/internal/store"
)

// Changing an Esemeny after it was proposed:
//
//   - EditEsemeny: the Szervezo edits the title, description and Helyszin
//     (every state but Elmaradt).
//   - AddIdopont and RemoveIdopont: any Tag adds an Idopont, and removes
//     the ones they added while nobody has answered them; the Szervezo
//     removes any. Only in Egyeztetes. An Idopont's time is never edited:
//     add a new one and remove the old one, or reopen Egyeztetes.
//   - CopyEsemeny: any Tag copies an Esemeny a week later.
//
// Every change re-renders the Esemeny's Discord messages through the
// refresh job, the one writer of those messages.

// Errors of changing an Esemeny. Each is a 409 with its own code on the web
// API; see httpapi.
var (
	// ErrEsemenyElmaradt means the Esemeny is Elmaradt, which is final: its
	// details can't be edited.
	ErrEsemenyElmaradt = errors.New("the esemeny is elmaradt")
	// ErrNotEgyeztetes means Idopontok can be added or removed only while
	// the Esemeny is in Egyeztetes.
	ErrNotEgyeztetes = errors.New("idopontok can be changed only in egyeztetes")
	// ErrTooManyIdopontok means the Esemeny already has MaxIdopontok.
	ErrTooManyIdopontok = errors.New("the esemeny has the most idopontok it can")
	// ErrIdopontHasAnswers means a Tag who isn't the Szervezo tried to
	// remove an Idopont that has Visszajelzesek.
	ErrIdopontHasAnswers = errors.New("the idopont has been answered")
	// ErrLastIdopont means the Esemeny's only Idopont can't be removed.
	ErrLastIdopont = errors.New("the last idopont can't be removed")
)

// EditEsemeny changes the title, description and Helyszin of an Esemeny,
// with the same checks as proposing it. Only the Szervezo may (a non-Tag
// gets ErrNotFound, another Tag ErrNotSzervezo), in any state but Elmaradt
// (ErrEsemenyElmaradt). Bad input gives ErrInvalidInput.
func (a *App) EditEsemeny(ctx context.Context, felhasznaloID, esemenyID string, in EsemenyDetailsInput) error {
	err := a.store.InTx(ctx, func(q store.Queries) error {
		e, err := requireSzervezo(ctx, q, esemenyID, felhasznaloID)
		if err != nil {
			return err
		}
		if e.Allapot == domain.Elmaradt {
			return ErrEsemenyElmaradt
		}
		d, err := validDetails(in)
		if err != nil {
			return err
		}
		return q.UpdateEsemenyDetails(ctx, e.ID, d.Title, d.Description, d.Helyszin)
	})
	if err != nil {
		return err
	}
	a.afterCommit(func(ctx context.Context) { a.refreshEsemenyMessages(ctx, esemenyID) })
	return nil
}

// AddIdopont adds a candidate time to an Esemeny in Egyeztetes, with the
// same input and checks as proposing it. Any Tag may; a non-Tag gets
// ErrNotFound. Another state gives ErrNotEgyeztetes, and an Esemeny that
// has MaxIdopontok ErrTooManyIdopontok. The Felhasznalo is recorded as who
// added it.
func (a *App) AddIdopont(ctx context.Context, felhasznaloID, esemenyID string, in IdopontInput) (domain.Idopont, error) {
	var i domain.Idopont
	err := a.store.InTx(ctx, func(q store.Queries) error {
		e, err := requireEsemenyTag(ctx, q, esemenyID, felhasznaloID)
		if err != nil {
			return err
		}
		if e.Allapot != domain.Egyeztetes {
			return ErrNotEgyeztetes
		}
		existing, err := q.ListIdopontok(ctx, e.ID)
		if err != nil {
			return err
		}
		if len(existing) >= MaxIdopontok {
			return ErrTooManyIdopontok
		}
		if i, err = newIdopont(in); err != nil {
			return err
		}
		i.ID = randomID()
		i.EsemenyID = e.ID
		i.AddedBy = felhasznaloID
		i.CreatedAt = a.clock.Now()
		return q.InsertIdopont(ctx, i)
	})
	if err != nil {
		return domain.Idopont{}, err
	}
	a.afterCommit(func(ctx context.Context) { a.refreshEsemenyMessages(ctx, esemenyID) })
	return i, nil
}

// RemoveIdopont removes an Idopont of an Esemeny in Egyeztetes, with its
// Visszajelzesek. A non-Tag, or an Idopont of another Esemeny, gets
// ErrNotFound. Another state gives ErrNotEgyeztetes. The Szervezo may
// remove any Idopont; the Tag who added it only while nobody has answered
// it (ErrIdopontHasAnswers); any other Tag gets ErrNotPermitted. The last
// Idopont stays (ErrLastIdopont).
func (a *App) RemoveIdopont(ctx context.Context, felhasznaloID, esemenyID, idopontID string) error {
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
		idopontok, err := q.ListIdopontok(ctx, e.ID)
		if err != nil {
			return err
		}
		visszajelzesek, err := q.ListVisszajelzesekOfEsemeny(ctx, e.ID)
		if err != nil {
			return err
		}
		answered := 0
		for _, v := range visszajelzesek {
			if v.IdopontID == i.ID {
				answered++
			}
		}
		if err := removeIdopontError(e, len(idopontok), felhasznaloID, i, answered); err != nil {
			return err
		}
		return q.DeleteIdopont(ctx, i.ID)
	})
	if err != nil {
		return err
	}
	a.afterCommit(func(ctx context.Context) { a.refreshEsemenyMessages(ctx, esemenyID) })
	return nil
}

// removeIdopontError is the one place that decides whether actorID may
// remove the Idopont now (permission first, then the state), given how many Idopontok the Esemeny has and how
// many Visszajelzesek this one has. RemoveIdopont refuses with it and the
// web shows the remove button where it is nil (IdopontAllas.Removable).
func removeIdopontError(e domain.Esemeny, idopontCount int, actorID string, i domain.Idopont, answered int) error {
	switch {
	// Permission first: a Tag who may never remove it gets 403 in any state.
	case actorID != e.SzervezoID && i.AddedBy != actorID:
		return ErrNotPermitted
	case e.Allapot != domain.Egyeztetes:
		return ErrNotEgyeztetes
	case actorID != e.SzervezoID && answered > 0:
		return ErrIdopontHasAnswers
	case idopontCount <= 1:
		return ErrLastIdopont
	}
	return nil
}

// CopyEsemeny copies an Esemeny, in any state, into a new Esemeny of the
// same Csoport. The new one is in Egyeztetes with the copier as Szervezo.
// It gets the title, description and Helyszin, every Idopont a week later
// (see weekLater), and the same Kiegeszitok switched on, without their
// data. The Hatarido isn't copied. It is announced like a newly proposed
// Esemeny. Any Tag may copy; a non-Tag gets ErrNotFound.
func (a *App) CopyEsemeny(ctx context.Context, felhasznaloID, esemenyID string) (domain.Esemeny, error) {
	var fresh domain.Esemeny
	var after []kiegeszitoJob
	err := a.store.InTx(ctx, func(q store.Queries) error {
		source, err := requireEsemenyTag(ctx, q, esemenyID, felhasznaloID)
		if err != nil {
			return err
		}
		idopontok, err := q.ListIdopontok(ctx, source.ID)
		if err != nil {
			return err
		}
		enabled, err := q.EnabledKiegeszitok(ctx, source.ID)
		if err != nil {
			return err
		}

		now := a.clock.Now()
		fresh = domain.Esemeny{
			ID:          randomID(),
			CsoportID:   source.CsoportID,
			Title:       source.Title,
			Description: source.Description,
			Helyszin:    source.Helyszin,
			SzervezoID:  felhasznaloID,
			Allapot:     domain.Egyeztetes,
			CreatedAt:   now,
		}
		if err := q.InsertEsemeny(ctx, fresh); err != nil {
			return err
		}
		for _, i := range idopontok {
			shifted := domain.Idopont{
				ID:         randomID(),
				EsemenyID:  fresh.ID,
				Start:      weekLater(i.Start),
				EgeszNapos: i.EgeszNapos,
				AddedBy:    felhasznaloID,
				CreatedAt:  now,
			}
			if i.End != nil {
				end := weekLater(*i.End)
				shifted.End = &end
			}
			if err := q.InsertIdopont(ctx, shifted); err != nil {
				return err
			}
		}

		// The same Kiegeszitok are switched on, as if the Szervezo had just
		// switched each one on (so a Kiegeszito that sets something up on
		// switching on does), but none of their data is copied.
		t := &kiegeszitoTx{a: a, q: q, e: fresh, enabled: true}
		for _, kind := range domain.KiegeszitoKinds {
			if !enabled[kind] {
				continue
			}
			if err := q.SetKiegeszitoEnabled(ctx, fresh.ID, kind, true, now); err != nil {
				return err
			}
			if k, ok := a.kiegeszitok[kind]; ok && k.switchedOn != nil {
				if err := k.switchedOn(ctx, t); err != nil {
					return fmt.Errorf("switch on %s: %w", kind, err)
				}
			}
		}
		after = t.after
		return nil
	})
	if err != nil {
		return domain.Esemeny{}, err
	}
	a.afterCommit(func(ctx context.Context) {
		a.runKiegeszitoJobs(ctx, after)
		a.announceEsemeny(ctx, fresh.ID)
	})
	return fresh, nil
}

// weekLater shifts a stored (UTC) time by one week of Budapest wall clock
// time: the same day of the week and the same clock reading, so a time
// across a DST change keeps its reading (20:00 stays 20:00). A whole-day
// Idopont's midnights stay midnights the same way. A reading that doesn't
// exist in the new week (the clocks jump over it) moves forward by the
// jump; one that happens twice means its first occurrence, as when parsing.
func weekLater(t time.Time) time.Time {
	local := t.In(Budapest)
	y, m, d := local.Date()
	h, min, s := local.Clock()
	shifted := time.Date(y, m, d+7, h, min, s, local.Nanosecond(), Budapest)
	if earlier := shifted.Add(-time.Hour); earlier.In(Budapest).Format(LocalTimeLayout) == shifted.Format(LocalTimeLayout) {
		shifted = earlier
	}
	return shifted.UTC()
}
