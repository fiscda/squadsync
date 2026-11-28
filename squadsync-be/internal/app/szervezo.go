package app

import (
	"context"
	"errors"
	"fmt"
	"slices"

	"squadsync-be/internal/domain"
	"squadsync-be/internal/gateway"
	"squadsync-be/internal/store"
)

// The Szervezo of an Esemeny can hand the role to another Tag at any time
// while the Esemeny is in Egyeztetes or Veglegesitve, without that Tag
// accepting. Only a Tag with a Visszajelzes on the Esemeny can receive it:
// not a Vendeg, and not someone who has left the Csoport.
//
// Handing over is also how a Szervezo leaves, or is removed by an Admin:
// a Tag who is the Szervezo of an active Esemeny can't go (see deleteTag)
// until each of them has a new Szervezo. If nobody qualifies for one, the
// Szervezo cancels it instead; an Admin can't, because Admins have no
// authority over any Esemeny, so that removal is refused.
//
// Everything that is addressed to "the Szervezo" (the drop-out and Hatarido
// DMs, the confirm buttons, the Kiegeszitok's powers) reads
// Esemeny.SzervezoID when it happens and never keeps it, so it follows the
// handover.

// ErrNotEligibleSzervezo means the Felhasznalo can't become the Szervezo of
// the Esemeny: they aren't a Tag of its Csoport with a Visszajelzes on it,
// or they already are its Szervezo.
var ErrNotEligibleSzervezo = errors.New("not eligible to be the szervezo of the esemeny")

// ErrNoEligibleSzervezo means an Esemeny has nobody to be handed to, which
// refuses removing its Szervezo. It is returned as a *NoEligibleSzervezoError.
var ErrNoEligibleSzervezo = errors.New("esemeny has no tag to hand the szervezo role to")

// EsemenyRef names an Esemeny in an error or a list.
type EsemenyRef struct {
	ID      string
	Title   string
	Allapot domain.Allapot
}

func refOf(e domain.Esemeny) EsemenyRef {
	return EsemenyRef{ID: e.ID, Title: e.Title, Allapot: e.Allapot}
}

// NoEligibleSzervezoError is the Esemeny with nobody to hand it to. It
// matches ErrNoEligibleSzervezo.
type NoEligibleSzervezoError struct{ Esemeny EsemenyRef }

func (e *NoEligibleSzervezoError) Error() string {
	return fmt.Sprintf("%v: %s", ErrNoEligibleSzervezo, e.Esemeny.Title)
}

func (e *NoEligibleSzervezoError) Unwrap() error { return ErrNoEligibleSzervezo }

// SzervezoOfActiveEsemenyError lists the active Esemenyek that keep a Tag
// from leaving or being removed. It matches ErrSzervezoOfActiveEsemeny.
type SzervezoOfActiveEsemenyError struct{ Esemenyek []EsemenyRef }

func (e *SzervezoOfActiveEsemenyError) Error() string { return ErrSzervezoOfActiveEsemeny.Error() }

func (e *SzervezoOfActiveEsemenyError) Unwrap() error { return ErrSzervezoOfActiveEsemeny }

// SzervezoEsemeny is an active Esemeny a Tag is the Szervezo of, with the
// Tags the role could go to.
type SzervezoEsemeny struct {
	EsemenyRef
	// Utodok are the eligible Tags, ordered by name: those with a
	// Visszajelzes on the Esemeny, other than the Szervezo. Empty means
	// only cancelling the Esemeny gets its Szervezo free.
	Utodok []domain.TagWithName
}

// SzervezoEsemenyek lists the Esemenyek in Egyeztetes or Veglegesitve that
// the target Tag is the Szervezo of, oldest first, each with its possible
// successors. They are what stops the target from leaving, and what an
// Admin must reassign to remove them; the web builds its pickers from it.
// A Tag may ask about themselves and an Admin about any Tag. Another Tag
// gets ErrNotAdmin, anyone who isn't a Tag ErrNotFound, as does a target
// who isn't a Tag.
func (a *App) SzervezoEsemenyek(ctx context.Context, felhasznaloID, csoportID, targetID string) ([]SzervezoEsemeny, error) {
	q := a.store.Queries
	actor, err := requireTag(ctx, q, csoportID, felhasznaloID)
	if err != nil {
		return nil, err
	}
	if felhasznaloID != targetID && !actor.Admin {
		return nil, ErrNotAdmin
	}
	if _, err := requireTag(ctx, q, csoportID, targetID); err != nil {
		return nil, err
	}
	esemenyek, err := q.ListActiveEsemenyekOfSzervezo(ctx, csoportID, targetID)
	if err != nil {
		return nil, err
	}
	out := make([]SzervezoEsemeny, 0, len(esemenyek))
	for _, e := range esemenyek {
		utodok, err := eligibleSzervezok(ctx, q, e)
		if err != nil {
			return nil, err
		}
		out = append(out, SzervezoEsemeny{EsemenyRef: refOf(e), Utodok: utodok})
	}
	return out, nil
}

// HandOverSzervezo makes another Tag the Szervezo of the Esemeny. Only its
// Szervezo may: a non-Tag gets ErrNotFound, any other Tag ErrNotSzervezo.
// An Esemeny that is Elmaradt or Lezajlott gives ErrInvalidTransition, and
// someone who isn't eligible (see eligibleSzervezok) ErrNotEligibleSzervezo,
// the Szervezo themself too. The new Szervezo is told by DM, and the
// Esemeny's Discord messages show them.
func (a *App) HandOverSzervezo(ctx context.Context, felhasznaloID, esemenyID, newSzervezoID string) error {
	var done handover
	err := a.store.InTx(ctx, func(q store.Queries) error {
		e, err := requireSzervezo(ctx, q, esemenyID, felhasznaloID)
		if err != nil {
			return err
		}
		utodok, err := eligibleSzervezok(ctx, q, e)
		if err != nil {
			return err
		}
		done, err = a.handOver(ctx, q, e, newSzervezoID, utodok)
		return err
	})
	if err != nil {
		return err
	}
	a.afterHandovers([]handover{done})
	return nil
}

// eligibleSzervezok returns the Tags the Esemeny's Szervezo role can go to:
// every current Tag of its Csoport with a Visszajelzes on it, but not its
// Szervezo.
func eligibleSzervezok(ctx context.Context, q store.Queries, e domain.Esemeny) ([]domain.TagWithName, error) {
	tags, err := q.ListVisszajelzoTags(ctx, e.ID)
	if err != nil {
		return nil, err
	}
	return slices.DeleteFunc(tags, func(t domain.TagWithName) bool { return t.FelhasznaloID == e.SzervezoID }), nil
}

// handover is a done change of Szervezo, waiting for its Discord work.
type handover struct {
	esemenyID, csoportID, newSzervezoID string
	dm                                  gateway.SzervezoDM
}

// handOver makes newSzervezoID the Szervezo of the Esemeny, in the caller's
// transaction, if they are one of the eligible Tags (see eligibleSzervezok).
// Only an Esemeny in Egyeztetes or Veglegesitve changes hands.
func (a *App) handOver(ctx context.Context, q store.Queries, e domain.Esemeny, newSzervezoID string, eligible []domain.TagWithName) (handover, error) {
	if e.Allapot != domain.Egyeztetes && e.Allapot != domain.Veglegesitve {
		return handover{}, ErrInvalidTransition
	}
	if !slices.ContainsFunc(eligible, func(t domain.TagWithName) bool { return t.FelhasznaloID == newSzervezoID }) {
		return handover{}, ErrNotEligibleSzervezo
	}
	if err := q.UpdateEsemenySzervezo(ctx, e.ID, newSzervezoID); err != nil {
		return handover{}, notFound(err)
	}
	csoport, err := q.FindCsoport(ctx, e.CsoportID)
	if err != nil {
		return handover{}, err
	}
	return handover{
		esemenyID: e.ID, csoportID: e.CsoportID, newSzervezoID: newSzervezoID,
		dm: gateway.SzervezoDM{CsoportName: csoport.Name, Title: e.Title, Link: a.esemenyURL(e.ID)},
	}, nil
}

// handOverAllOf hands every active Esemeny the target is the Szervezo of to
// the Tag picked for it in szervezok (Esemeny id to Felhasznalo id), in the
// caller's transaction. It is all or nothing:
//
//   - An Esemeny nobody can take gives *NoEligibleSzervezoError, checked
//     first, so the Admin learns what only the Szervezo can fix.
//   - Esemenyek without a pick give *SzervezoOfActiveEsemenyError naming
//     them, as if nothing had been picked.
//   - A pick that isn't eligible gives ErrNotEligibleSzervezo.
//
// Picks for Esemenyek that aren't the target's active ones are ignored: the
// picker may be stale, and they change nothing.
func (a *App) handOverAllOf(ctx context.Context, q store.Queries, csoportID, targetID string, szervezok map[string]string) ([]handover, error) {
	esemenyek, err := q.ListActiveEsemenyekOfSzervezo(ctx, csoportID, targetID)
	if err != nil {
		return nil, err
	}
	eligible := make([][]domain.TagWithName, len(esemenyek))
	for n, e := range esemenyek {
		if eligible[n], err = eligibleSzervezok(ctx, q, e); err != nil {
			return nil, err
		}
		if len(eligible[n]) == 0 {
			return nil, &NoEligibleSzervezoError{Esemeny: refOf(e)}
		}
	}
	var missing []EsemenyRef
	for _, e := range esemenyek {
		if szervezok[e.ID] == "" {
			missing = append(missing, refOf(e))
		}
	}
	if len(missing) > 0 {
		return nil, &SzervezoOfActiveEsemenyError{Esemenyek: missing}
	}
	done := make([]handover, 0, len(esemenyek))
	for n, e := range esemenyek {
		h, err := a.handOver(ctx, q, e, szervezok[e.ID], eligible[n])
		if err != nil {
			return nil, err
		}
		done = append(done, h)
	}
	return done, nil
}

// afterHandovers, once the change commits, re-renders each Esemeny's
// Discord messages, which name the Szervezo, and tells the new Szervezo.
func (a *App) afterHandovers(done []handover) {
	if len(done) == 0 {
		return
	}
	a.afterCommit(func(ctx context.Context) {
		for _, h := range done {
			a.refreshEsemenyMessages(ctx, h.esemenyID)
			a.sendSzervezoDM(ctx, h.csoportID, h.newSzervezoID, h.dm)
		}
	})
}
