package app

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"time"

	"squadsync-be/internal/domain"
	"squadsync-be/internal/gateway"
	"squadsync-be/internal/store"
)

// The Elszamolas of an Esemeny's Koltsegek (#22): the one-time calculation
// of the Utalasok that settle its kifizetve Kiadasok.
//
// # When there is one
//
// Only while Koltsegek is on (every use case starts with
// requireKiegeszitoOn) and the Esemeny has at least one Kiadas. It opens
// when the Esemeny is Lezajlott; before that (Veglegesitve) the web shows
// that it is coming. An Elmaradt Esemeny never becomes Lezajlott, so it
// never has one, and Egyeztetes has no Kiadasok.
//
// # Confirming
//
// Each Resztvevo presses "Rögzítettem a kiadásaimat"
// (ConfirmKiadasokRecorded), and may take it back ("Mégsem",
// UnconfirmKiadasokRecorded) until the calculation runs. A Vendeg who is a
// Felhasznalo confirms for themselves; a Tag's press confirms (and takes
// back) their name-only Vendegek too, even if the Tag isn't a Resztvevo. Nobody confirms
// while they are the payer of a tervezett Kiadas (ErrElszamolasTervezett),
// and recording or editing a Kiadas so that someone is the payer of a
// tervezett one takes their confirmation back. Kiadasok can still be
// recorded and edited while confirmations are collected. Three days after
// Lezajlott, everyone who hasn't confirmed yet gets one reminder DM
// (runElszamolasEmlekezteto), unless it was calculated by then.
//
// Who must confirm are the Resztvevok as they are now. In Lezajlott nobody
// can answer any more, so they change only when a Tag leaves the Csoport;
// if that leaves everyone confirmed, nothing is calculated by itself, and
// the Szervezo closes it.
//
// # The calculation
//
// It runs exactly once, inside the transaction of the action that triggers
// it: the last missing confirmation, or the Szervezo closing it without the
// missing ones (CloseElszamolas). It turns the balances of egyenlegekOf
// (kifizetve Kiadasok only: a tervezett one at that moment is ignored, and
// the web warns about it) into Utalasok (utalasokOf). While a calculated
// Elszamolas exists, kiadasokLocked locks every Kiadas write. Each write
// checks it in its own BEGIN IMMEDIATE transaction, as the calculation
// stores it in one, so SQLite's write lock serialises them: an edit either
// commits before the calculation (which then counts it) or is refused.
//
// After the commit every Felhasznalo with a balance and every Resztvevo
// gets one DM with their result: whom they pay, with a "megvolt" button
// each, or who pays them. Those with nothing to pay or get get a short
// "rendben vagy" DM, so everyone knows it is done.
//
// # Afterwards
//
// Either party of an Utalas, or the Szervezo, marks it megvolt
// (SetUtalasMegvolt), and may take that back until every Utalas is
// megvolt: then the Esemeny is "Elszamolva ✓" (on the web and in the
// summary line), and the megvolt marks can't be taken back any more. With
// no Utalas at all (everyone square), it is Elszamolva at once, and since
// nothing is megvolt the Szervezo can still reopen it.
//
// The Szervezo can reopen it (ReopenElszamolas) only while no Utalas is
// megvolt: the Utalasok are deleted, the Kiadasok unlock, and every
// confirmation is cleared, so everyone confirms again.

// ElszamolasAllapot is where an Esemeny's Elszamolas stands.
type ElszamolasAllapot string

// The states of an Elszamolas.
const (
	// ElszamolasLezajlasElott: the Esemeny has Kiadasok but isn't
	// Lezajlott yet, so confirming hasn't opened.
	ElszamolasLezajlasElott ElszamolasAllapot = "lezajlas_elott"
	// ElszamolasMegerosites: Lezajlott, collecting the confirmations.
	ElszamolasMegerosites ElszamolasAllapot = "megerosites"
	// ElszamolasKiszamolva: calculated, some Utalas isn't megvolt yet.
	ElszamolasKiszamolva ElszamolasAllapot = "kiszamolva"
	// ElszamolasElszamolva: calculated and every Utalas is megvolt.
	ElszamolasElszamolva ElszamolasAllapot = "elszamolva"
)

// elszamolasEmlekeztetoDelay is how long after Lezajlott the unconfirmed
// get their reminder.
const elszamolasEmlekeztetoDelay = 3 * 24 * time.Hour

// Errors of the Elszamolas.
var (
	// ErrElszamolasNotOpen means there is nothing to confirm or close: the
	// Esemeny isn't Lezajlott, or it has no Kiadas.
	ErrElszamolasNotOpen = errors.New("the elszamolas opens once the esemeny is lezajlott and has a kiadas")
	// ErrElszamolasTervezett means the Felhasznalo can't confirm while they
	// are the payer of a tervezett Kiadas.
	ErrElszamolasTervezett = errors.New("the payer of a tervezett kiadas can't confirm: mark it paid or delete it")
	// ErrElszamolasNotResztvevo means only a Resztvevo confirms. It wraps
	// ErrNotResztvevo.
	ErrElszamolasNotResztvevo = fmt.Errorf("%w: only resztvevok confirm their kiadasok", ErrNotResztvevo)
	// ErrElszamolasKiszamolva means the Elszamolas is already calculated,
	// so confirmations can't change and it can't be closed again.
	ErrElszamolasKiszamolva = errors.New("the elszamolas is already calculated")
	// ErrElszamolasNotKiszamolva means there is no calculated Elszamolas
	// to reopen.
	ErrElszamolasNotKiszamolva = errors.New("the elszamolas isn't calculated")
	// ErrElszamolasMegvolt means the Elszamolas can't be reopened because
	// an Utalas is already marked megvolt.
	ErrElszamolasMegvolt = errors.New("the elszamolas can't be reopened once an utalas is megvolt")
	// ErrElszamolva means every Utalas is megvolt, so no megvolt mark can
	// be taken back.
	ErrElszamolva = errors.New("every utalas is megvolt: the esemeny is elszamolva")
)

// Elszamolas is an Esemeny's Elszamolas as one viewer sees it.
type Elszamolas struct {
	Allapot ElszamolasAllapot
	// Megerositettek and Hianyzok are the Resztvevok who have and haven't
	// confirmed their Kiadasok, ordered by name. Hianyzok is empty once it
	// is calculated.
	Megerositettek []ElszamolasSzemely
	Hianyzok       []ElszamolasSzemely
	// Megerositette tells whether the viewer has confirmed (for themselves
	// and every name-only Vendeg of theirs who is a Resztvevo).
	Megerositette bool
	// CanConfirm tells whether the viewer may confirm now (for themselves or
	// their name-only Vendegek), leaving aside TervezettFizeto.
	CanConfirm bool
	// TervezettFizeto are the descriptions of the tervezett Kiadasok the
	// viewer pays, which block their confirming.
	TervezettFizeto []string
	// CanUnconfirm tells whether the viewer may take their confirmation
	// back.
	CanUnconfirm bool
	// CanClose tells whether the viewer (the Szervezo) may close it now.
	CanClose bool
	// CanReopen tells whether the viewer (the Szervezo) may reopen it.
	CanReopen bool
	// KiszamolvaAt is when it was calculated, nil before.
	KiszamolvaAt *time.Time
	// LezartaName is the Szervezo who closed it without every
	// confirmation, "" otherwise.
	LezartaName string
	// Utalasok are the calculated transfers, in the calculation's order.
	Utalasok []UtalasAllas
}

// ElszamolasSzemely is a Resztvevo in the Elszamolas's confirmation list.
type ElszamolasSzemely struct {
	Holder domain.ClaimHolder
	Name   string
}

// UtalasAllas is an Utalas as one viewer sees it.
type UtalasAllas struct {
	domain.Utalas
	// CanMark tells whether the viewer may mark it megvolt now, or take
	// that back if it is.
	CanMark bool
}

// elszamolasAllas is the state of an Esemeny's Elszamolas read inside a
// transaction. e must have Koltsegek on.
type elszamolasAllas struct {
	e domain.Esemeny
	// kiadasok are its Kiadasok with their splits.
	kiadasok []KiadasAllas
	// kiszamolva is the calculated Elszamolas, nil before.
	kiszamolva    *domain.Elszamolas
	utalasok      []domain.Utalas
	resztvevok    []domain.Resztvevo
	megerositesek map[domain.ClaimHolder]bool
}

func readElszamolas(ctx context.Context, q store.Queries, e domain.Esemeny) (elszamolasAllas, error) {
	s := elszamolasAllas{e: e}
	var err error
	if s.kiadasok, err = koltsegekAllas(ctx, q, e.ID); err != nil {
		return s, err
	}
	if s.resztvevok, err = koltsegekResztvevok(ctx, q, e); err != nil {
		return s, err
	}
	if s.megerositesek, err = q.ListElszamolasMegerositesek(ctx, e.ID); err != nil {
		return s, err
	}
	k, err := q.FindElszamolas(ctx, e.ID)
	switch {
	case errors.Is(err, store.ErrNotFound):
	case err != nil:
		return s, err
	default:
		s.kiszamolva = &k
		if s.utalasok, err = q.ListUtalasok(ctx, e.ID); err != nil {
			return s, err
		}
	}
	return s, nil
}

// exists tells whether the Esemeny has an Elszamolas at all: Kiadasok, and
// a state with Resztvevok (Veglegesitve or Lezajlott).
func (s elszamolasAllas) exists() bool {
	return len(s.kiadasok) > 0 && hasResztvevok(s.e.Allapot)
}

// open tells whether confirmations are collected now.
func (s elszamolasAllas) open() bool {
	return len(s.kiadasok) > 0 && s.e.Allapot == domain.Lezajlott && s.kiszamolva == nil
}

// requireOpen refuses with ErrElszamolasNotOpen or ErrElszamolasKiszamolva
// unless confirmations are collected now.
func (s elszamolasAllas) requireOpen() error {
	if len(s.kiadasok) == 0 || s.e.Allapot != domain.Lezajlott {
		return ErrElszamolasNotOpen
	}
	if s.kiszamolva != nil {
		return ErrElszamolasKiszamolva
	}
	return nil
}

// hianyzok are the Resztvevok who haven't confirmed. A name-only Vendeg is
// confirmed by their Tag (see ConfirmKiadasokRecorded).
func (s elszamolasAllas) hianyzok() []domain.Resztvevo {
	out := []domain.Resztvevo{}
	for _, r := range s.resztvevok {
		if !s.megerositesek[domain.HolderOf(r)] {
			out = append(out, r)
		}
	}
	return out
}

// elszamolva tells whether every Utalas of the calculated Elszamolas is
// megvolt.
func (s elszamolasAllas) elszamolva() bool {
	if s.kiszamolva == nil {
		return false
	}
	for _, u := range s.utalasok {
		if u.MegvoltAt == nil {
			return false
		}
	}
	return true
}

func (s elszamolasAllas) anyMegvolt() bool {
	for _, u := range s.utalasok {
		if u.MegvoltAt != nil {
			return true
		}
	}
	return false
}

// confirmsFor are the holders the Felhasznalo's press confirms: themselves
// if they are a Resztvevo, and the name-only Vendegek they added who are.
func (s elszamolasAllas) confirmsFor(felhasznaloID string) []domain.ClaimHolder {
	var out []domain.ClaimHolder
	for _, r := range s.resztvevok {
		if r.FelhasznaloID == felhasznaloID || (r.VendegID != "" && r.AddedBy == felhasznaloID) {
			out = append(out, domain.HolderOf(r))
		}
	}
	return out
}

// tervezettOf returns the descriptions of the tervezett Kiadasok the
// Felhasznalo pays.
func (s elszamolasAllas) tervezettOf(felhasznaloID string) []string {
	out := []string{}
	for _, k := range s.kiadasok {
		if k.Allapot == domain.Tervezett && k.PayerID == felhasznaloID {
			out = append(out, k.Description)
		}
	}
	return out
}

// mayMarkUtalas tells whether the Felhasznalo is a party of the Utalas or
// the Szervezo.
func mayMarkUtalas(e domain.Esemeny, u domain.Utalas, felhasznaloID string) bool {
	return u.FromID == felhasznaloID || u.ToID == felhasznaloID || e.SzervezoID == felhasznaloID
}

// view is the Elszamolas as the viewer sees it, nil when the Esemeny has
// none.
func (s elszamolasAllas) view(ctx context.Context, q store.Queries, felhasznaloID string) (*Elszamolas, error) {
	if !s.exists() {
		return nil, nil
	}
	mine := s.confirmsFor(felhasznaloID)
	confirmed, unconfirmed := 0, 0
	for _, h := range mine {
		if s.megerositesek[h] {
			confirmed++
		} else {
			unconfirmed++
		}
	}
	out := &Elszamolas{
		Allapot:         ElszamolasLezajlasElott,
		Megerositettek:  []ElszamolasSzemely{},
		Hianyzok:        []ElszamolasSzemely{},
		Megerositette:   len(mine) > 0 && unconfirmed == 0,
		TervezettFizeto: s.tervezettOf(felhasznaloID),
		Utalasok:        []UtalasAllas{},
	}
	isSzervezo := s.e.SzervezoID == felhasznaloID
	switch {
	case s.kiszamolva != nil:
		out.Allapot = ElszamolasKiszamolva
		if s.elszamolva() {
			out.Allapot = ElszamolasElszamolva
		}
		at := s.kiszamolva.KiszamolvaAt
		out.KiszamolvaAt = &at
		if s.kiszamolva.LezartaID != "" {
			f, err := q.FindFelhasznalo(ctx, s.kiszamolva.LezartaID)
			if err != nil {
				return nil, err
			}
			out.LezartaName = f.Name
		}
		out.CanReopen = isSzervezo && !s.anyMegvolt()
		settled := s.elszamolva()
		for _, u := range s.utalasok {
			out.Utalasok = append(out.Utalasok, UtalasAllas{
				Utalas:  u,
				CanMark: mayMarkUtalas(s.e, u, felhasznaloID) && !settled,
			})
		}
	case s.open():
		out.Allapot = ElszamolasMegerosites
		out.CanConfirm = unconfirmed > 0
		out.CanUnconfirm = confirmed > 0
		out.CanClose = isSzervezo
	}
	for _, r := range s.resztvevok {
		p := ElszamolasSzemely{Holder: domain.HolderOf(r), Name: r.Name}
		if s.megerositesek[p.Holder] {
			out.Megerositettek = append(out.Megerositettek, p)
		} else if s.kiszamolva == nil {
			out.Hianyzok = append(out.Hianyzok, p)
		}
	}
	return out, nil
}

// kiadasokLocked tells whether the Esemeny's Kiadasok are locked because an
// Elszamolas has been calculated (and not reopened).
func kiadasokLocked(ctx context.Context, q store.Queries, esemenyID string) (bool, error) {
	return q.HasElszamolas(ctx, esemenyID)
}

// unconfirmTervezettPayer takes back the confirmation of the payer of a
// tervezett Kiadas, in the transaction that recorded or edited it: they
// can't be confirmed while they pay a planned one.
func unconfirmTervezettPayer(ctx context.Context, q store.Queries, esemenyID string, k domain.Kiadas) error {
	if k.Allapot != domain.Tervezett {
		return nil
	}
	return q.DeleteElszamolasMegerosites(ctx, esemenyID, domain.ClaimHolder{FelhasznaloID: k.PayerID})
}

// ConfirmKiadasokRecorded is the Felhasznalo's "Rögzítettem a
// kiadásaimat", for themselves (a Tag or a Vendeg) and the name-only
// Vendegek they added. If they were the last ones missing, the Elszamolas
// is calculated in the same transaction. Besides the errors of
// requireKiegeszitoOn: ErrElszamolasNotOpen before Lezajlott or without
// Kiadasok, ErrElszamolasKiszamolva once calculated,
// ErrElszamolasNotResztvevo for anyone who is no Resztvevo and added none,
// and
// ErrElszamolasTervezett while they pay a tervezett Kiadas. Confirming
// again changes nothing.
func (a *App) ConfirmKiadasokRecorded(ctx context.Context, felhasznaloID, esemenyID string) error {
	var after []kiegeszitoJob
	err := a.store.InTx(ctx, func(q store.Queries) error {
		e, err := requireKiegeszitoOn(ctx, q, esemenyID, felhasznaloID, domain.Koltsegek)
		if err != nil {
			return err
		}
		s, err := readElszamolas(ctx, q, e)
		if err != nil {
			return err
		}
		if err := s.requireOpen(); err != nil {
			return err
		}
		mine := s.confirmsFor(felhasznaloID)
		if len(mine) == 0 {
			return ErrElszamolasNotResztvevo
		}
		// A name-only Vendeg pays nothing, so only the Tag's own tervezett
		// Kiadasok block it.
		if len(s.tervezettOf(felhasznaloID)) > 0 {
			return ErrElszamolasTervezett
		}
		// The press confirms the Felhasznalo and their name-only Vendegek,
		// each recorded as confirmed by them.
		for _, h := range mine {
			if err := q.InsertElszamolasMegerosites(ctx, e.ID, h, felhasznaloID, a.clock.Now()); err != nil {
				return err
			}
			s.megerositesek[h] = true
		}
		if len(s.hianyzok()) > 0 {
			return nil
		}
		job, err := a.calculateElszamolas(ctx, q, s, "")
		if err != nil {
			return err
		}
		after = append(after, job)
		return nil
	})
	if err != nil {
		return err
	}
	a.kiegeszitoChanged(esemenyID, after...)
	return nil
}

// UnconfirmKiadasokRecorded takes the Felhasznalo's confirmation back
// ("Mégsem"), until the Elszamolas is calculated (ErrElszamolasKiszamolva).
// Anyone but a Resztvevo gets ErrElszamolasNotResztvevo; the other errors
// are those of ConfirmKiadasokRecorded. Taking back a confirmation that
// isn't there changes nothing.
func (a *App) UnconfirmKiadasokRecorded(ctx context.Context, felhasznaloID, esemenyID string) error {
	err := a.store.InTx(ctx, func(q store.Queries) error {
		e, err := requireKiegeszitoOn(ctx, q, esemenyID, felhasznaloID, domain.Koltsegek)
		if err != nil {
			return err
		}
		s, err := readElszamolas(ctx, q, e)
		if err != nil {
			return err
		}
		if err := s.requireOpen(); err != nil {
			return err
		}
		mine := s.confirmsFor(felhasznaloID)
		if len(mine) == 0 {
			return ErrElszamolasNotResztvevo
		}
		// Their name-only Vendegek's confirmations go with theirs.
		for _, h := range mine {
			if err := q.DeleteElszamolasMegerosites(ctx, e.ID, h); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return err
	}
	a.kiegeszitoChanged(esemenyID)
	return nil
}

// CloseElszamolas is the Szervezo calculating the Elszamolas without the
// missing confirmations (the web names them first). Only the Szervezo may
// (ErrNotSzervezo); the other errors are those of ConfirmKiadasokRecorded.
// The tervezett Kiadasok are left out of it.
func (a *App) CloseElszamolas(ctx context.Context, felhasznaloID, esemenyID string) error {
	var job kiegeszitoJob
	err := a.store.InTx(ctx, func(q store.Queries) error {
		e, err := requireKiegeszitoOn(ctx, q, esemenyID, felhasznaloID, domain.Koltsegek)
		if err != nil {
			return err
		}
		if e.SzervezoID != felhasznaloID {
			return ErrNotSzervezo
		}
		s, err := readElszamolas(ctx, q, e)
		if err != nil {
			return err
		}
		if err := s.requireOpen(); err != nil {
			return err
		}
		lezarta := felhasznaloID
		if len(s.hianyzok()) == 0 {
			lezarta = "" // nobody was missing after all
		}
		job, err = a.calculateElszamolas(ctx, q, s, lezarta)
		return err
	})
	if err != nil {
		return err
	}
	a.kiegeszitoChanged(esemenyID, job)
	return nil
}

// ReopenElszamolas is the Szervezo undoing a calculated Elszamolas: its
// Utalasok are deleted, the Kiadasok unlock and every confirmation is
// cleared. Only the Szervezo may (ErrNotSzervezo), only once it is
// calculated (ErrElszamolasNotKiszamolva) and only while no Utalas is
// megvolt (ErrElszamolasMegvolt).
func (a *App) ReopenElszamolas(ctx context.Context, felhasznaloID, esemenyID string) error {
	err := a.store.InTx(ctx, func(q store.Queries) error {
		e, err := requireKiegeszitoOn(ctx, q, esemenyID, felhasznaloID, domain.Koltsegek)
		if err != nil {
			return err
		}
		if e.SzervezoID != felhasznaloID {
			return ErrNotSzervezo
		}
		s, err := readElszamolas(ctx, q, e)
		if err != nil {
			return err
		}
		if s.kiszamolva == nil {
			return ErrElszamolasNotKiszamolva
		}
		if s.anyMegvolt() {
			return ErrElszamolasMegvolt
		}
		if err := q.DeleteElszamolas(ctx, e.ID); err != nil {
			return err
		}
		return q.DeleteElszamolasMegerositesek(ctx, e.ID)
	})
	if err != nil {
		return err
	}
	a.kiegeszitoChanged(esemenyID)
	return nil
}

// SetUtalasMegvolt marks an Utalas of a calculated Elszamolas megvolt, or
// takes that back. Either of its parties or the Szervezo may
// (ErrNotPermitted for anyone else); an unknown Utalas, or one of another
// Esemeny, is ErrNotFound. Once every Utalas is megvolt the Esemeny is
// Elszamolva and nothing can be taken back (ErrElszamolva). Marking it the
// way it already is changes nothing.
func (a *App) SetUtalasMegvolt(ctx context.Context, felhasznaloID, esemenyID, utalasID string, megvolt bool) error {
	err := a.store.InTx(ctx, func(q store.Queries) error {
		e, err := requireKiegeszitoOn(ctx, q, esemenyID, felhasznaloID, domain.Koltsegek)
		if err != nil {
			return err
		}
		u, err := q.FindUtalas(ctx, utalasID)
		if err != nil {
			return notFound(err)
		}
		if u.EsemenyID != e.ID {
			return ErrNotFound
		}
		if !mayMarkUtalas(e, u, felhasznaloID) {
			return ErrNotPermitted
		}
		s, err := readElszamolas(ctx, q, e)
		if err != nil {
			return err
		}
		if s.elszamolva() {
			if megvolt {
				return nil
			}
			return ErrElszamolva
		}
		if (u.MegvoltAt != nil) == megvolt {
			return nil
		}
		var at *time.Time
		if megvolt {
			now := a.clock.Now()
			at = &now
		}
		return q.SetUtalasMegvolt(ctx, u.ID, at, felhasznaloID)
	})
	if err != nil {
		return err
	}
	a.kiegeszitoChanged(esemenyID)
	return nil
}

// calculateElszamolas calculates and stores the Elszamolas in the caller's
// transaction, which has checked s.requireOpen. lezarta is the Szervezo who
// closed it without every confirmation, "" otherwise. It returns the job
// that sends everyone their result DM after the commit.
func (a *App) calculateElszamolas(ctx context.Context, q store.Queries, s elszamolasAllas, lezarta string) (kiegeszitoJob, error) {
	egyenlegek, err := egyenlegekOf(s.kiadasok)
	if err != nil {
		return nil, err
	}
	utalasok, err := utalasokOf(egyenlegek)
	if err != nil {
		return nil, err
	}
	if err := q.InsertElszamolas(ctx, domain.Elszamolas{EsemenyID: s.e.ID, KiszamolvaAt: a.clock.Now(), LezartaID: lezarta}); err != nil {
		return nil, err
	}
	for n := range utalasok {
		utalasok[n].ID = randomID()
		utalasok[n].EsemenyID = s.e.ID
		if err := q.InsertUtalas(ctx, utalasok[n], n); err != nil {
			return nil, err
		}
	}
	csoport, err := q.FindCsoport(ctx, s.e.CsoportID)
	if err != nil {
		return nil, err
	}

	// Everyone with a balance and every Resztvevo, each once.
	var cimzettek []string
	dms := map[string]*gateway.ElszamolasDM{}
	add := func(id string) {
		if id == "" || dms[id] != nil {
			return
		}
		cimzettek = append(cimzettek, id)
		dms[id] = &gateway.ElszamolasDM{
			EsemenyID:   s.e.ID,
			CsoportName: csoport.Name,
			Title:       s.e.Title,
			Fizetendo:   []gateway.ElszamolasUtalas{},
			Kapando:     []gateway.ElszamolasUtalas{},
			Link:        a.esemenyURL(s.e.ID),
		}
	}
	for _, b := range egyenlegek {
		add(b.FelhasznaloID)
	}
	for _, r := range s.resztvevok {
		add(r.FelhasznaloID) // a name-only Vendeg has none: their Tag has a balance
	}
	for _, u := range utalasok {
		dms[u.FromID].Fizetendo = append(dms[u.FromID].Fizetendo, gateway.ElszamolasUtalas{UtalasID: u.ID, Name: u.ToName, Amount: u.Amount})
		dms[u.ToID].Kapando = append(dms[u.ToID].Kapando, gateway.ElszamolasUtalas{UtalasID: u.ID, Name: u.FromName, Amount: u.Amount})
	}
	csoportID := s.e.CsoportID
	return func(ctx context.Context, a *App) {
		for _, id := range cimzettek {
			a.sendElszamolasDM(ctx, csoportID, id, *dms[id])
		}
	}, nil
}

// utalasokOf turns balances into Utalasok between Felhasznalok with the
// standard greedy approach: repeatedly the largest debtor pays the largest
// creditor as much as settles one of them (ties by name, then id). Each
// step settles at least one person and the last settles two, so n people
// with a non-zero balance need at most n-1 Utalasok. That is the usual
// bound, not a guaranteed global minimum (finding the true minimum is
// NP-hard).
// The balances must add up to 0.
func utalasokOf(egyenlegek []Egyenleg) ([]domain.Utalas, error) {
	type fel struct {
		id, name string
		amount   int
	}
	var debtors, creditors []*fel
	sum := 0
	for _, b := range egyenlegek {
		sum += b.Netto
		switch {
		case b.Netto < 0:
			debtors = append(debtors, &fel{id: b.FelhasznaloID, name: b.Name, amount: -b.Netto})
		case b.Netto > 0:
			creditors = append(creditors, &fel{id: b.FelhasznaloID, name: b.Name, amount: b.Netto})
		}
	}
	if sum != 0 {
		return nil, fmt.Errorf("the balances add up to %d, not 0", sum)
	}
	largestFirst := func(list []*fel) {
		sort.SliceStable(list, func(i, j int) bool {
			if list[i].amount != list[j].amount {
				return list[i].amount > list[j].amount
			}
			if list[i].name != list[j].name {
				return list[i].name < list[j].name
			}
			return list[i].id < list[j].id
		})
	}
	out := []domain.Utalas{}
	for len(debtors) > 0 && len(creditors) > 0 {
		largestFirst(debtors)
		largestFirst(creditors)
		d, c := debtors[0], creditors[0]
		x := min(d.amount, c.amount)
		out = append(out, domain.Utalas{FromID: d.id, FromName: d.name, ToID: c.id, ToName: c.name, Amount: x})
		d.amount -= x
		c.amount -= x
		if d.amount == 0 {
			debtors = debtors[1:]
		}
		if c.amount == 0 {
			creditors = creditors[1:]
		}
	}
	return out, nil
}

// elszamolasEmlekeztetoPayload identifies the reminder job: one per
// Esemeny.
type elszamolasEmlekeztetoPayload struct{}

// scheduleElszamolasEmlekezteto schedules the reminder to confirm, 3 days
// after the Esemeny ended (endedAt), in the transaction that makes it
// Lezajlott. Whether there is anything to remind of is decided when it
// runs: Koltsegek may still be switched on, and Kiadasok recorded.
func (a *App) scheduleElszamolasEmlekezteto(ctx context.Context, q store.Queries, esemenyID string, endedAt time.Time) error {
	return a.scheduleJob(ctx, q, jobElszamolasEmlekezteto, esemenyID, endedAt.Add(elszamolasEmlekeztetoDelay), elszamolasEmlekeztetoPayload{})
}

// runElszamolasEmlekezteto DMs every Resztvevo who hasn't confirmed yet
// (the Tag of a name-only Vendeg who hasn't), once, if the Esemeny has an open Elszamolas: Lezajlott, Koltsegek on,
// a Kiadas, not calculated.
func (a *App) runElszamolasEmlekezteto(ctx context.Context, q store.Queries, j store.Job) (func(context.Context), error) {
	e, err := q.FindEsemeny(ctx, j.EsemenyID)
	if err != nil {
		return nil, err
	}
	if e.Allapot != domain.Lezajlott {
		return nil, nil
	}
	on, err := q.EnabledKiegeszitok(ctx, e.ID)
	if err != nil || !on[domain.Koltsegek] {
		return nil, err
	}
	s, err := readElszamolas(ctx, q, e)
	if err != nil {
		return nil, err
	}
	if !s.open() {
		return nil, nil
	}
	var cimzettek []string
	seen := map[string]bool{}
	for _, r := range s.hianyzok() {
		// A name-only Vendeg's Tag confirms for them: remind the Tag.
		f := r.FelhasznaloID
		if r.VendegID != "" {
			f = r.AddedBy
		}
		if f != "" && !seen[f] {
			seen[f] = true
			cimzettek = append(cimzettek, f)
		}
	}
	if len(cimzettek) == 0 {
		return nil, nil
	}
	csoport, err := q.FindCsoport(ctx, e.CsoportID)
	if err != nil {
		return nil, err
	}
	msg := gateway.ElszamolasEmlekezteto{CsoportName: csoport.Name, Title: e.Title, Link: a.esemenyURL(e.ID)}
	return func(ctx context.Context) {
		for _, f := range cimzettek {
			a.sendElszamolasEmlekeztetoDM(ctx, e.CsoportID, f, msg)
		}
	}, nil
}

// elszamolasSummary is the Elszamolas's part of Koltsegek's summary
// fragment: "Elszámolva ✓" once settled, how many Utalasok are megvolt
// while calculated, "" otherwise.
func elszamolasSummary(ctx context.Context, q store.Queries, e domain.Esemeny) (string, error) {
	if _, err := q.FindElszamolas(ctx, e.ID); errors.Is(err, store.ErrNotFound) {
		return "", nil
	} else if err != nil {
		return "", err
	}
	utalasok, err := q.ListUtalasok(ctx, e.ID)
	if err != nil {
		return "", err
	}
	megvolt := 0
	for _, u := range utalasok {
		if u.MegvoltAt != nil {
			megvolt++
		}
	}
	if megvolt == len(utalasok) {
		return "Elszámolva ✓", nil
	}
	return fmt.Sprintf("💸 %d/%d utalás megvolt", megvolt, len(utalasok)), nil
}
