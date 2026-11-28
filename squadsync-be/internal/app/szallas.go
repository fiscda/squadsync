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

// Szallas is the Kiegeszito for sleeping at one place. The Szervezo sets up
// Szobak, each with Agyak sleeping one or two (the femohely), and the
// Resztvevok claim a spot in an Agy (a Tag for their name-only Vendeg) or
// mark themselves "máshol alszik". There is one Szallas per Esemeny, which
// is just the Kiegeszito itself with an optional note about the place, and
// one assignment for the whole stay.
//
// Who may do what:
//   - Setup (the note, Szobak, Agyak): only the Szervezo, in Egyeztetes and
//     Veglegesitve (requireSetupAllowed). The Szervezo can delete a Szoba
//     or an Agy, and lower an Agy's femohely, but not below the people
//     sleeping in it (delete the Agy for that).
//   - Spots, only while the Esemeny is Veglegesitve: a Resztvevo claims one
//     for themselves, a Tag for their Vendeg (#13). At most one spot per
//     person, so moving means leaving first; an Agy never holds more than
//     its femohely, so two people claiming a double Agy share it. Anyone
//     leaves their own spot.
//   - The Szervezo moves anyone into another Agy with room, or removes them
//     from their spot.
//   - "Máshol alszik" clears the spot, and taking a spot clears the mark.
//   - Deleting a Szoba removes its Agyak and spots and, after the commit,
//     sends everyone who slept there a DM. Deleting an Agy only frees its
//     spots: the Szervezo did it on purpose, and the count of people
//     without a bed shows the gap.
//   - A person leaving the Esemeny (releaseFoglalasok) loses their spot and
//     mark. Nobody else loses anything, so nobody else is told.

func init() {
	registerKiegeszito(kiegeszito{
		kind:          domain.Szallas,
		releaseClaims: releaseFoglalasok,
		summary:       szallasSummary,
	})
}

// Limits on a Szallas.
const (
	MaxSzallasNoteLength = 200
	MaxSzobaNameLength   = 60
	MaxAgyLabelLength    = 40
	MinAgyCapacity       = 1
	MaxAgyCapacity       = 2
)

// Errors of Szallas.
var (
	// ErrAgyFull means the Agy has no free spot.
	ErrAgyFull = errors.New("the agy has no free spot")
	// ErrAlreadyInAgy means the holder has a spot in another Agy: they leave
	// it first (only the Szervezo moves people).
	ErrAlreadyInAgy = errors.New("the holder already has a spot in another agy")
	// ErrAgyCapacityBelowAlvok means the Agy can't have a smaller femohely
	// than the number of people sleeping in it.
	ErrAgyCapacityBelowAlvok = errors.New("the agy has more sleepers than that femohely")
)

// SzallasInput is the Szallas note being set.
type SzallasInput struct {
	Note string
}

// AgyInput is an Agy being added or edited.
type AgyInput struct {
	// Capacity is the femohely: 1 (single) or 2 (double).
	Capacity int
	Label    string
}

// FoglalasInput says whose spot a call is about: a Felhasznalo (empty: the
// caller) or the caller's name-only Vendeg (#13).
type FoglalasInput struct {
	FelhasznaloID string
	VendegID      string
}

func (in FoglalasInput) holder(actorID string) domain.ClaimHolder {
	switch {
	case in.VendegID != "":
		return domain.ClaimHolder{VendegID: in.VendegID}
	case in.FelhasznaloID != "":
		return domain.ClaimHolder{FelhasznaloID: in.FelhasznaloID}
	default:
		return domain.ClaimHolder{FelhasznaloID: actorID}
	}
}

// AgyAllas is an Agy with its sleepers.
type AgyAllas struct {
	domain.Agy
	// Alvok are the people sleeping in it, ordered by name.
	Alvok []domain.Foglalas
	// Szabad is how many spots are still free.
	Szabad int
}

// SzobaAllas is a Szoba with its Agyak.
type SzobaAllas struct {
	domain.Szoba
	Agyak []AgyAllas
}

// Szallas is an Esemeny's Szallas as one viewer sees it.
type Szallas struct {
	Note string
	// Szobak are in order.
	Szobak []SzobaAllas
	// MasholAlszik are the Resztvevok marked máshol alszik, ordered by name.
	MasholAlszik []domain.MasholAlszik
	// AgyNelkul are the Resztvevok without a bed: everyone but the sleepers
	// and those marked máshol alszik.
	AgyNelkul []domain.Resztvevo
	// CanSetup tells whether the viewer may set the Szallas up now (the
	// Szervezo, in Egyeztetes or Veglegesitve).
	CanSetup bool
	// CanClaim tells whether the viewer may take a spot or mark themselves
	// máshol alszik now: a Resztvevo while Veglegesitve.
	CanClaim bool
	// SajatAgyID is the Agy the viewer sleeps in, "" for none.
	SajatAgyID string
	// SajatMasholAlszik tells whether the viewer is marked máshol alszik.
	SajatMasholAlszik bool
	// IsSzervezo tells whether the viewer may move people.
	IsSzervezo bool
}

// szallasData is everything stored about an Esemeny's Szallas.
type szallasData struct {
	note         string
	szobak       []domain.Szoba
	agyak        []domain.Agy
	foglalasok   []domain.Foglalas
	masholAlszik []domain.MasholAlszik
}

func readSzallas(ctx context.Context, q store.Queries, esemenyID string) (szallasData, error) {
	var d szallasData
	var err error
	if d.note, err = q.GetSzallasNote(ctx, esemenyID); err != nil {
		return szallasData{}, err
	}
	if d.szobak, err = q.ListSzobak(ctx, esemenyID); err != nil {
		return szallasData{}, err
	}
	if d.agyak, err = q.ListAgyak(ctx, esemenyID); err != nil {
		return szallasData{}, err
	}
	if d.foglalasok, err = q.ListFoglalasok(ctx, esemenyID); err != nil {
		return szallasData{}, err
	}
	if d.masholAlszik, err = q.ListMasholAlszik(ctx, esemenyID); err != nil {
		return szallasData{}, err
	}
	return d, nil
}

// foglalasokOf returns the sleepers of one Agy.
func (d szallasData) foglalasokOf(agyID string) []domain.Foglalas {
	out := []domain.Foglalas{}
	for _, g := range d.foglalasok {
		if g.AgyID == agyID {
			out = append(out, g)
		}
	}
	return out
}

func (d szallasData) foglalasOf(holder domain.ClaimHolder) (domain.Foglalas, bool) {
	for _, g := range d.foglalasok {
		if g.Holder == holder {
			return g, true
		}
	}
	return domain.Foglalas{}, false
}

// agyNelkul returns the Resztvevok who have no spot and aren't marked
// máshol alszik.
func (d szallasData) agyNelkul(resztvevok []domain.Resztvevo) []domain.Resztvevo {
	has := map[domain.ClaimHolder]bool{}
	for _, g := range d.foglalasok {
		has[g.Holder] = true
	}
	for _, m := range d.masholAlszik {
		has[m.Holder] = true
	}
	out := []domain.Resztvevo{}
	for _, r := range resztvevok {
		if !has[domain.HolderOf(r)] {
			out = append(out, r)
		}
	}
	return out
}

// Szallas returns the Esemeny's Szallas. A non-Tag gets ErrNotFound; while
// it is switched off, ErrKiegeszitoOff.
func (a *App) Szallas(ctx context.Context, felhasznaloID, esemenyID string) (Szallas, error) {
	q := a.store.Queries
	e, err := requireKiegeszitoOn(ctx, q, esemenyID, felhasznaloID, domain.Szallas)
	if err != nil {
		return Szallas{}, err
	}
	d, err := readSzallas(ctx, q, e.ID)
	if err != nil {
		return Szallas{}, err
	}
	vs, err := q.ListVisszajelzesekOfEsemeny(ctx, e.ID)
	if err != nil {
		return Szallas{}, err
	}
	me := domain.ClaimHolder{FelhasznaloID: felhasznaloID}
	out := Szallas{
		Note:         d.note,
		Szobak:       make([]SzobaAllas, 0, len(d.szobak)),
		MasholAlszik: d.masholAlszik,
		AgyNelkul:    d.agyNelkul(resztvevokOf(e, vs)),
		IsSzervezo:   e.SzervezoID == felhasznaloID,
		CanSetup:     e.SzervezoID == felhasznaloID && setupAllowed(e),
	}
	if out.MasholAlszik == nil {
		out.MasholAlszik = []domain.MasholAlszik{}
	}
	for _, szoba := range d.szobak {
		sz := SzobaAllas{Szoba: szoba, Agyak: []AgyAllas{}}
		for _, agy := range d.agyak {
			if agy.SzobaID != szoba.ID {
				continue
			}
			alvok := d.foglalasokOf(agy.ID)
			sz.Agyak = append(sz.Agyak, AgyAllas{Agy: agy, Alvok: alvok, Szabad: max(0, agy.Capacity-len(alvok))})
		}
		out.Szobak = append(out.Szobak, sz)
	}
	if g, ok := d.foglalasOf(me); ok {
		out.SajatAgyID = g.AgyID
	}
	for _, m := range d.masholAlszik {
		out.SajatMasholAlszik = out.SajatMasholAlszik || m.Holder == me
	}
	if out.CanClaim, err = canClaim(ctx, q, e, felhasznaloID); err != nil {
		return Szallas{}, err
	}
	return out, nil
}

// requireSzallasSetup returns the Esemeny if the Kiegeszito is on and the
// Felhasznalo, its Szervezo, may set it up now.
func requireSzallasSetup(ctx context.Context, q store.Queries, esemenyID, felhasznaloID string) (domain.Esemeny, error) {
	e, err := requireKiegeszitoOn(ctx, q, esemenyID, felhasznaloID, domain.Szallas)
	if err != nil {
		return domain.Esemeny{}, err
	}
	if e.SzervezoID != felhasznaloID {
		return domain.Esemeny{}, ErrNotPermitted
	}
	if err := requireSetupAllowed(e); err != nil {
		return domain.Esemeny{}, err
	}
	return e, nil
}

func validSzobaName(name string) (string, error) {
	name = strings.TrimSpace(name)
	if name == "" || utf8.RuneCountInString(name) > MaxSzobaNameLength {
		return "", fmt.Errorf("%w: a szoba's name is 1 to %d characters", ErrInvalidInput, MaxSzobaNameLength)
	}
	return name, nil
}

func validAgy(in AgyInput) (AgyInput, error) {
	if in.Capacity < MinAgyCapacity || in.Capacity > MaxAgyCapacity {
		return AgyInput{}, fmt.Errorf("%w: an agy sleeps %d or %d", ErrInvalidInput, MinAgyCapacity, MaxAgyCapacity)
	}
	in.Label = strings.TrimSpace(in.Label)
	if utf8.RuneCountInString(in.Label) > MaxAgyLabelLength {
		return AgyInput{}, fmt.Errorf("%w: an agy's label is at most %d characters", ErrInvalidInput, MaxAgyLabelLength)
	}
	return in, nil
}

// SetSzallas sets the Szallas note (the name of the place, an address, how
// to get in). Only the Szervezo may (anyone else ErrNotPermitted), in
// Egyeztetes and Veglegesitve. Besides the errors of requireKiegeszitoOn.
func (a *App) SetSzallas(ctx context.Context, felhasznaloID, esemenyID string, in SzallasInput) error {
	note := strings.TrimSpace(in.Note)
	if utf8.RuneCountInString(note) > MaxSzallasNoteLength {
		return fmt.Errorf("%w: the szallas note is at most %d characters", ErrInvalidInput, MaxSzallasNoteLength)
	}
	err := a.store.InTx(ctx, func(q store.Queries) error {
		e, err := requireSzallasSetup(ctx, q, esemenyID, felhasznaloID)
		if err != nil {
			return err
		}
		return q.SetSzallasNote(ctx, e.ID, note)
	})
	if err != nil {
		return err
	}
	a.kiegeszitoChanged(esemenyID)
	return nil
}

// AddSzoba adds a Szoba after the others. Only the Szervezo may, in
// Egyeztetes and Veglegesitve. Bad input is ErrInvalidInput.
func (a *App) AddSzoba(ctx context.Context, felhasznaloID, esemenyID, name string) (domain.Szoba, error) {
	name, err := validSzobaName(name)
	if err != nil {
		return domain.Szoba{}, err
	}
	var szoba domain.Szoba
	err = a.store.InTx(ctx, func(q store.Queries) error {
		e, err := requireSzallasSetup(ctx, q, esemenyID, felhasznaloID)
		if err != nil {
			return err
		}
		szoba = domain.Szoba{ID: randomID(), EsemenyID: e.ID, Name: name, CreatedAt: a.clock.Now()}
		return q.InsertSzoba(ctx, &szoba)
	})
	if err != nil {
		return domain.Szoba{}, err
	}
	a.kiegeszitoChanged(esemenyID)
	return szoba, nil
}

// findSzobaOf returns the Szoba of the Esemeny, or ErrNotFound.
func findSzobaOf(ctx context.Context, q store.Queries, e domain.Esemeny, szobaID string) (domain.Szoba, error) {
	szoba, err := q.FindSzoba(ctx, szobaID)
	if err != nil {
		return domain.Szoba{}, notFound(err)
	}
	if szoba.EsemenyID != e.ID {
		return domain.Szoba{}, ErrNotFound
	}
	return szoba, nil
}

// findAgyOf returns the Agy of the Esemeny, or ErrNotFound.
func findAgyOf(ctx context.Context, q store.Queries, e domain.Esemeny, agyID string) (domain.Agy, error) {
	agy, err := q.FindAgy(ctx, agyID)
	if err != nil {
		return domain.Agy{}, notFound(err)
	}
	if agy.EsemenyID != e.ID {
		return domain.Agy{}, ErrNotFound
	}
	return agy, nil
}

// RenameSzoba renames a Szoba. Only the Szervezo may, in Egyeztetes and
// Veglegesitve. A Szoba of another Esemeny is ErrNotFound.
func (a *App) RenameSzoba(ctx context.Context, felhasznaloID, esemenyID, szobaID, name string) error {
	name, err := validSzobaName(name)
	if err != nil {
		return err
	}
	err = a.store.InTx(ctx, func(q store.Queries) error {
		e, err := requireSzallasSetup(ctx, q, esemenyID, felhasznaloID)
		if err != nil {
			return err
		}
		szoba, err := findSzobaOf(ctx, q, e, szobaID)
		if err != nil {
			return err
		}
		return q.RenameSzoba(ctx, szoba.ID, name)
	})
	if err != nil {
		return err
	}
	a.kiegeszitoChanged(esemenyID)
	return nil
}

// DeleteSzoba deletes a Szoba with its Agyak and spots, and sends each
// person who slept there a DM. Only the Szervezo may, in Egyeztetes and
// Veglegesitve. A Szoba of another Esemeny is ErrNotFound.
func (a *App) DeleteSzoba(ctx context.Context, felhasznaloID, esemenyID, szobaID string) error {
	var jobs []kiegeszitoJob
	err := a.store.InTx(ctx, func(q store.Queries) error {
		e, err := requireSzallasSetup(ctx, q, esemenyID, felhasznaloID)
		if err != nil {
			return err
		}
		szoba, err := findSzobaOf(ctx, q, e, szobaID)
		if err != nil {
			return err
		}
		d, err := readSzallas(ctx, q, e.ID)
		if err != nil {
			return err
		}
		inSzoba := map[string]bool{}
		for _, agy := range d.agyak {
			if agy.SzobaID == szoba.ID {
				inSzoba[agy.ID] = true
			}
		}
		var alvok []domain.Foglalas
		for _, g := range d.foglalasok {
			if inSzoba[g.AgyID] {
				alvok = append(alvok, g)
			}
		}
		if err := q.DeleteSzoba(ctx, szoba.ID); err != nil {
			return err
		}
		jobs = a.szobaGoneJobs(e, szoba.Name, alvok)
		return nil
	})
	if err != nil {
		return err
	}
	a.kiegeszitoChanged(esemenyID, jobs...)
	return nil
}

// szobaGoneJobs queues the DMs that tell the sleepers of a deleted Szoba to
// claim a new Agy. Only Felhasznalok are told: a name-only Vendeg (#13) has
// no Discord, and #13 decides whether their Tag is told instead. It
// captures plain values only.
func (a *App) szobaGoneJobs(e domain.Esemeny, szobaName string, alvok []domain.Foglalas) []kiegeszitoJob {
	dm := gateway.SzallasDM{SzobaName: szobaName, Title: e.Title, Link: a.esemenyURL(e.ID)}
	csoportID := e.CsoportID
	var jobs []kiegeszitoJob
	for _, g := range alvok {
		if g.Holder.FelhasznaloID == "" {
			continue
		}
		felhasznaloID := g.Holder.FelhasznaloID
		jobs = append(jobs, func(ctx context.Context, a *App) {
			a.sendSzallasDM(ctx, csoportID, felhasznaloID, dm)
		})
	}
	return jobs
}

// AddAgy adds an Agy to a Szoba. Only the Szervezo may, in Egyeztetes and
// Veglegesitve. A Szoba of another Esemeny is ErrNotFound, bad input
// ErrInvalidInput.
func (a *App) AddAgy(ctx context.Context, felhasznaloID, esemenyID, szobaID string, in AgyInput) (domain.Agy, error) {
	in, err := validAgy(in)
	if err != nil {
		return domain.Agy{}, err
	}
	var agy domain.Agy
	err = a.store.InTx(ctx, func(q store.Queries) error {
		e, err := requireSzallasSetup(ctx, q, esemenyID, felhasznaloID)
		if err != nil {
			return err
		}
		szoba, err := findSzobaOf(ctx, q, e, szobaID)
		if err != nil {
			return err
		}
		agy = domain.Agy{
			ID: randomID(), SzobaID: szoba.ID, EsemenyID: e.ID,
			Capacity: in.Capacity, Label: in.Label, CreatedAt: a.clock.Now(),
		}
		return q.InsertAgy(ctx, &agy)
	})
	if err != nil {
		return domain.Agy{}, err
	}
	a.kiegeszitoChanged(esemenyID)
	return agy, nil
}

// UpdateAgy changes an Agy's femohely and label. Only the Szervezo may, in
// Egyeztetes and Veglegesitve. The femohely can't go below the number of
// sleepers (ErrAgyCapacityBelowAlvok); the Szervezo deletes the Agy
// instead. An Agy of another Esemeny is ErrNotFound.
func (a *App) UpdateAgy(ctx context.Context, felhasznaloID, esemenyID, agyID string, in AgyInput) error {
	in, err := validAgy(in)
	if err != nil {
		return err
	}
	err = a.store.InTx(ctx, func(q store.Queries) error {
		e, err := requireSzallasSetup(ctx, q, esemenyID, felhasznaloID)
		if err != nil {
			return err
		}
		agy, err := findAgyOf(ctx, q, e, agyID)
		if err != nil {
			return err
		}
		d, err := readSzallas(ctx, q, e.ID)
		if err != nil {
			return err
		}
		if n := len(d.foglalasokOf(agy.ID)); in.Capacity < n {
			return fmt.Errorf("%w: %d sleepers", ErrAgyCapacityBelowAlvok, n)
		}
		return q.UpdateAgy(ctx, agy.ID, in.Capacity, in.Label)
	})
	if err != nil {
		return err
	}
	a.kiegeszitoChanged(esemenyID)
	return nil
}

// DeleteAgy deletes an Agy and frees the spots in it. Only the Szervezo
// may, in Egyeztetes and Veglegesitve. An Agy of another Esemeny is
// ErrNotFound.
func (a *App) DeleteAgy(ctx context.Context, felhasznaloID, esemenyID, agyID string) error {
	err := a.store.InTx(ctx, func(q store.Queries) error {
		e, err := requireSzallasSetup(ctx, q, esemenyID, felhasznaloID)
		if err != nil {
			return err
		}
		agy, err := findAgyOf(ctx, q, e, agyID)
		if err != nil {
			return err
		}
		return q.DeleteAgy(ctx, agy.ID)
	})
	if err != nil {
		return err
	}
	a.kiegeszitoChanged(esemenyID)
	return nil
}

// ClaimAgy gives the caller (or their name-only Vendeg, #13) a spot in an
// Agy. The holder is the caller unless in names their Vendeg; anyone else
// gets ErrNotPermitted (the Szervezo moves people with MoveAlvo). Claiming
// the spot they already hold changes nothing. Errors, besides those of
// requireKiegeszitoOn and requireClaimAllowed:
//   - ErrNotFound: an Agy of another Esemeny;
//   - ErrAlreadyInAgy: the holder sleeps in another Agy, so leaving comes
//     first;
//   - ErrAgyFull: no free spot, checked in the transaction.
//
// Taking a spot removes the holder's "máshol alszik" mark.
func (a *App) ClaimAgy(ctx context.Context, felhasznaloID, esemenyID, agyID string, in FoglalasInput) error {
	holder := in.holder(felhasznaloID)
	err := a.store.InTx(ctx, func(q store.Queries) error {
		e, err := requireKiegeszitoOn(ctx, q, esemenyID, felhasznaloID, domain.Szallas)
		if err != nil {
			return err
		}
		if err := requireClaimAllowed(ctx, q, e, felhasznaloID, holder); err != nil {
			return err
		}
		agy, err := findAgyOf(ctx, q, e, agyID)
		if err != nil {
			return err
		}
		return a.sleep(ctx, q, e, agy, holder, false)
	})
	if err != nil {
		return err
	}
	a.kiegeszitoChanged(esemenyID)
	return nil
}

// MoveAlvo is the Szervezo's: it puts anyone, a sleeper or a Resztvevo
// without a bed, into an Agy with a free spot, taking them out of their old
// one. Anyone but the Szervezo gets ErrNotPermitted. The errors are those
// of ClaimAgy, except ErrAlreadyInAgy.
func (a *App) MoveAlvo(ctx context.Context, felhasznaloID, esemenyID, agyID string, in FoglalasInput) error {
	holder := in.holder(felhasznaloID)
	err := a.store.InTx(ctx, func(q store.Queries) error {
		e, err := requireKiegeszitoOn(ctx, q, esemenyID, felhasznaloID, domain.Szallas)
		if err != nil {
			return err
		}
		if e.SzervezoID != felhasznaloID {
			return ErrNotPermitted
		}
		if err := requireClaimAllowedBy(ctx, q, e, felhasznaloID, holder, true); err != nil {
			return err
		}
		agy, err := findAgyOf(ctx, q, e, agyID)
		if err != nil {
			return err
		}
		return a.sleep(ctx, q, e, agy, holder, true)
	})
	if err != nil {
		return err
	}
	a.kiegeszitoChanged(esemenyID)
	return nil
}

// sleep gives the holder a spot in the Agy, in the transaction. With move
// the holder may leave another Agy for it.
func (a *App) sleep(ctx context.Context, q store.Queries, e domain.Esemeny, agy domain.Agy, holder domain.ClaimHolder, move bool) error {
	d, err := readSzallas(ctx, q, e.ID)
	if err != nil {
		return err
	}
	if g, sleeping := d.foglalasOf(holder); sleeping {
		if g.AgyID == agy.ID {
			return nil
		}
		if !move {
			return ErrAlreadyInAgy
		}
	}
	if len(d.foglalasokOf(agy.ID)) >= agy.Capacity {
		return ErrAgyFull
	}
	if err := q.SetMasholAlszik(ctx, e.ID, holder, false, a.clock.Now()); err != nil {
		return err
	}
	return q.SetFoglalas(ctx, e.ID, agy.ID, holder, a.clock.Now())
}

// ReleaseFoglalas takes the caller's spot (or their Vendeg's, #13) away. The
// Szervezo may take anyone's: in names them. Anyone else's gets
// ErrNotPermitted. Having no spot changes nothing. Errors are those of
// requireKiegeszitoOn and requireClaimAllowedBy.
func (a *App) ReleaseFoglalas(ctx context.Context, felhasznaloID, esemenyID string, in FoglalasInput) error {
	holder := in.holder(felhasznaloID)
	err := a.store.InTx(ctx, func(q store.Queries) error {
		e, err := requireKiegeszitoOn(ctx, q, esemenyID, felhasznaloID, domain.Szallas)
		if err != nil {
			return err
		}
		if err := requireClaimAllowedBy(ctx, q, e, felhasznaloID, holder, true); err != nil {
			return err
		}
		return q.DeleteFoglalas(ctx, e.ID, holder)
	})
	if err != nil {
		return err
	}
	a.kiegeszitoChanged(esemenyID)
	return nil
}

// MasholAlszikInput marks or unmarks "máshol alszik". VendegID is the
// caller's name-only Vendeg (#13), "" for the caller.
type MasholAlszikInput struct {
	On       bool
	VendegID string
}

// SetMasholAlszik marks the caller (or their Vendeg) "máshol alszik", which
// takes away any spot they hold, or unmarks them. Errors are those of
// requireKiegeszitoOn and requireClaimAllowed.
func (a *App) SetMasholAlszik(ctx context.Context, felhasznaloID, esemenyID string, in MasholAlszikInput) error {
	holder := FoglalasInput{VendegID: in.VendegID}.holder(felhasznaloID)
	err := a.store.InTx(ctx, func(q store.Queries) error {
		e, err := requireKiegeszitoOn(ctx, q, esemenyID, felhasznaloID, domain.Szallas)
		if err != nil {
			return err
		}
		if err := requireClaimAllowed(ctx, q, e, felhasznaloID, holder); err != nil {
			return err
		}
		if in.On {
			if err := q.DeleteFoglalas(ctx, e.ID, holder); err != nil {
				return err
			}
		}
		return q.SetMasholAlszik(ctx, e.ID, holder, in.On, a.clock.Now())
	})
	if err != nil {
		return err
	}
	a.kiegeszitoChanged(esemenyID)
	return nil
}

// releaseFoglalasok is Szallas's claim releaser: a released sleeper's spot
// is freed, and a released holder's "máshol alszik" mark is cleared. Nobody
// else loses anything, so there are no DMs.
func releaseFoglalasok(ctx context.Context, t *kiegeszitoTx, release func(domain.ClaimHolder) bool) ([]string, error) {
	d, err := readSzallas(ctx, t.q, t.e.ID)
	if err != nil {
		return nil, err
	}
	szobaOfAgy := map[string]string{}
	szobaName := map[string]string{}
	for _, szoba := range d.szobak {
		szobaName[szoba.ID] = szoba.Name
	}
	for _, agy := range d.agyak {
		szobaOfAgy[agy.ID] = szobaName[agy.SzobaID]
	}
	var freed []string
	for _, g := range d.foglalasok {
		if !release(g.Holder) {
			continue
		}
		if err := t.q.DeleteFoglalas(ctx, t.e.ID, g.Holder); err != nil {
			return nil, err
		}
		freed = append(freed, fmt.Sprintf("1 ágy (%s)", szobaOfAgy[g.AgyID]))
	}
	for _, m := range d.masholAlszik {
		if !release(m.Holder) {
			continue
		}
		if err := t.q.SetMasholAlszik(ctx, t.e.ID, m.Holder, false, t.a.clock.Now()); err != nil {
			return nil, err
		}
	}
	return freed, nil
}

// szallasSummary is Szallas's fragment of the Discord summary line: how many
// Resztvevok have no bed, "" while nothing is set up or nobody can be
// counted (no Resztvevok).
func szallasSummary(ctx context.Context, t *kiegeszitoTx) (string, error) {
	d, err := readSzallas(ctx, t.q, t.e.ID)
	if err != nil {
		return "", err
	}
	if len(d.szobak) == 0 && len(d.masholAlszik) == 0 {
		return "", nil
	}
	resztvevok, err := t.Resztvevok(ctx)
	if err != nil {
		return "", err
	}
	if len(resztvevok) == 0 {
		return "", nil
	}
	if n := len(d.agyNelkul(resztvevok)); n > 0 {
		return fmt.Sprintf("🛏 %d főnek nincs ágya", n), nil
	}
	return "🛏 mindenkinek van ágya", nil
}

// sendSzallasDM tells a sleeper, on every Discord account, that their Szoba
// is gone.
func (a *App) sendSzallasDM(ctx context.Context, csoportID, felhasznaloID string, d gateway.SzallasDM) {
	for _, discordID := range a.dmAccounts(ctx, csoportID, felhasznaloID) {
		cctx, cancel := context.WithTimeout(ctx, discordCallTimeout)
		err := a.discord.SendSzallasDM(cctx, discordID, d)
		cancel()
		a.logDiscordError("send szallas dm", err, "felhasznalo", felhasznaloID, "discord_id", discordID, "esemeny", d.Title)
	}
}
