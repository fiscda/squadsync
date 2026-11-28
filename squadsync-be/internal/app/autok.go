package app

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"

	"squadsync-be/internal/domain"
	"squadsync-be/internal/gateway"
	"squadsync-be/internal/store"
)

// Autok is the Kiegeszito for getting to the Esemeny: Resztvevok offer an
// Auto with a number of free seats and become its Sofor, and the other
// Resztvevok claim a seat (a Tag for their name-only Vendeg) or mark
// themselves "saját úton". There is one assignment for the whole Esemeny,
// with no return trip.
//
// Who may do what, all of it only while the Esemeny is Veglegesitve (offering
// an Auto is setup, but it is tied to a Resztvevo, so it follows the claim
// rules: requireClaimAllowed and requireClaimAllowedBy):
//   - Offering: a Resztvevo, who becomes the Sofor. Nobody drives two Autok,
//     and a Sofor holds no seat and isn't "saját úton": offering removes
//     both.
//   - Seats: a Resztvevo claims one for themselves, a Tag for their
//     Vendeg (#13). At most one seat per person, so moving means leaving
//     first; nobody can overbook an Auto.
//   - The Szervezo moves anyone to another Auto (with a free seat) or
//     removes them from a seat.
//   - The Sofor edits their Auto's seats (not below the passengers) and
//     note, or deletes it; the Szervezo deletes any Auto. Deleting sends
//     every passenger a DM.
//   - A Sofor dropping out (releaseUlesek) removes the Auto, makes the
//     passengers seatless and DMs them.
//
// For #21 (the fuel shortcut): AutoUtasai, or autoUtasai inside a
// transaction, gives an Auto's Sofor and passengers.

func init() {
	registerKiegeszito(kiegeszito{
		kind:          domain.Autok,
		releaseClaims: releaseUlesek,
		summary:       autokSummary,
	})
}

// Limits on an Auto.
const (
	MinAutoSeats      = 1
	MaxAutoSeats      = 8
	MaxAutoNoteLength = 100
)

// Errors of Autok.
var (
	// ErrAutoFull means the Auto has no free seat.
	ErrAutoFull = errors.New("the auto has no free seat")
	// ErrAlreadySofor means the Resztvevo already offers an Auto.
	ErrAlreadySofor = errors.New("the resztvevo already offers an auto")
	// ErrSoforCantRide means a Sofor can't hold a seat or be "saját úton".
	ErrSoforCantRide = errors.New("a sofor can't hold a seat or be saját úton")
	// ErrAlreadySeated means the holder has a seat in another Auto: they
	// leave it first (only the Szervezo moves people).
	ErrAlreadySeated = errors.New("the holder already has a seat in another auto")
	// ErrAutoSeatsBelowUtasok means the Auto can't have fewer seats than it
	// has passengers.
	ErrAutoSeatsBelowUtasok = errors.New("the auto has more passengers than that many seats")
)

// AutoInput is an Auto being offered or edited.
type AutoInput struct {
	// Seats is the number of free seats, not counting the Sofor (1 to 8).
	Seats int
	Note  string
}

// UtasInput says whose seat a call is about: a Felhasznalo (empty: the
// caller) or the caller's name-only Vendeg (#13).
type UtasInput struct {
	FelhasznaloID string
	VendegID      string
}

func (in UtasInput) holder(actorID string) domain.ClaimHolder {
	switch {
	case in.VendegID != "":
		return domain.ClaimHolder{VendegID: in.VendegID}
	case in.FelhasznaloID != "":
		return domain.ClaimHolder{FelhasznaloID: in.FelhasznaloID}
	default:
		return domain.ClaimHolder{FelhasznaloID: actorID}
	}
}

// AutoAllas is an Auto with its passengers, as one viewer sees it.
type AutoAllas struct {
	domain.Auto
	// Utasok are the passengers, ordered by name.
	Utasok []domain.Utas
	// Szabad is how many seats are still free.
	Szabad int
	// CanEdit tells whether the viewer may edit it (they are its Sofor).
	CanEdit bool
	// CanDelete tells whether the viewer may delete it (its Sofor or the
	// Szervezo).
	CanDelete bool
}

// Autok is an Esemeny's Autok as one viewer sees them.
type Autok struct {
	// Autok are oldest first.
	Autok []AutoAllas
	// SajatUton are the Resztvevok marked saját úton, ordered by name.
	SajatUton []domain.SajatUton
	// HelyNelkul are the Resztvevok with no seat: everyone but the Sofors,
	// the passengers and those saját úton.
	HelyNelkul []domain.Resztvevo
	// CanClaim tells whether the viewer may offer an Auto, claim a seat or
	// mark themselves saját úton now: a Resztvevo while Veglegesitve who
	// isn't a Sofor yet (a Sofor can do none of the three).
	CanClaim bool
	// SajatAutoID is the Auto the viewer drives, "" for none.
	SajatAutoID string
	// SajatUlesAutoID is the Auto the viewer has a seat in, "" for none.
	SajatUlesAutoID string
	// SajatUtonVan tells whether the viewer is marked saját úton.
	SajatUtonVan bool
	// IsSzervezo tells whether the viewer may move people.
	IsSzervezo bool
}

// autokData is everything stored about an Esemeny's Autok.
type autokData struct {
	autok     []domain.Auto
	utasok    []domain.Utas
	sajatUton []domain.SajatUton
}

func readAutok(ctx context.Context, q store.Queries, esemenyID string) (autokData, error) {
	var d autokData
	var err error
	if d.autok, err = q.ListAutok(ctx, esemenyID); err != nil {
		return autokData{}, err
	}
	if d.utasok, err = q.ListUtasokOfEsemeny(ctx, esemenyID); err != nil {
		return autokData{}, err
	}
	if d.sajatUton, err = q.ListSajatUton(ctx, esemenyID); err != nil {
		return autokData{}, err
	}
	return d, nil
}

// utasokOf returns the passengers of one Auto.
func (d autokData) utasokOf(autoID string) []domain.Utas {
	out := []domain.Utas{}
	for _, u := range d.utasok {
		if u.AutoID == autoID {
			out = append(out, u)
		}
	}
	return out
}

// helyNelkul returns the Resztvevok who have no seat and aren't saját úton.
func (d autokData) helyNelkul(resztvevok []domain.Resztvevo) []domain.Resztvevo {
	has := map[domain.ClaimHolder]bool{}
	for _, a := range d.autok {
		has[domain.ClaimHolder{FelhasznaloID: a.SoforID}] = true
	}
	for _, u := range d.utasok {
		has[u.Holder] = true
	}
	for _, s := range d.sajatUton {
		has[s.Holder] = true
	}
	out := []domain.Resztvevo{}
	for _, r := range resztvevok {
		if !has[domain.HolderOf(r)] {
			out = append(out, r)
		}
	}
	return out
}

func (d autokData) soforOf(holder domain.ClaimHolder) (domain.Auto, bool) {
	for _, a := range d.autok {
		if holder == (domain.ClaimHolder{FelhasznaloID: a.SoforID}) {
			return a, true
		}
	}
	return domain.Auto{}, false
}

func (d autokData) ulesOf(holder domain.ClaimHolder) (domain.Utas, bool) {
	for _, u := range d.utasok {
		if u.Holder == holder {
			return u, true
		}
	}
	return domain.Utas{}, false
}

// Autok returns the Esemeny's Autok. A non-Tag gets ErrNotFound; while it
// is switched off, ErrKiegeszitoOff.
func (a *App) Autok(ctx context.Context, felhasznaloID, esemenyID string) (Autok, error) {
	q := a.store.Queries
	e, err := requireKiegeszitoOn(ctx, q, esemenyID, felhasznaloID, domain.Autok)
	if err != nil {
		return Autok{}, err
	}
	d, err := readAutok(ctx, q, e.ID)
	if err != nil {
		return Autok{}, err
	}
	vs, err := q.ListVisszajelzesekOfEsemeny(ctx, e.ID)
	if err != nil {
		return Autok{}, err
	}
	me := domain.ClaimHolder{FelhasznaloID: felhasznaloID}
	out := Autok{
		Autok:      make([]AutoAllas, 0, len(d.autok)),
		SajatUton:  d.sajatUton,
		HelyNelkul: d.helyNelkul(resztvevokOf(e, vs)),
		IsSzervezo: e.SzervezoID == felhasznaloID,
	}
	if out.SajatUton == nil {
		out.SajatUton = []domain.SajatUton{}
	}
	for _, auto := range d.autok {
		utasok := d.utasokOf(auto.ID)
		isSofor := auto.SoforID == felhasznaloID
		out.Autok = append(out.Autok, AutoAllas{
			Auto:      auto,
			Utasok:    utasok,
			Szabad:    max(0, auto.Seats-len(utasok)),
			CanEdit:   isSofor && e.Allapot == domain.Veglegesitve,
			CanDelete: (isSofor || e.SzervezoID == felhasznaloID) && e.Allapot == domain.Veglegesitve,
		})
		if isSofor {
			out.SajatAutoID = auto.ID
		}
	}
	if u, ok := d.ulesOf(me); ok {
		out.SajatUlesAutoID = u.AutoID
	}
	for _, s := range d.sajatUton {
		out.SajatUtonVan = out.SajatUtonVan || s.Holder == me
	}
	can, err := canClaim(ctx, q, e, felhasznaloID)
	if err != nil {
		return Autok{}, err
	}
	out.CanClaim = can && out.SajatAutoID == ""
	return out, nil
}

// AutoUtasai is an Auto's occupants: its Sofor and its passengers. #21's
// fuel shortcut splits among them.
type AutoUtasai struct {
	Auto domain.Auto
	// Utasok are the passengers, ordered by name.
	Utasok []domain.Utas
}

// Holders returns everyone in the Auto: the Sofor first, then the
// passengers.
func (u AutoUtasai) Holders() []domain.ClaimHolder {
	out := []domain.ClaimHolder{{FelhasznaloID: u.Auto.SoforID}}
	for _, p := range u.Utasok {
		out = append(out, p.Holder)
	}
	return out
}

// autoUtasai reads an Auto's occupants inside a transaction. An Auto of
// another Esemeny is ErrNotFound.
func autoUtasai(ctx context.Context, q store.Queries, esemenyID, autoID string) (AutoUtasai, error) {
	auto, err := q.FindAuto(ctx, autoID)
	if err != nil {
		return AutoUtasai{}, notFound(err)
	}
	if auto.EsemenyID != esemenyID {
		return AutoUtasai{}, ErrNotFound
	}
	utasok, err := q.ListUtasokOfEsemeny(ctx, esemenyID)
	if err != nil {
		return AutoUtasai{}, err
	}
	out := AutoUtasai{Auto: auto, Utasok: []domain.Utas{}}
	for _, u := range utasok {
		if u.AutoID == autoID {
			out.Utasok = append(out.Utasok, u)
		}
	}
	return out, nil
}

// AutoUtasai returns an Auto's Sofor and passengers. A non-Tag, or an Auto
// of another Esemeny, gets ErrNotFound; while Autok is switched off,
// ErrKiegeszitoOff.
func (a *App) AutoUtasai(ctx context.Context, felhasznaloID, esemenyID, autoID string) (AutoUtasai, error) {
	q := a.store.Queries
	e, err := requireKiegeszitoOn(ctx, q, esemenyID, felhasznaloID, domain.Autok)
	if err != nil {
		return AutoUtasai{}, err
	}
	return autoUtasai(ctx, q, e.ID, autoID)
}

func validAuto(in AutoInput) (AutoInput, error) {
	if in.Seats < MinAutoSeats || in.Seats > MaxAutoSeats {
		return AutoInput{}, fmt.Errorf("%w: an auto has %d to %d free seats", ErrInvalidInput, MinAutoSeats, MaxAutoSeats)
	}
	in.Note = strings.TrimSpace(in.Note)
	if utf8.RuneCountInString(in.Note) > MaxAutoNoteLength {
		return AutoInput{}, fmt.Errorf("%w: an auto's note is at most %d characters", ErrInvalidInput, MaxAutoNoteLength)
	}
	return in, nil
}

// OfferAuto has a Resztvevo offer an Auto and become its Sofor. It takes
// away their own seat and "saját úton" mark. Besides the errors of
// requireKiegeszitoOn and requireClaimAllowed: ErrAlreadySofor if they
// already offer one, ErrInvalidInput for bad input.
func (a *App) OfferAuto(ctx context.Context, felhasznaloID, esemenyID string, in AutoInput) (domain.Auto, error) {
	in, err := validAuto(in)
	if err != nil {
		return domain.Auto{}, err
	}
	me := domain.ClaimHolder{FelhasznaloID: felhasznaloID}
	var auto domain.Auto
	err = a.store.InTx(ctx, func(q store.Queries) error {
		e, err := requireKiegeszitoOn(ctx, q, esemenyID, felhasznaloID, domain.Autok)
		if err != nil {
			return err
		}
		if err := requireClaimAllowed(ctx, q, e, felhasznaloID, me); err != nil {
			return err
		}
		if _, err := q.FindAutoOfSofor(ctx, e.ID, felhasznaloID); err == nil {
			return ErrAlreadySofor
		} else if !errors.Is(err, store.ErrNotFound) {
			return err
		}
		// A Sofor holds no seat and needs none.
		if err := q.DeleteUles(ctx, e.ID, me); err != nil {
			return err
		}
		if err := q.SetSajatUton(ctx, e.ID, me, false, a.clock.Now()); err != nil {
			return err
		}
		auto = domain.Auto{
			ID:        randomID(),
			EsemenyID: e.ID,
			SoforID:   felhasznaloID,
			Seats:     in.Seats,
			Note:      in.Note,
			CreatedAt: a.clock.Now(),
		}
		return q.InsertAuto(ctx, auto)
	})
	if err != nil {
		return domain.Auto{}, err
	}
	a.kiegeszitoChanged(esemenyID)
	return auto, nil
}

// findAutoOf returns the Auto of the Esemeny, or ErrNotFound.
func findAutoOf(ctx context.Context, q store.Queries, e domain.Esemeny, autoID string) (domain.Auto, error) {
	auto, err := q.FindAuto(ctx, autoID)
	if err != nil {
		return domain.Auto{}, notFound(err)
	}
	if auto.EsemenyID != e.ID {
		return domain.Auto{}, ErrNotFound
	}
	return auto, nil
}

// UpdateAuto changes the seats and note of the caller's own Auto. Anyone
// but its Sofor gets ErrNotPermitted. The seats can't go below the number of
// passengers (ErrAutoSeatsBelowUtasok). Besides the errors of
// requireKiegeszitoOn and requireClaimAllowed: an Auto of another Esemeny
// is ErrNotFound, bad input ErrInvalidInput.
func (a *App) UpdateAuto(ctx context.Context, felhasznaloID, esemenyID, autoID string, in AutoInput) error {
	in, err := validAuto(in)
	if err != nil {
		return err
	}
	err = a.store.InTx(ctx, func(q store.Queries) error {
		e, err := requireKiegeszitoOn(ctx, q, esemenyID, felhasznaloID, domain.Autok)
		if err != nil {
			return err
		}
		auto, err := findAutoOf(ctx, q, e, autoID)
		if err != nil {
			return err
		}
		if auto.SoforID != felhasznaloID {
			return ErrNotPermitted
		}
		if err := requireClaimAllowed(ctx, q, e, felhasznaloID, domain.ClaimHolder{FelhasznaloID: felhasznaloID}); err != nil {
			return err
		}
		occupants, err := autoUtasai(ctx, q, e.ID, auto.ID)
		if err != nil {
			return err
		}
		if in.Seats < len(occupants.Utasok) {
			return fmt.Errorf("%w: %d passengers", ErrAutoSeatsBelowUtasok, len(occupants.Utasok))
		}
		return q.UpdateAuto(ctx, auto.ID, in.Seats, in.Note)
	})
	if err != nil {
		return err
	}
	a.kiegeszitoChanged(esemenyID)
	return nil
}

// DeleteAuto deletes an Auto with its seats and sends each passenger a DM.
// Only its Sofor and the Szervezo may (anyone else ErrNotPermitted).
// Besides the errors of requireKiegeszitoOn and requireClaimAllowedBy: an
// Auto of another Esemeny is ErrNotFound.
func (a *App) DeleteAuto(ctx context.Context, felhasznaloID, esemenyID, autoID string) error {
	var jobs []kiegeszitoJob
	err := a.store.InTx(ctx, func(q store.Queries) error {
		e, err := requireKiegeszitoOn(ctx, q, esemenyID, felhasznaloID, domain.Autok)
		if err != nil {
			return err
		}
		auto, err := findAutoOf(ctx, q, e, autoID)
		if err != nil {
			return err
		}
		if auto.SoforID != felhasznaloID && e.SzervezoID != felhasznaloID {
			return ErrNotPermitted
		}
		// The Szervezo deletes on the Sofor's behalf.
		if err := requireClaimAllowedBy(ctx, q, e, felhasznaloID, domain.ClaimHolder{FelhasznaloID: auto.SoforID}, true); err != nil {
			return err
		}
		occupants, err := autoUtasai(ctx, q, e.ID, auto.ID)
		if err != nil {
			return err
		}
		if err := q.DeleteAuto(ctx, auto.ID); err != nil {
			return err
		}
		jobs = a.autoGoneJobs(e, auto, occupants.Utasok, nil, true)
		return nil
	})
	if err != nil {
		return err
	}
	a.kiegeszitoChanged(esemenyID, jobs...)
	return nil
}

// autoGoneJobs queues the DMs that tell the passengers of an Auto that it is
// gone, except those the skip function names (who are released too). Only
// Felhasznalok are told: a name-only Vendeg (#13) has no Discord, and #13
// decides whether their Tag is told instead. It captures plain values only.
func (a *App) autoGoneJobs(e domain.Esemeny, auto domain.Auto, utasok []domain.Utas, skip func(domain.ClaimHolder) bool, deleted bool) []kiegeszitoJob {
	dm := gateway.AutoDM{SoforName: auto.SoforName, Title: e.Title, Deleted: deleted, Link: a.esemenyURL(e.ID)}
	csoportID := e.CsoportID
	var jobs []kiegeszitoJob
	for _, u := range utasok {
		if u.Holder.FelhasznaloID == "" || (skip != nil && skip(u.Holder)) {
			continue
		}
		felhasznaloID := u.Holder.FelhasznaloID
		jobs = append(jobs, func(ctx context.Context, a *App) {
			a.sendAutoDM(ctx, csoportID, felhasznaloID, dm)
		})
	}
	return jobs
}

// ClaimUles gives the caller (or, #13, their name-only Vendeg) a seat in an
// Auto. The holder is the caller unless in names their Vendeg; anyone else
// gets ErrNotPermitted (the Szervezo moves people with MoveUtas). Claiming
// the seat they already hold changes nothing. Errors, besides those of
// requireKiegeszitoOn and requireClaimAllowed:
//   - ErrNotFound: an Auto of another Esemeny;
//   - ErrSoforCantRide: the holder drives an Auto;
//   - ErrAlreadySeated: the holder sits in another Auto, so leaving comes
//     first;
//   - ErrAutoFull: no free seat, checked in the transaction.
//
// Taking a seat removes the holder's "saját úton" mark.
func (a *App) ClaimUles(ctx context.Context, felhasznaloID, esemenyID, autoID string, in UtasInput) error {
	holder := in.holder(felhasznaloID)
	err := a.store.InTx(ctx, func(q store.Queries) error {
		e, err := requireKiegeszitoOn(ctx, q, esemenyID, felhasznaloID, domain.Autok)
		if err != nil {
			return err
		}
		auto, err := findAutoOf(ctx, q, e, autoID)
		if err != nil {
			return err
		}
		if err := requireClaimAllowed(ctx, q, e, felhasznaloID, holder); err != nil {
			return err
		}
		return a.seat(ctx, q, e, auto, holder, false)
	})
	if err != nil {
		return err
	}
	a.kiegeszitoChanged(esemenyID)
	return nil
}

// MoveUtas is the Szervezo's: it puts anyone, a seated passenger or a
// Resztvevo without a seat, into an Auto with a free seat, taking them out
// of their old one. Anyone but the Szervezo gets ErrNotPermitted. The
// errors are those of ClaimUles, except ErrAlreadySeated.
func (a *App) MoveUtas(ctx context.Context, felhasznaloID, esemenyID, autoID string, in UtasInput) error {
	holder := in.holder(felhasznaloID)
	err := a.store.InTx(ctx, func(q store.Queries) error {
		e, err := requireKiegeszitoOn(ctx, q, esemenyID, felhasznaloID, domain.Autok)
		if err != nil {
			return err
		}
		if e.SzervezoID != felhasznaloID {
			return ErrNotPermitted
		}
		auto, err := findAutoOf(ctx, q, e, autoID)
		if err != nil {
			return err
		}
		if err := requireClaimAllowedBy(ctx, q, e, felhasznaloID, holder, true); err != nil {
			return err
		}
		return a.seat(ctx, q, e, auto, holder, true)
	})
	if err != nil {
		return err
	}
	a.kiegeszitoChanged(esemenyID)
	return nil
}

// seat gives the holder a seat in the Auto, in the transaction. With move
// the holder may leave another Auto for it.
func (a *App) seat(ctx context.Context, q store.Queries, e domain.Esemeny, auto domain.Auto, holder domain.ClaimHolder, move bool) error {
	d, err := readAutok(ctx, q, e.ID)
	if err != nil {
		return err
	}
	if _, drives := d.soforOf(holder); drives {
		return ErrSoforCantRide
	}
	if u, seated := d.ulesOf(holder); seated {
		if u.AutoID == auto.ID {
			return nil
		}
		if !move {
			return ErrAlreadySeated
		}
	}
	if len(d.utasokOf(auto.ID)) >= auto.Seats {
		return ErrAutoFull
	}
	if err := q.SetSajatUton(ctx, e.ID, holder, false, a.clock.Now()); err != nil {
		return err
	}
	return q.SetUles(ctx, e.ID, auto.ID, holder, a.clock.Now())
}

// ReleaseUles takes the caller's seat (or their Vendeg's, #13) away. The
// Szervezo may take anyone's: in names them. Anyone else's gets
// ErrNotPermitted. Having no seat changes nothing. Errors are those of
// requireKiegeszitoOn and requireClaimAllowedBy.
func (a *App) ReleaseUles(ctx context.Context, felhasznaloID, esemenyID string, in UtasInput) error {
	holder := in.holder(felhasznaloID)
	err := a.store.InTx(ctx, func(q store.Queries) error {
		e, err := requireKiegeszitoOn(ctx, q, esemenyID, felhasznaloID, domain.Autok)
		if err != nil {
			return err
		}
		if err := requireClaimAllowedBy(ctx, q, e, felhasznaloID, holder, true); err != nil {
			return err
		}
		return q.DeleteUles(ctx, e.ID, holder)
	})
	if err != nil {
		return err
	}
	a.kiegeszitoChanged(esemenyID)
	return nil
}

// SajatUtonInput marks or unmarks "saját úton". VendegID is the caller's
// name-only Vendeg (#13), "" for the caller.
type SajatUtonInput struct {
	On       bool
	VendegID string
}

// SetSajatUton marks the caller (or their Vendeg) "saját úton", which takes
// away any seat they hold, or unmarks them. A Sofor gets ErrSoforCantRide.
// Besides the errors of requireKiegeszitoOn and requireClaimAllowed.
func (a *App) SetSajatUton(ctx context.Context, felhasznaloID, esemenyID string, in SajatUtonInput) error {
	holder := UtasInput{VendegID: in.VendegID}.holder(felhasznaloID)
	err := a.store.InTx(ctx, func(q store.Queries) error {
		e, err := requireKiegeszitoOn(ctx, q, esemenyID, felhasznaloID, domain.Autok)
		if err != nil {
			return err
		}
		if err := requireClaimAllowed(ctx, q, e, felhasznaloID, holder); err != nil {
			return err
		}
		if in.On {
			d, err := readAutok(ctx, q, e.ID)
			if err != nil {
				return err
			}
			if _, drives := d.soforOf(holder); drives {
				return ErrSoforCantRide
			}
			if err := q.DeleteUles(ctx, e.ID, holder); err != nil {
				return err
			}
		}
		return q.SetSajatUton(ctx, e.ID, holder, in.On, a.clock.Now())
	})
	if err != nil {
		return err
	}
	a.kiegeszitoChanged(esemenyID)
	return nil
}

// releaseUlesek is Autok's claim releaser. For each released Sofor their
// Auto is removed, which leaves its passengers seatless, and (only while
// Autok is on) each passenger who stays is told by DM. A released
// passenger's seat is freed, and a released holder's "saját úton" mark is
// cleared.
func releaseUlesek(ctx context.Context, t *kiegeszitoTx, release func(domain.ClaimHolder) bool) ([]string, error) {
	d, err := readAutok(ctx, t.q, t.e.ID)
	if err != nil {
		return nil, err
	}
	var freed []string
	gone := map[string]bool{}
	soforName := map[string]string{}
	for _, auto := range d.autok {
		soforName[auto.ID] = auto.SoforName
		if !release(domain.ClaimHolder{FelhasznaloID: auto.SoforID}) {
			continue
		}
		gone[auto.ID] = true
		if err := t.q.DeleteAuto(ctx, auto.ID); err != nil {
			return nil, err
		}
		utasok := d.utasokOf(auto.ID)
		staying := 0
		for _, u := range utasok {
			if !release(u.Holder) {
				staying++
			}
		}
		if staying > 0 {
			freed = append(freed, fmt.Sprintf("1 autó (%d utas hely nélkül)", staying))
		} else {
			freed = append(freed, "1 autó")
		}
		if t.enabled {
			for _, job := range t.a.autoGoneJobs(t.e, auto, utasok, release, false) {
				t.afterCommit(job)
			}
		}
	}
	for _, u := range d.utasok {
		if gone[u.AutoID] || !release(u.Holder) {
			continue
		}
		if err := t.q.DeleteUles(ctx, t.e.ID, u.Holder); err != nil {
			return nil, err
		}
		freed = append(freed, fmt.Sprintf("1 hely %s autójában", soforName[u.AutoID]))
	}
	for _, s := range d.sajatUton {
		if !release(s.Holder) {
			continue
		}
		if err := t.q.SetSajatUton(ctx, t.e.ID, s.Holder, false, t.a.clock.Now()); err != nil {
			return nil, err
		}
	}
	return freed, nil
}

// autokSummary is Autok's fragment of the Discord summary line: how many
// Resztvevok have no seat, "" while nothing is set up or nobody can be
// counted (no Resztvevok).
func autokSummary(ctx context.Context, t *kiegeszitoTx) (string, error) {
	d, err := readAutok(ctx, t.q, t.e.ID)
	if err != nil {
		return "", err
	}
	if len(d.autok) == 0 && len(d.sajatUton) == 0 {
		return "", nil
	}
	resztvevok, err := t.Resztvevok(ctx)
	if err != nil {
		return "", err
	}
	if len(resztvevok) == 0 {
		return "", nil
	}
	if n := len(d.helyNelkul(resztvevok)); n > 0 {
		return fmt.Sprintf("🚗 %d főnek nincs helye", n), nil
	}
	return "🚗 mindenkinek van helye", nil
}

// sendAutoDM tells a passenger, on every Discord account, that their Auto
// is gone.
func (a *App) sendAutoDM(ctx context.Context, csoportID, felhasznaloID string, d gateway.AutoDM) {
	for _, discordID := range a.dmAccounts(ctx, csoportID, felhasznaloID) {
		cctx, cancel := context.WithTimeout(ctx, discordCallTimeout)
		err := a.discord.SendAutoDM(cctx, discordID, d)
		cancel()
		a.logDiscordError("send auto dm", err, "felhasznalo", felhasznaloID, "discord_id", discordID, "esemeny", d.Title)
	}
}
