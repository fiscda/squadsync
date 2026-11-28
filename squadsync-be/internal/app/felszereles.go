package app

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"

	"squadsync-be/internal/domain"
	"squadsync-be/internal/store"
)

// Felszereles is the Kiegeszito for things to bring: a list of Tetelek,
// each with a needed quantity that Resztvevok claim, partially if they
// like ("Anna: 2 of 3"). A "mindenki hozza" Tetel can't be claimed; it is a
// personal reminder for every Resztvevo.
//
// Who may do what:
//   - Adding a Tetel: any Tag while the Esemeny is in Egyeztetes (there are
//     no Resztvevok yet, and anyone may help plan, but not a Vendeg); once
//     Veglegesitve, the Resztvevok (Vendegek too) and the Szervezo.
//   - Editing or deleting one: whoever added it, or the Szervezo, in
//     Egyeztetes or Veglegesitve.
//   - Claiming: Resztvevok, only in Veglegesitve (requireClaimAllowed): a
//     Vendeg who is a Felhasznalo for themselves, a Tag for their name-only
//     Vendeg.

func init() {
	registerKiegeszito(kiegeszito{
		kind:          domain.Felszereles,
		releaseClaims: releaseVallalasok,
		summary:       felszerelesSummary,
	})
}

// Limits on a Felszereles list.
const (
	MaxTetelNameLength = 100
	MaxTetelQuantity   = 999
	MaxTetelek         = 100
)

// Errors of Felszereles.
var (
	// ErrTetelMindenkiHozza means a "mindenki hozza" Tetel can't be claimed.
	ErrTetelMindenkiHozza = errors.New("a mindenki hozza tetel can't be claimed")
	// ErrVallalasTooMuch means the claim is more than what is missing.
	ErrVallalasTooMuch = errors.New("the vallalas is more than what is missing")
	// ErrTetelClaimed means the change conflicts with the Tetel's claims:
	// lowering its quantity below what is claimed, or marking a claimed
	// Tetel "mindenki hozza".
	ErrTetelClaimed = errors.New("the change conflicts with the tetel's vallalasok")
)

// TetelInput is a Tetel being added or edited. Quantity 0 means the
// default, 1.
type TetelInput struct {
	Name          string
	Quantity      int
	MindenkiHozza bool
}

// VallalasInput is a claim of part of a Tetel: Quantity pieces (0 removes
// the claim). VendegID claims for the caller's name-only Vendeg; "" claims
// for the caller.
type VallalasInput struct {
	Quantity int
	VendegID string
}

// Felszereles is an Esemeny's Felszereles list as one viewer sees it.
type Felszereles struct {
	// Tetelek are oldest first.
	Tetelek []TetelAllas
	// Hianyzik is how many pieces are missing over every claimable Tetel.
	Hianyzik int
	// Emlekezteto names the "mindenki hozza" Tetelek: the viewer's personal
	// reminder list, set only while the viewer is a Resztvevo.
	Emlekezteto []string
	// CanAdd tells whether the viewer may add a Tetel now.
	CanAdd bool
	// CanClaim tells whether the viewer may claim now.
	CanClaim bool
}

// TetelAllas is a Tetel with its claims.
type TetelAllas struct {
	domain.Tetel
	// Vallalt is how many are claimed, Hianyzik how many are still missing
	// (both 0 for a "mindenki hozza" Tetel).
	Vallalt, Hianyzik int
	// Vallalasok are ordered by name.
	Vallalasok []domain.Vallalas
	// Sajat is how many the viewer claims.
	Sajat int
	// CanEdit tells whether the viewer may edit or delete it now.
	CanEdit bool
}

// Felszereles returns the Esemeny's Felszereles list. A non-Tag gets
// ErrNotFound; while it is switched off, ErrKiegeszitoOff.
func (a *App) Felszereles(ctx context.Context, felhasznaloID, esemenyID string) (Felszereles, error) {
	q := a.store.Queries
	e, err := requireKiegeszitoOn(ctx, q, esemenyID, felhasznaloID, domain.Felszereles)
	if err != nil {
		return Felszereles{}, err
	}
	tetelek, err := felszerelesAllas(ctx, q, e.ID)
	if err != nil {
		return Felszereles{}, err
	}
	out := Felszereles{Tetelek: tetelek, Emlekezteto: []string{}}
	if out.CanClaim, err = canClaim(ctx, q, e, felhasznaloID); err != nil {
		return Felszereles{}, err
	}
	if out.CanAdd, err = canAddTetel(ctx, q, e, felhasznaloID); err != nil {
		return Felszereles{}, err
	}
	resztvevo, err := isResztvevo(ctx, q, e, domain.ClaimHolder{FelhasznaloID: felhasznaloID})
	if err != nil {
		return Felszereles{}, err
	}
	for n := range out.Tetelek {
		t := &out.Tetelek[n]
		out.Hianyzik += t.Hianyzik
		t.CanEdit = setupAllowed(e) && (t.AddedBy == felhasznaloID || e.SzervezoID == felhasznaloID)
		for _, v := range t.Vallalasok {
			if v.Holder == (domain.ClaimHolder{FelhasznaloID: felhasznaloID}) {
				t.Sajat = v.Quantity
			}
		}
		if t.MindenkiHozza && resztvevo {
			out.Emlekezteto = append(out.Emlekezteto, t.Name)
		}
	}
	return out, nil
}

// felszerelesAllas reads the Esemeny's Tetelek with their claims.
func felszerelesAllas(ctx context.Context, q store.Queries, esemenyID string) ([]TetelAllas, error) {
	tetelek, err := q.ListTetelek(ctx, esemenyID)
	if err != nil {
		return nil, err
	}
	vallalasok, err := q.ListVallalasokOfEsemeny(ctx, esemenyID)
	if err != nil {
		return nil, err
	}
	out := make([]TetelAllas, len(tetelek))
	index := make(map[string]int, len(tetelek))
	for n, t := range tetelek {
		out[n] = TetelAllas{Tetel: t, Vallalasok: []domain.Vallalas{}}
		index[t.ID] = n
	}
	for _, v := range vallalasok {
		n, ok := index[v.TetelID]
		if !ok {
			continue
		}
		out[n].Vallalasok = append(out[n].Vallalasok, v)
		out[n].Vallalt += v.Quantity
	}
	for n := range out {
		if !out[n].MindenkiHozza {
			out[n].Hianyzik = max(0, out[n].Quantity-out[n].Vallalt)
		}
	}
	return out, nil
}

// canAddTetel tells whether the Felhasznalo may add a Tetel now (see the
// rules at the top of this file).
func canAddTetel(ctx context.Context, q store.Queries, e domain.Esemeny, felhasznaloID string) (bool, error) {
	switch e.Allapot {
	case domain.Egyeztetes:
		if _, err := q.FindTag(ctx, e.CsoportID, felhasznaloID); errors.Is(err, store.ErrNotFound) {
			return false, nil
		} else if err != nil {
			return false, err
		}
		return true, nil
	case domain.Veglegesitve:
		if e.SzervezoID == felhasznaloID {
			return true, nil
		}
		return isResztvevo(ctx, q, e, domain.ClaimHolder{FelhasznaloID: felhasznaloID})
	default:
		return false, nil
	}
}

func validTetel(in TetelInput) (TetelInput, error) {
	in.Name = strings.TrimSpace(in.Name)
	if in.Name == "" {
		return TetelInput{}, fmt.Errorf("%w: a tetel needs a name", ErrInvalidInput)
	}
	if utf8.RuneCountInString(in.Name) > MaxTetelNameLength {
		return TetelInput{}, fmt.Errorf("%w: a tetel name is at most %d characters", ErrInvalidInput, MaxTetelNameLength)
	}
	if in.Quantity == 0 {
		in.Quantity = 1
	}
	if in.Quantity < 1 || in.Quantity > MaxTetelQuantity {
		return TetelInput{}, fmt.Errorf("%w: a tetel's quantity is 1 to %d", ErrInvalidInput, MaxTetelQuantity)
	}
	return in, nil
}

// AddTetel adds a Tetel to the Esemeny's Felszereles list. Besides the
// errors of requireKiegeszitoOn and requireSetupAllowed, someone who may
// not add one now gets ErrNotPermitted and bad input ErrInvalidInput.
func (a *App) AddTetel(ctx context.Context, felhasznaloID, esemenyID string, in TetelInput) (domain.Tetel, error) {
	in, err := validTetel(in)
	if err != nil {
		return domain.Tetel{}, err
	}
	var t domain.Tetel
	err = a.store.InTx(ctx, func(q store.Queries) error {
		e, err := requireKiegeszitoOn(ctx, q, esemenyID, felhasznaloID, domain.Felszereles)
		if err != nil {
			return err
		}
		if err := requireSetupAllowed(e); err != nil {
			return err
		}
		ok, err := canAddTetel(ctx, q, e, felhasznaloID)
		if err != nil {
			return err
		}
		if !ok {
			return ErrNotPermitted
		}
		existing, err := q.ListTetelek(ctx, e.ID)
		if err != nil {
			return err
		}
		if len(existing) >= MaxTetelek {
			return fmt.Errorf("%w: at most %d tetelek", ErrInvalidInput, MaxTetelek)
		}
		t = domain.Tetel{
			ID:            randomID(),
			EsemenyID:     e.ID,
			Name:          in.Name,
			Quantity:      in.Quantity,
			MindenkiHozza: in.MindenkiHozza,
			AddedBy:       felhasznaloID,
			CreatedAt:     a.clock.Now(),
		}
		return q.InsertTetel(ctx, t)
	})
	if err != nil {
		return domain.Tetel{}, err
	}
	a.kiegeszitoChanged(esemenyID)
	return t, nil
}

// editableTetel returns the Tetel of the Esemeny if the Felhasznalo may
// edit or delete it now: whoever added it, or the Szervezo, in Egyeztetes
// or Veglegesitve.
func editableTetel(ctx context.Context, q store.Queries, esemenyID, tetelID, felhasznaloID string) (domain.Tetel, error) {
	e, err := requireKiegeszitoOn(ctx, q, esemenyID, felhasznaloID, domain.Felszereles)
	if err != nil {
		return domain.Tetel{}, err
	}
	t, err := q.FindTetel(ctx, tetelID)
	if err != nil {
		return domain.Tetel{}, notFound(err)
	}
	if t.EsemenyID != e.ID {
		return domain.Tetel{}, ErrNotFound
	}
	if err := requireSetupAllowed(e); err != nil {
		return domain.Tetel{}, err
	}
	if t.AddedBy != felhasznaloID && e.SzervezoID != felhasznaloID {
		return domain.Tetel{}, ErrNotPermitted
	}
	return t, nil
}

// UpdateTetel changes a Tetel's name, quantity and "mindenki hozza" mark.
// The errors are those of DeleteTetel, plus ErrInvalidInput for bad input
// and ErrTetelClaimed when the quantity would go below what is claimed or a
// claimed Tetel would become "mindenki hozza".
func (a *App) UpdateTetel(ctx context.Context, felhasznaloID, esemenyID, tetelID string, in TetelInput) error {
	in, err := validTetel(in)
	if err != nil {
		return err
	}
	err = a.store.InTx(ctx, func(q store.Queries) error {
		t, err := editableTetel(ctx, q, esemenyID, tetelID, felhasznaloID)
		if err != nil {
			return err
		}
		vallalt, err := vallaltOf(ctx, q, t)
		if err != nil {
			return err
		}
		if vallalt > 0 && in.MindenkiHozza {
			return fmt.Errorf("%w: it has vallalasok", ErrTetelClaimed)
		}
		if in.Quantity < vallalt {
			return fmt.Errorf("%w: %d already claimed", ErrTetelClaimed, vallalt)
		}
		t.Name, t.Quantity, t.MindenkiHozza = in.Name, in.Quantity, in.MindenkiHozza
		return q.UpdateTetel(ctx, t)
	})
	if err != nil {
		return err
	}
	a.kiegeszitoChanged(esemenyID)
	return nil
}

// DeleteTetel deletes a Tetel with its claims. A non-Tag, or a Tetel of
// another Esemeny, gets ErrNotFound; anyone but its adder and the Szervezo
// ErrNotPermitted; plus the errors of requireKiegeszitoOn and
// requireSetupAllowed.
func (a *App) DeleteTetel(ctx context.Context, felhasznaloID, esemenyID, tetelID string) error {
	err := a.store.InTx(ctx, func(q store.Queries) error {
		t, err := editableTetel(ctx, q, esemenyID, tetelID, felhasznaloID)
		if err != nil {
			return err
		}
		return q.DeleteTetel(ctx, t.ID)
	})
	if err != nil {
		return err
	}
	a.kiegeszitoChanged(esemenyID)
	return nil
}

// vallaltOf is how many of the Tetel are claimed.
func vallaltOf(ctx context.Context, q store.Queries, t domain.Tetel) (int, error) {
	tetelek, err := felszerelesAllas(ctx, q, t.EsemenyID)
	if err != nil {
		return 0, err
	}
	for _, allas := range tetelek {
		if allas.ID == t.ID {
			return allas.Vallalt, nil
		}
	}
	return 0, nil
}

// SetVallalas sets how many of a Tetel the caller claims (or their
// name-only Vendeg), replacing their earlier claim; 0 removes it. At most
// what is missing plus their own earlier claim can be claimed
// (ErrVallalasTooMuch). Besides the errors of requireKiegeszitoOn and
// requireClaimAllowed: a Tetel of another Esemeny is ErrNotFound, a
// "mindenki hozza" one ErrTetelMindenkiHozza, a negative quantity
// ErrInvalidInput.
func (a *App) SetVallalas(ctx context.Context, felhasznaloID, esemenyID, tetelID string, in VallalasInput) error {
	if in.Quantity < 0 {
		return fmt.Errorf("%w: a vallalas can't be negative", ErrInvalidInput)
	}
	holder := domain.ClaimHolder{FelhasznaloID: felhasznaloID}
	if in.VendegID != "" {
		holder = domain.ClaimHolder{VendegID: in.VendegID}
	}
	err := a.store.InTx(ctx, func(q store.Queries) error {
		e, err := requireKiegeszitoOn(ctx, q, esemenyID, felhasznaloID, domain.Felszereles)
		if err != nil {
			return err
		}
		t, err := q.FindTetel(ctx, tetelID)
		if err != nil {
			return notFound(err)
		}
		if t.EsemenyID != e.ID {
			return ErrNotFound
		}
		if err := requireClaimAllowed(ctx, q, e, felhasznaloID, holder); err != nil {
			return err
		}
		if t.MindenkiHozza {
			return ErrTetelMindenkiHozza
		}
		tetelek, err := felszerelesAllas(ctx, q, e.ID)
		if err != nil {
			return err
		}
		for _, allas := range tetelek {
			if allas.ID != t.ID {
				continue
			}
			sajat := 0
			for _, v := range allas.Vallalasok {
				if v.Holder == holder {
					sajat = v.Quantity
				}
			}
			if in.Quantity > allas.Hianyzik+sajat {
				return fmt.Errorf("%w: %d missing", ErrVallalasTooMuch, allas.Hianyzik+sajat)
			}
		}
		return q.SetVallalas(ctx, t.ID, holder, in.Quantity, a.clock.Now())
	})
	if err != nil {
		return err
	}
	a.kiegeszitoChanged(esemenyID)
	return nil
}

// releaseVallalasok is Felszereles's claim releaser: it removes the
// matching Vallalasok and names each freed Tetel with its quantity.
func releaseVallalasok(ctx context.Context, t *kiegeszitoTx, release func(domain.ClaimHolder) bool) ([]string, error) {
	tetelek, err := felszerelesAllas(ctx, t.q, t.e.ID)
	if err != nil {
		return nil, err
	}
	var freed []string
	for _, allas := range tetelek {
		for _, v := range allas.Vallalasok {
			if !release(v.Holder) {
				continue
			}
			if err := t.q.SetVallalas(ctx, allas.ID, v.Holder, 0, t.a.clock.Now()); err != nil {
				return nil, err
			}
			freed = append(freed, fmt.Sprintf("%d db %s", v.Quantity, allas.Name))
		}
	}
	return freed, nil
}

// felszerelesSummary is Felszereles's fragment of the Discord summary line.
func felszerelesSummary(ctx context.Context, t *kiegeszitoTx) (string, error) {
	tetelek, err := felszerelesAllas(ctx, t.q, t.e.ID)
	if err != nil {
		return "", err
	}
	hianyzik, claimable := 0, false
	for _, allas := range tetelek {
		hianyzik += allas.Hianyzik
		claimable = claimable || !allas.MindenkiHozza
	}
	if !claimable {
		return "", nil // nothing to bring that anyone claims
	}
	if hianyzik > 0 {
		return fmt.Sprintf("🎒 %d hiányzik", hianyzik), nil
	}
	return "🎒 minden megvan", nil
}
