package app

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"unicode/utf8"

	"squadsync-be/internal/domain"
	"squadsync-be/internal/store"
)

// Koltsegek is the Kiegeszito for shared money: the Esemeny's Kiadasok,
// each with a payer, an amount in whole HUF, a description and the people
// it is split among (equally, or in custom amounts that add up to it). A
// Kiadas is either tervezett (an estimate) or kifizetve (the real amount);
// only kifizetve ones count in the balances and in the Elszamolas
// (elszamolas.go).
//
// Who may do what:
//   - Kiadasok are recorded, edited and deleted only while the Esemeny is
//     Veglegesitve or Lezajlott (there are no Resztvevok in Egyeztetes, and
//     an Elmaradt Esemeny has no Elszamolas), and only while no Elszamolas
//     has been calculated (kiadasokLocked): requireKiadasokOpen.
//   - Any Resztvevo adds one, naming any Resztvevo as payer.
//   - Its payer, its creator or the Szervezo edits or deletes it, even if
//     they aren't a Resztvevo (any more): a payer who dropped out still
//     fixes their own Kiadas. A new payer or a newly added split member
//     must be a Resztvevo, though.
//
// The split is fixed when the Kiadas is created: Koltsegek has no claim
// releaser, so drop-outs and re-confirmations never touch it, and its
// people stay in it even once they aren't Resztvevok. Only an edit changes
// it. An equal split is stored as computed shares (see egyenloReszek), so
// the Elszamolas only sums kiadas_resz.

func init() {
	registerKiegeszito(kiegeszito{
		kind:          domain.Koltsegek,
		switchable:    koltsegekSwitchable,
		vetoSwitchOff: vetoKoltsegekOff,
		summary:       koltsegekSummary,
	})
}

// Limits on Koltsegek.
const (
	MaxKiadasDescriptionLength = 200
	// MaxKiadasAmount is 100 million HUF.
	MaxKiadasAmount = 100_000_000
	MaxKiadasok     = 200
)

// Errors of Koltsegek.
var (
	// ErrKoltsegekClosed means Kiadasok can't be recorded or changed in the
	// Esemeny's state: only in Veglegesitve and Lezajlott.
	ErrKoltsegekClosed = errors.New("kiadasok can be changed only while the esemeny is veglegesitve or lezajlott")
	// ErrKiadasokLocked means an Elszamolas has been calculated, so the
	// Kiadasok can't change (see elszamolas.go).
	ErrKiadasokLocked = errors.New("the kiadasok are locked by the elszamolas")
	// ErrKiadasNotResztvevo means a Kiadas write names someone who isn't a
	// Resztvevo: the one adding it, a new payer or a new split member. It
	// wraps ErrNotResztvevo.
	ErrKiadasNotResztvevo = fmt.Errorf("%w: a kiadas names someone who isn't a resztvevo", ErrNotResztvevo)
	// ErrKiadasSplitSum means the custom amounts of a split don't add up to
	// the Kiadas's amount.
	ErrKiadasSplitSum = errors.New("the custom amounts don't add up to the kiadas's amount")
)

// KiadasInput is a Kiadas being added or edited.
type KiadasInput struct {
	Description string
	// Amount is in whole HUF.
	Amount int
	// PayerID is the Felhasznalo who pays (or paid); "" means the caller.
	PayerID string
	// Description "" keeps the current one when editing (adding needs one).
	// Allapot "" means Kifizetve when adding and keeps the current one when
	// editing.
	Allapot domain.KiadasAllapot
	// Split is who it is split among and how. nil means, when adding, every
	// Resztvevo equally; when editing, the current split unchanged (an equal
	// one is recomputed for the new amount, custom amounts must still add
	// up to it).
	Split *SplitInput
}

// SplitInput is who a Kiadas is split among.
type SplitInput struct {
	// Holders are the people it is split among. Newly added ones must be
	// Resztvevok; ones already in the Kiadas's split may stay even if they
	// aren't any more.
	Holders []domain.ClaimHolder
	// Custom are custom amounts, one per holder in the same order, adding
	// up to the Kiadas's amount. nil means an equal split.
	Custom []int
}

// Koltsegek is an Esemeny's Koltsegek as one viewer sees it.
type Koltsegek struct {
	// Kiadasok are oldest first.
	Kiadasok []KiadasAllas
	// Egyenlegek are the balances over the kifizetve Kiadasok (see
	// egyenlegekOf), ordered by name.
	Egyenlegek []Egyenleg
	// Tervezett is how many Kiadasok are still only planned.
	Tervezett int
	// CanAdd tells whether the viewer may add a Kiadas now.
	CanAdd bool
	// Elszamolas is the Elszamolas as the viewer sees it, nil while the
	// Esemeny has none (no Kiadas, or Egyeztetes or Elmaradt).
	Elszamolas *Elszamolas
}

// KiadasAllas is a Kiadas with its split.
type KiadasAllas struct {
	domain.Kiadas
	// Reszek are in the stable id order of their holders.
	Reszek []domain.KiadasResz
	// CanEdit tells whether the viewer may edit or delete it now.
	CanEdit bool
}

// Egyenleg is one Felhasznalo's balance over the kifizetve Kiadasok:
// Fizetett is what they paid, Resz their share, Netto = Fizetett - Resz
// (positive: they get money back).
type Egyenleg struct {
	FelhasznaloID string
	Name          string
	Fizetett      int
	Resz          int
	Netto         int
}

// koltsegekSwitchable lets the Szervezo switch Koltsegek in Egyeztetes,
// Veglegesitve and Lezajlott, so a Szervezo who forgot can still switch it
// on after the trip.
func koltsegekSwitchable(e domain.Esemeny) bool {
	return e.Allapot == domain.Egyeztetes || e.Allapot == domain.Veglegesitve || e.Allapot == domain.Lezajlott
}

// vetoKoltsegekOff keeps Koltsegek on while it has any Kiadas, so debts are
// never hidden.
func vetoKoltsegekOff(ctx context.Context, t *kiegeszitoTx) error {
	n, err := t.q.CountKiadasok(ctx, t.e.ID)
	if err != nil {
		return err
	}
	if n > 0 {
		return fmt.Errorf("%w: a Költségekben még %d kiadás van", ErrKiegeszitoInUse, n)
	}
	return nil
}

// koltsegekSummary is Koltsegek's fragment of the Discord summary line.
func koltsegekSummary(ctx context.Context, t *kiegeszitoTx) (string, error) {
	n, err := t.q.CountKiadasok(ctx, t.e.ID)
	if err != nil || n == 0 {
		return "", err
	}
	s := fmt.Sprintf("🧾 %d Kiadás", n)
	elszamolas, err := elszamolasSummary(ctx, t.q, t.e)
	if err != nil {
		return "", err
	}
	if elszamolas != "" {
		s += " · " + elszamolas
	}
	return s, nil
}

// requireKiadasokOpen is Koltsegek's guard (see the top of kiegeszito.go):
// the Esemeny is Veglegesitve or Lezajlott (ErrKoltsegekClosed) and no
// Elszamolas locks the Kiadasok (ErrKiadasokLocked). Who may write is
// checked on top of it: a Resztvevo adds (AddKiadas), the payer, the
// creator or the Szervezo edits and deletes (editableKiadas).
func requireKiadasokOpen(ctx context.Context, q store.Queries, e domain.Esemeny) error {
	if !hasResztvevok(e.Allapot) {
		return ErrKoltsegekClosed
	}
	locked, err := kiadasokLocked(ctx, q, e.ID)
	if err != nil {
		return err
	}
	if locked {
		return ErrKiadasokLocked
	}
	return nil
}

// kiadasokOpen is requireKiadasokOpen as a yes or no, for views.
func kiadasokOpen(ctx context.Context, q store.Queries, e domain.Esemeny) (bool, error) {
	err := requireKiadasokOpen(ctx, q, e)
	switch {
	case err == nil:
		return true, nil
	case errors.Is(err, ErrKoltsegekClosed), errors.Is(err, ErrKiadasokLocked):
		return false, nil
	default:
		return false, err
	}
}

// koltsegekResztvevok returns the Esemeny's Resztvevok.
func koltsegekResztvevok(ctx context.Context, q store.Queries, e domain.Esemeny) ([]domain.Resztvevo, error) {
	vs, err := q.ListVisszajelzesekOfEsemeny(ctx, e.ID)
	if err != nil {
		return nil, err
	}
	return resztvevokOf(e, vs), nil
}

func holdsResztvevo(resztvevok []domain.Resztvevo, h domain.ClaimHolder) bool {
	for _, r := range resztvevok {
		if domain.HolderOf(r) == h {
			return true
		}
	}
	return false
}

// mayEditKiadas tells whether the Felhasznalo is the Kiadas's payer, its
// creator or the Szervezo.
func mayEditKiadas(e domain.Esemeny, k domain.Kiadas, felhasznaloID string) bool {
	return k.PayerID == felhasznaloID || k.CreatedBy == felhasznaloID || e.SzervezoID == felhasznaloID
}

// Koltsegek returns the Esemeny's Kiadasok and balances. A non-Tag gets
// ErrNotFound; while it is switched off, ErrKiegeszitoOff. Any Tag can read
// it, in every state.
func (a *App) Koltsegek(ctx context.Context, felhasznaloID, esemenyID string) (Koltsegek, error) {
	var out Koltsegek
	// One transaction, so the Kiadasok and their shares are read together.
	err := a.store.InTx(ctx, func(q store.Queries) error {
		e, err := requireKiegeszitoOn(ctx, q, esemenyID, felhasznaloID, domain.Koltsegek)
		if err != nil {
			return err
		}
		kiadasok, err := koltsegekAllas(ctx, q, e.ID)
		if err != nil {
			return err
		}
		egyenlegek, err := egyenlegekOf(kiadasok)
		if err != nil {
			return err
		}
		open, err := kiadasokOpen(ctx, q, e)
		if err != nil {
			return err
		}
		s, err := readElszamolas(ctx, q, e)
		if err != nil {
			return err
		}
		elszamolas, err := s.view(ctx, q, felhasznaloID)
		if err != nil {
			return err
		}
		out = Koltsegek{
			Kiadasok:   kiadasok,
			Egyenlegek: egyenlegek,
			CanAdd:     open && holdsResztvevo(s.resztvevok, domain.ClaimHolder{FelhasznaloID: felhasznaloID}),
			Elszamolas: elszamolas,
		}
		for n := range out.Kiadasok {
			k := &out.Kiadasok[n]
			k.CanEdit = open && mayEditKiadas(e, k.Kiadas, felhasznaloID)
			if k.Allapot == domain.Tervezett {
				out.Tervezett++
			}
		}
		return nil
	})
	if err != nil {
		return Koltsegek{}, err
	}
	return out, nil
}

// koltsegekAllas reads the Esemeny's Kiadasok with their splits.
func koltsegekAllas(ctx context.Context, q store.Queries, esemenyID string) ([]KiadasAllas, error) {
	kiadasok, err := q.ListKiadasok(ctx, esemenyID)
	if err != nil {
		return nil, err
	}
	reszek, err := q.ListKiadasReszekOfEsemeny(ctx, esemenyID)
	if err != nil {
		return nil, err
	}
	out := make([]KiadasAllas, len(kiadasok))
	index := make(map[string]int, len(kiadasok))
	for n, k := range kiadasok {
		out[n] = KiadasAllas{Kiadas: k, Reszek: []domain.KiadasResz{}}
		index[k.ID] = n
	}
	for _, r := range reszek {
		if n, ok := index[r.KiadasID]; ok {
			out[n].Reszek = append(out[n].Reszek, r)
		}
	}
	return out, nil
}

// egyenlegekOf computes everyone's balance over the kifizetve Kiadasok:
// what they paid, their share and the net (paid - share). Tervezett ones
// don't count. It never returns nil; the Netto values add up to 0. The
// Elszamolas turns these into Utalasok (utalasokOf).
func egyenlegekOf(kiadasok []KiadasAllas) ([]Egyenleg, error) {
	byID := map[string]*Egyenleg{}
	get := func(id, name string) *Egyenleg {
		if b, ok := byID[id]; ok {
			if b.Name == "" {
				b.Name = name
			}
			return b
		}
		b := &Egyenleg{FelhasznaloID: id, Name: name}
		byID[id] = b
		return b
	}
	for _, k := range kiadasok {
		if k.Allapot != domain.Kifizetve {
			continue
		}
		get(k.PayerID, k.PayerName).Fizetett += k.Amount
		for _, r := range k.Reszek {
			if r.Holder.VendegID != "" {
				// #13: a name-only Vendeg's share is charged to the Tag who
				// added them: map r.Holder.VendegID to that Tag's
				// Felhasznalo id (and name) here instead of refusing it.
				// Until then resolveSplit never stores such a share.
				return nil, fmt.Errorf("kiadas %s: a vendeg share can't be charged to their tag before #13", k.ID)
			}
			get(r.Holder.FelhasznaloID, r.Name).Resz += r.Amount
		}
	}
	out := make([]Egyenleg, 0, len(byID))
	for _, b := range byID {
		b.Netto = b.Fizetett - b.Resz
		out = append(out, *b)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Name != out[j].Name {
			return out[i].Name < out[j].Name
		}
		return out[i].FelhasznaloID < out[j].FelhasznaloID
	})
	return out, nil
}

// holderLess is the stable id order of holders: Felhasznalok by id, then
// name-only Vendegs by id. Equal splits hand out their extra forints in it.
func holderLess(a, b domain.ClaimHolder) bool {
	if (a.FelhasznaloID == "") != (b.FelhasznaloID == "") {
		return a.FelhasznaloID != ""
	}
	if a.FelhasznaloID != b.FelhasznaloID {
		return a.FelhasznaloID < b.FelhasznaloID
	}
	return a.VendegID < b.VendegID
}

// egyenloReszek splits amount equally in whole forints: everyone gets
// amount / n, and the remaining forints go one each to the first holders in
// the stable id order (1000 among three: 334, 333, 333).
func egyenloReszek(amount int, holders []domain.ClaimHolder) []domain.KiadasResz {
	sorted := append([]domain.ClaimHolder(nil), holders...)
	sort.Slice(sorted, func(i, j int) bool { return holderLess(sorted[i], sorted[j]) })
	n := len(sorted)
	out := make([]domain.KiadasResz, n)
	for i, h := range sorted {
		share := amount / n
		if i < amount%n {
			share++
		}
		out[i] = domain.KiadasResz{Holder: h, Amount: share}
	}
	return out
}

// resolveSplit turns a SplitInput into the shares of a Kiadas of the given
// amount. previous is the Kiadas's current split (nil when adding), whose
// holders may stay in it even if they aren't Resztvevok any more.
func resolveSplit(in *SplitInput, amount int, resztvevok []domain.Resztvevo, previous []domain.KiadasResz, prevEgyenlo bool) ([]domain.KiadasResz, bool, error) {
	switch {
	case in == nil && previous == nil:
		// Every Resztvevo equally, checked like any other split (so a
		// name-only Vendeg is refused until #13).
		in = &SplitInput{}
		for _, r := range resztvevok {
			in.Holders = append(in.Holders, domain.HolderOf(r))
		}
	case in == nil:
		in = &SplitInput{}
		for _, r := range previous {
			in.Holders = append(in.Holders, r.Holder)
			if !prevEgyenlo {
				in.Custom = append(in.Custom, r.Amount)
			}
		}
	}
	if len(in.Holders) == 0 {
		return nil, false, fmt.Errorf("%w: a kiadas is split among at least one person", ErrInvalidInput)
	}
	if in.Custom != nil && len(in.Custom) != len(in.Holders) {
		return nil, false, fmt.Errorf("%w: one custom amount per person", ErrInvalidInput)
	}
	kept := map[domain.ClaimHolder]bool{}
	for _, r := range previous {
		kept[r.Holder] = true
	}
	seen := map[domain.ClaimHolder]bool{}
	for _, h := range in.Holders {
		if h.FelhasznaloID == "" && h.VendegID == "" || h.FelhasznaloID != "" && h.VendegID != "" {
			return nil, false, fmt.Errorf("%w: a share is for a felhasznalo or a vendeg", ErrInvalidInput)
		}
		if seen[h] {
			return nil, false, fmt.Errorf("%w: someone is in the split twice", ErrInvalidInput)
		}
		seen[h] = true
		if kept[h] {
			continue
		}
		if h.VendegID != "" {
			// #13 checks the name-only Vendeg here (a Resztvevo of the
			// Esemeny); until then there is none.
			return nil, false, fmt.Errorf("%w: no such vendeg", ErrNotFound)
		}
		if !holdsResztvevo(resztvevok, h) {
			return nil, false, ErrKiadasNotResztvevo
		}
	}
	if in.Custom == nil {
		return egyenloReszek(amount, in.Holders), true, nil
	}
	reszek := make([]domain.KiadasResz, len(in.Holders))
	sum := 0
	for i, h := range in.Holders {
		c := in.Custom[i]
		if c < 0 || c > MaxKiadasAmount {
			return nil, false, fmt.Errorf("%w: a share is 0 to %d", ErrInvalidInput, MaxKiadasAmount)
		}
		sum += c
		reszek[i] = domain.KiadasResz{Holder: h, Amount: c}
	}
	if sum != amount {
		return nil, false, fmt.Errorf("%w: %d instead of %d", ErrKiadasSplitSum, sum, amount)
	}
	sort.Slice(reszek, func(i, j int) bool { return holderLess(reszek[i].Holder, reszek[j].Holder) })
	return reszek, false, nil
}

// validKiadas checks a KiadasInput; when editing, an empty description
// means the current one.
func validKiadas(in KiadasInput, editing bool) (KiadasInput, error) {
	in.Description = strings.TrimSpace(in.Description)
	if in.Description == "" && !editing {
		return KiadasInput{}, fmt.Errorf("%w: a kiadas needs a description", ErrInvalidInput)
	}
	if utf8.RuneCountInString(in.Description) > MaxKiadasDescriptionLength {
		return KiadasInput{}, fmt.Errorf("%w: a kiadas description is at most %d characters", ErrInvalidInput, MaxKiadasDescriptionLength)
	}
	if in.Amount < 1 || in.Amount > MaxKiadasAmount {
		return KiadasInput{}, fmt.Errorf("%w: a kiadas is 1 to %d Ft", ErrInvalidInput, MaxKiadasAmount)
	}
	if in.Allapot != "" && !in.Allapot.Valid() {
		return KiadasInput{}, fmt.Errorf("%w: unknown kiadas allapot %q", ErrInvalidInput, in.Allapot)
	}
	return in, nil
}

// AddKiadas records a Kiadas. Besides the errors of requireKiegeszitoOn and
// requireKiadasokOpen: the caller, the payer or a split member not being a
// Resztvevo gets ErrKiadasNotResztvevo, custom amounts that don't add up ErrKiadasSplitSum and
// other bad input ErrInvalidInput. #21's fuel Kiadas comes through here
// with the Auto's people as Split.Holders.
func (a *App) AddKiadas(ctx context.Context, felhasznaloID, esemenyID string, in KiadasInput) (domain.Kiadas, error) {
	in, err := validKiadas(in, false)
	if err != nil {
		return domain.Kiadas{}, err
	}
	if in.PayerID == "" {
		in.PayerID = felhasznaloID
	}
	if in.Allapot == "" {
		in.Allapot = domain.Kifizetve
	}
	var k domain.Kiadas
	err = a.store.InTx(ctx, func(q store.Queries) error {
		e, err := requireKiegeszitoOn(ctx, q, esemenyID, felhasznaloID, domain.Koltsegek)
		if err != nil {
			return err
		}
		if err := requireKiadasokOpen(ctx, q, e); err != nil {
			return err
		}
		resztvevok, err := koltsegekResztvevok(ctx, q, e)
		if err != nil {
			return err
		}
		if !holdsResztvevo(resztvevok, domain.ClaimHolder{FelhasznaloID: felhasznaloID}) ||
			!holdsResztvevo(resztvevok, domain.ClaimHolder{FelhasznaloID: in.PayerID}) {
			return ErrKiadasNotResztvevo
		}
		reszek, egyenlo, err := resolveSplit(in.Split, in.Amount, resztvevok, nil, false)
		if err != nil {
			return err
		}
		n, err := q.CountKiadasok(ctx, e.ID)
		if err != nil {
			return err
		}
		if n >= MaxKiadasok {
			return fmt.Errorf("%w: at most %d kiadasok", ErrInvalidInput, MaxKiadasok)
		}
		now := a.clock.Now()
		k = domain.Kiadas{
			ID:          randomID(),
			EsemenyID:   e.ID,
			Description: in.Description,
			Amount:      in.Amount,
			PayerID:     in.PayerID,
			Allapot:     in.Allapot,
			Egyenlo:     egyenlo,
			CreatedBy:   felhasznaloID,
			CreatedAt:   now,
			UpdatedAt:   now,
		}
		if err := q.InsertKiadas(ctx, k); err != nil {
			return err
		}
		if err := unconfirmTervezettPayer(ctx, q, e.ID, k); err != nil {
			return err
		}
		return q.SetKiadasReszek(ctx, k.ID, reszek)
	})
	if err != nil {
		return domain.Kiadas{}, err
	}
	a.kiegeszitoChanged(esemenyID)
	return k, nil
}

// editableKiadas returns the Esemeny, the Kiadas, its split and the
// Resztvevok if the Felhasznalo may edit or delete the Kiadas now: while
// requireKiadasokOpen allows it, as its payer, its creator or the Szervezo
// (ErrNotPermitted otherwise), Resztvevo or not.
func editableKiadas(ctx context.Context, q store.Queries, esemenyID, kiadasID, felhasznaloID string) (domain.Esemeny, KiadasAllas, []domain.Resztvevo, error) {
	e, err := requireKiegeszitoOn(ctx, q, esemenyID, felhasznaloID, domain.Koltsegek)
	if err != nil {
		return domain.Esemeny{}, KiadasAllas{}, nil, err
	}
	k, err := q.FindKiadas(ctx, kiadasID)
	if err != nil {
		return domain.Esemeny{}, KiadasAllas{}, nil, notFound(err)
	}
	if k.EsemenyID != e.ID {
		return domain.Esemeny{}, KiadasAllas{}, nil, ErrNotFound
	}
	if err := requireKiadasokOpen(ctx, q, e); err != nil {
		return domain.Esemeny{}, KiadasAllas{}, nil, err
	}
	resztvevok, err := koltsegekResztvevok(ctx, q, e)
	if err != nil {
		return domain.Esemeny{}, KiadasAllas{}, nil, err
	}
	if !mayEditKiadas(e, k, felhasznaloID) {
		return domain.Esemeny{}, KiadasAllas{}, nil, ErrNotPermitted
	}
	all, err := koltsegekAllas(ctx, q, e.ID)
	if err != nil {
		return domain.Esemeny{}, KiadasAllas{}, nil, err
	}
	for _, allas := range all {
		if allas.ID == k.ID {
			return e, allas, resztvevok, nil
		}
	}
	return domain.Esemeny{}, KiadasAllas{}, nil, ErrNotFound
}

// UpdateKiadas changes a Kiadas: its description, amount, payer, state
// (tervezett and kifizetve both ways, such as marking it paid with the real
// amount) and, with in.Split, its split. Its payer, its creator or the
// Szervezo may, even if they aren't a Resztvevo (ErrNotPermitted for
// anyone else). A new payer or a newly added split member must be a
// Resztvevo (ErrKiadasNotResztvevo); the current ones may stay. The other
// errors are those of AddKiadas.
func (a *App) UpdateKiadas(ctx context.Context, felhasznaloID, esemenyID, kiadasID string, in KiadasInput) error {
	in, err := validKiadas(in, true)
	if err != nil {
		return err
	}
	err = a.store.InTx(ctx, func(q store.Queries) error {
		e, k, resztvevok, err := editableKiadas(ctx, q, esemenyID, kiadasID, felhasznaloID)
		if err != nil {
			return err
		}
		if in.PayerID == "" {
			in.PayerID = k.PayerID
		}
		if in.PayerID != k.PayerID && !holdsResztvevo(resztvevok, domain.ClaimHolder{FelhasznaloID: in.PayerID}) {
			return ErrKiadasNotResztvevo
		}
		if in.Description == "" {
			in.Description = k.Description
		}
		if in.Allapot == "" {
			in.Allapot = k.Allapot
		}
		reszek, egyenlo, err := resolveSplit(in.Split, in.Amount, resztvevok, k.Reszek, k.Egyenlo)
		if err != nil {
			return err
		}
		updated := k.Kiadas
		updated.Description, updated.Amount, updated.PayerID = in.Description, in.Amount, in.PayerID
		updated.Allapot, updated.Egyenlo, updated.UpdatedAt = in.Allapot, egyenlo, a.clock.Now()
		if err := q.UpdateKiadas(ctx, updated); err != nil {
			return err
		}
		if err := unconfirmTervezettPayer(ctx, q, e.ID, updated); err != nil {
			return err
		}
		return q.SetKiadasReszek(ctx, k.ID, reszek)
	})
	if err != nil {
		return err
	}
	a.kiegeszitoChanged(esemenyID)
	return nil
}

// DeleteKiadas deletes a Kiadas with its split. A non-Tag, or a Kiadas of
// another Esemeny, gets ErrNotFound; anyone but its payer, its creator and
// the Szervezo ErrNotPermitted; plus the errors of requireKiegeszitoOn and
// requireKiadasokOpen.
func (a *App) DeleteKiadas(ctx context.Context, felhasznaloID, esemenyID, kiadasID string) error {
	err := a.store.InTx(ctx, func(q store.Queries) error {
		_, k, _, err := editableKiadas(ctx, q, esemenyID, kiadasID, felhasznaloID)
		if err != nil {
			return err
		}
		return q.DeleteKiadas(ctx, k.ID)
	})
	if err != nil {
		return err
	}
	a.kiegeszitoChanged(esemenyID)
	return nil
}
