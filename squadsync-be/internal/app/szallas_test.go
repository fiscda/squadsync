package app_test

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"

	"squadsync-be/internal/app"
	"squadsync-be/internal/apptest"
	"squadsync-be/internal/domain"
	"squadsync-be/internal/gateway"
	"squadsync-be/internal/gateway/gatewayfake"
)

// szallasTrip has Anna (the Szervezo) propose an Esemeny with two Idopontok
// in a Csoport with Bela, Cecil, Dora and Emil, switch Szallas on, and
// confirm the first Idopont with Anna, Bela, Cecil and Dora as Resztvevok
// (Emil says nem).
func szallasTrip(t *testing.T) (*apptest.Harness, app.EsemenyDetail) {
	t.Helper()
	h := apptest.New(t)
	c := csoportOf(t, h, "bela", "cecil", "dora", "emil")
	e := propose(t, h, "anna", c.ID, 2)
	switchKiegeszito(t, h, "anna", e.ID, domain.Szallas, true)
	d := detail(t, h, "anna", e.ID)
	for _, who := range []string{"anna", "bela", "cecil", "dora"} {
		answer(t, h, who, d, 0, domain.Igen)
	}
	answer(t, h, "emil", d, 0, domain.Nem)
	confirm(t, h, "anna", d, 0)
	return h, d
}

func szallasOf(t *testing.T, h *apptest.Harness, felhasznaloID, esemenyID string) app.Szallas {
	t.Helper()
	z, err := h.App.Szallas(context.Background(), felhasznaloID, esemenyID)
	if err != nil {
		t.Fatalf("Szallas(%s): %v", felhasznaloID, err)
	}
	return z
}

func addSzoba(t *testing.T, h *apptest.Harness, esemenyID, name string) domain.Szoba {
	t.Helper()
	s, err := h.App.AddSzoba(context.Background(), "anna", esemenyID, name)
	if err != nil {
		t.Fatalf("AddSzoba(%s): %v", name, err)
	}
	return s
}

func addAgy(t *testing.T, h *apptest.Harness, esemenyID, szobaID string, capacity int) domain.Agy {
	t.Helper()
	a, err := h.App.AddAgy(context.Background(), "anna", esemenyID, szobaID, app.AgyInput{Capacity: capacity})
	if err != nil {
		t.Fatalf("AddAgy(%d): %v", capacity, err)
	}
	return a
}

func claimAgy(t *testing.T, h *apptest.Harness, felhasznaloID, esemenyID, agyID string) {
	t.Helper()
	if err := h.App.ClaimAgy(context.Background(), felhasznaloID, esemenyID, agyID, app.FoglalasInput{}); err != nil {
		t.Fatalf("ClaimAgy(%s): %v", felhasznaloID, err)
	}
}

// szallasLayout shows each Szoba as "name[alvo,alvo|alvo]", one group per
// Agy, in order.
func szallasLayout(z app.Szallas) []string {
	out := []string{}
	for _, szoba := range z.Szobak {
		var agyak []string
		for _, agy := range szoba.Agyak {
			var alvok []string
			for _, g := range agy.Alvok {
				alvok = append(alvok, g.Holder.FelhasznaloID)
			}
			agyak = append(agyak, strings.Join(alvok, ","))
		}
		out = append(out, szoba.Name+"["+strings.Join(agyak, "|")+"]")
	}
	return out
}

func expectSzallasLayout(t *testing.T, z app.Szallas, want ...string) {
	t.Helper()
	if got := szallasLayout(z); fmt.Sprint(got) != fmt.Sprint(want) {
		t.Fatalf("szallas = %v, want %v", got, want)
	}
}

func agyNelkulIDs(z app.Szallas) []string {
	out := []string{}
	for _, r := range z.AgyNelkul {
		out = append(out, r.FelhasznaloID)
	}
	return out
}

func szallasDMs(h *apptest.Harness) []gatewayfake.DM {
	var out []gatewayfake.DM
	for _, dm := range h.Discord.DMs() {
		if dm.Szallas != nil {
			out = append(out, dm)
		}
	}
	return out
}

func TestOnlyTheSzervezoSetsUpTheSzallasInEgyeztetesAndVeglegesitve(t *testing.T) {
	h := apptest.New(t)
	ctx := context.Background()
	c := csoportOf(t, h, "bela")
	e := propose(t, h, "anna", c.ID, 1)
	switchKiegeszito(t, h, "anna", e.ID, domain.Szallas, true)

	// Egyeztetes: set up, in order.
	szoba, err := h.App.AddSzoba(ctx, "anna", e.ID, "  Emeleti szoba ")
	if err != nil || szoba.Name != "Emeleti szoba" {
		t.Fatalf("AddSzoba = %+v, %v", szoba, err)
	}
	addSzoba(t, h, e.ID, "Padlás")
	agy, err := h.App.AddAgy(ctx, "anna", e.ID, szoba.ID, app.AgyInput{Capacity: 2, Label: " franciaágy "})
	if err != nil || agy.Capacity != 2 || agy.Label != "franciaágy" {
		t.Fatalf("AddAgy = %+v, %v", agy, err)
	}
	if err := h.App.SetSzallas(ctx, "anna", e.ID, app.SzallasInput{Note: " Napfény Vendégház "}); err != nil {
		t.Fatalf("SetSzallas: %v", err)
	}
	z := szallasOf(t, h, "bela", e.ID)
	expectSzallasLayout(t, z, "Emeleti szoba[]", "Padlás[]")
	if z.Note != "Napfény Vendégház" || z.CanSetup || z.CanClaim || z.IsSzervezo {
		t.Fatalf("Bela sees %+v", z)
	}
	if z := szallasOf(t, h, "anna", e.ID); !z.CanSetup || !z.IsSzervezo || z.CanClaim {
		t.Fatalf("Anna sees %+v", z)
	}

	// Nobody else sets up.
	if _, err := h.App.AddSzoba(ctx, "bela", e.ID, "Pince"); !errors.Is(err, app.ErrNotPermitted) {
		t.Fatalf("a Tag adding a Szoba: err = %v, want ErrNotPermitted", err)
	}
	if err := h.App.SetSzallas(ctx, "bela", e.ID, app.SzallasInput{Note: "x"}); !errors.Is(err, app.ErrNotPermitted) {
		t.Fatalf("a Tag setting the note: err = %v, want ErrNotPermitted", err)
	}
	if _, err := h.App.AddAgy(ctx, "bela", e.ID, szoba.ID, app.AgyInput{Capacity: 1}); !errors.Is(err, app.ErrNotPermitted) {
		t.Fatalf("a Tag adding an Agy: err = %v, want ErrNotPermitted", err)
	}
	if _, err := h.App.AddSzoba(ctx, "dani", e.ID, "Pince"); !errors.Is(err, app.ErrNotFound) {
		t.Fatalf("a non-Tag: err = %v, want ErrNotFound", err)
	}

	// Rename, edit, bad input.
	if err := h.App.RenameSzoba(ctx, "anna", e.ID, szoba.ID, "Alsó szoba"); err != nil {
		t.Fatalf("RenameSzoba: %v", err)
	}
	if err := h.App.UpdateAgy(ctx, "anna", e.ID, agy.ID, app.AgyInput{Capacity: 1, Label: "egyszemélyes"}); err != nil {
		t.Fatalf("UpdateAgy: %v", err)
	}
	if z := szallasOf(t, h, "bela", e.ID); z.Szobak[0].Name != "Alsó szoba" || z.Szobak[0].Agyak[0].Capacity != 1 || z.Szobak[0].Agyak[0].Label != "egyszemélyes" {
		t.Fatalf("szallas = %+v", z.Szobak[0])
	}
	for _, name := range []string{"", "   ", strings.Repeat("x", 61)} {
		if _, err := h.App.AddSzoba(ctx, "anna", e.ID, name); !errors.Is(err, app.ErrInvalidInput) {
			t.Fatalf("AddSzoba(%q): err = %v, want ErrInvalidInput", name, err)
		}
	}
	for _, bad := range []app.AgyInput{{Capacity: 0}, {Capacity: 3}, {Capacity: 1, Label: strings.Repeat("x", 41)}} {
		if _, err := h.App.AddAgy(ctx, "anna", e.ID, szoba.ID, bad); !errors.Is(err, app.ErrInvalidInput) {
			t.Fatalf("AddAgy(%+v): err = %v, want ErrInvalidInput", bad, err)
		}
	}
	if err := h.App.SetSzallas(ctx, "anna", e.ID, app.SzallasInput{Note: strings.Repeat("x", 201)}); !errors.Is(err, app.ErrInvalidInput) {
		t.Fatalf("a long note: err = %v, want ErrInvalidInput", err)
	}
	if err := h.App.RenameSzoba(ctx, "anna", e.ID, "no-such-szoba", "x"); !errors.Is(err, app.ErrNotFound) {
		t.Fatalf("an unknown Szoba: err = %v, want ErrNotFound", err)
	}

	// Veglegesitve still allows setup; Lezajlott and Elmaradt don't.
	d := detail(t, h, "anna", e.ID)
	answer(t, h, "anna", d, 0, domain.Igen)
	confirm(t, h, "anna", d, 0)
	addSzoba(t, h, e.ID, "Pince")
	if err := h.App.CancelEsemeny(ctx, "anna", e.ID); err != nil {
		t.Fatalf("CancelEsemeny: %v", err)
	}
	if _, err := h.App.AddSzoba(ctx, "anna", e.ID, "Garázs"); !errors.Is(err, app.ErrSetupClosed) {
		t.Fatalf("setup once Elmaradt: err = %v, want ErrSetupClosed", err)
	}
	if z := szallasOf(t, h, "anna", e.ID); z.CanSetup {
		t.Fatalf("CanSetup once Elmaradt")
	}
}

func TestSzallasIsHiddenWhileItIsSwitchedOff(t *testing.T) {
	h, d := szallasTrip(t)
	ctx := context.Background()
	szoba := addSzoba(t, h, d.ID, "Szoba")
	agy := addAgy(t, h, d.ID, szoba.ID, 1)
	claimAgy(t, h, "bela", d.ID, agy.ID)
	switchKiegeszito(t, h, "anna", d.ID, domain.Szallas, false)

	if _, err := h.App.Szallas(ctx, "bela", d.ID); !errors.Is(err, app.ErrKiegeszitoOff) {
		t.Fatalf("Szallas while off: err = %v, want ErrKiegeszitoOff", err)
	}
	if _, err := h.App.AddSzoba(ctx, "anna", d.ID, "Másik"); !errors.Is(err, app.ErrKiegeszitoOff) {
		t.Fatalf("AddSzoba while off: err = %v, want ErrKiegeszitoOff", err)
	}
	// Nothing is lost.
	switchKiegeszito(t, h, "anna", d.ID, domain.Szallas, true)
	expectSzallasLayout(t, szallasOf(t, h, "anna", d.ID), "Szoba[bela]")
}

func TestClaimingASpotNeedsAResztvevoInVeglegesitve(t *testing.T) {
	h := apptest.New(t)
	ctx := context.Background()
	c := csoportOf(t, h, "bela", "cecil")
	e := propose(t, h, "anna", c.ID, 1)
	switchKiegeszito(t, h, "anna", e.ID, domain.Szallas, true)
	szoba := addSzoba(t, h, e.ID, "Szoba")
	agy := addAgy(t, h, e.ID, szoba.ID, 2)
	d := detail(t, h, "anna", e.ID)

	if err := h.App.ClaimAgy(ctx, "bela", e.ID, agy.ID, app.FoglalasInput{}); !errors.Is(err, app.ErrClaimingClosed) {
		t.Fatalf("claiming in Egyeztetes: err = %v, want ErrClaimingClosed", err)
	}
	// The state and the Resztvevo checks come before the lookup.
	if err := h.App.ClaimAgy(ctx, "bela", e.ID, "no-such-agy", app.FoglalasInput{}); !errors.Is(err, app.ErrClaimingClosed) {
		t.Fatalf("an unknown Agy in Egyeztetes: err = %v, want ErrClaimingClosed", err)
	}
	answer(t, h, "anna", d, 0, domain.Igen)
	answer(t, h, "bela", d, 0, domain.Igen)
	answer(t, h, "cecil", d, 0, domain.Nem)
	confirm(t, h, "anna", d, 0)

	if err := h.App.ClaimAgy(ctx, "cecil", e.ID, "no-such-agy", app.FoglalasInput{}); !errors.Is(err, app.ErrNotResztvevo) {
		t.Fatalf("a non-Resztvevo and an unknown Agy: err = %v, want ErrNotResztvevo", err)
	}
	if err := h.App.ClaimAgy(ctx, "cecil", e.ID, agy.ID, app.FoglalasInput{}); !errors.Is(err, app.ErrNotResztvevo) {
		t.Fatalf("a non-Resztvevo: err = %v, want ErrNotResztvevo", err)
	}
	if err := h.App.ClaimAgy(ctx, "dani", e.ID, agy.ID, app.FoglalasInput{}); !errors.Is(err, app.ErrNotFound) {
		t.Fatalf("a non-Tag: err = %v, want ErrNotFound", err)
	}
	if err := h.App.ClaimAgy(ctx, "bela", e.ID, "no-such-agy", app.FoglalasInput{}); !errors.Is(err, app.ErrNotFound) {
		t.Fatalf("an unknown Agy: err = %v, want ErrNotFound", err)
	}
	// A name-only Vendeg isn't there before #13.
	if err := h.App.ClaimAgy(ctx, "bela", e.ID, agy.ID, app.FoglalasInput{VendegID: "v1"}); !errors.Is(err, app.ErrNotFound) {
		t.Fatalf("a Vendeg: err = %v, want ErrNotFound", err)
	}
	// Nobody claims for someone else.
	if err := h.App.ClaimAgy(ctx, "bela", e.ID, agy.ID, app.FoglalasInput{FelhasznaloID: "anna"}); !errors.Is(err, app.ErrNotPermitted) {
		t.Fatalf("claiming for someone else: err = %v, want ErrNotPermitted", err)
	}
	claimAgy(t, h, "bela", e.ID, agy.ID)
	if z := szallasOf(t, h, "bela", e.ID); z.SajatAgyID != agy.ID || !z.CanClaim {
		t.Fatalf("Bela sees %+v", z)
	}

	// Claims are frozen while Egyeztetes is open again, setup is not.
	reopen(t, h, "anna", e.ID)
	if err := h.App.ReleaseFoglalas(ctx, "bela", e.ID, app.FoglalasInput{}); !errors.Is(err, app.ErrClaimingClosed) {
		t.Fatalf("leaving while frozen: err = %v, want ErrClaimingClosed", err)
	}
	if err := h.App.SetMasholAlszik(ctx, "anna", e.ID, app.MasholAlszikInput{On: true}); !errors.Is(err, app.ErrClaimingClosed) {
		t.Fatalf("marking while frozen: err = %v, want ErrClaimingClosed", err)
	}
	addAgy(t, h, e.ID, szoba.ID, 1)
	expectSzallasLayout(t, szallasOf(t, h, "anna", e.ID), "Szoba[bela|]")
}

func TestAnAgyCantExceedItsFemohely(t *testing.T) {
	h, d := szallasTrip(t)
	ctx := context.Background()
	szoba := addSzoba(t, h, d.ID, "Szoba")
	single := addAgy(t, h, d.ID, szoba.ID, 1)
	double := addAgy(t, h, d.ID, szoba.ID, 2)

	claimAgy(t, h, "bela", d.ID, single.ID)
	if err := h.App.ClaimAgy(ctx, "cecil", d.ID, single.ID, app.FoglalasInput{}); !errors.Is(err, app.ErrAgyFull) {
		t.Fatalf("a full single: err = %v, want ErrAgyFull", err)
	}
	// Two people share a double, a third doesn't fit.
	claimAgy(t, h, "cecil", d.ID, double.ID)
	claimAgy(t, h, "dora", d.ID, double.ID)
	if err := h.App.ClaimAgy(ctx, "anna", d.ID, double.ID, app.FoglalasInput{}); !errors.Is(err, app.ErrAgyFull) {
		t.Fatalf("a full double: err = %v, want ErrAgyFull", err)
	}
	// Claiming the spot you hold changes nothing.
	claimAgy(t, h, "dora", d.ID, double.ID)
	z := szallasOf(t, h, "anna", d.ID)
	expectSzallasLayout(t, z, "Szoba[bela|cecil,dora]")
	if z.Szobak[0].Agyak[0].Szabad != 0 || z.Szobak[0].Agyak[1].Szabad != 0 {
		t.Fatalf("szabad = %+v", z.Szobak[0].Agyak)
	}
	if got := agyNelkulIDs(z); fmt.Sprint(got) != "[anna]" {
		t.Fatalf("without a bed = %v", got)
	}
}

func TestTwoPeopleRacingForTheLastSpotOfADoubleOnlyOneGetsIt(t *testing.T) {
	h, d := szallasTrip(t)
	ctx := context.Background()
	szoba := addSzoba(t, h, d.ID, "Szoba")
	double := addAgy(t, h, d.ID, szoba.ID, 2)
	claimAgy(t, h, "anna", d.ID, double.ID)

	var wg sync.WaitGroup
	errs := make([]error, 3)
	for n, who := range []string{"bela", "cecil", "dora"} {
		wg.Add(1)
		go func() {
			defer wg.Done()
			errs[n] = h.App.ClaimAgy(ctx, who, d.ID, double.ID, app.FoglalasInput{})
		}()
	}
	wg.Wait()

	won := 0
	for _, err := range errs {
		switch {
		case err == nil:
			won++
		case !errors.Is(err, app.ErrAgyFull):
			t.Fatalf("unexpected error: %v", err)
		}
	}
	if won != 1 {
		t.Fatalf("%d claims won the last spot, want 1 (%v)", won, errs)
	}
	if z := szallasOf(t, h, "anna", d.ID); len(z.Szobak[0].Agyak[0].Alvok) != 2 {
		t.Fatalf("alvok = %+v, want two", z.Szobak[0].Agyak[0].Alvok)
	}
}

func TestOneSpotPerPersonSoMovingMeansLeavingFirst(t *testing.T) {
	h, d := szallasTrip(t)
	ctx := context.Background()
	szoba := addSzoba(t, h, d.ID, "Szoba")
	first := addAgy(t, h, d.ID, szoba.ID, 1)
	second := addAgy(t, h, d.ID, szoba.ID, 1)

	claimAgy(t, h, "cecil", d.ID, first.ID)
	if err := h.App.ClaimAgy(ctx, "cecil", d.ID, second.ID, app.FoglalasInput{}); !errors.Is(err, app.ErrAlreadyInAgy) {
		t.Fatalf("a second spot: err = %v, want ErrAlreadyInAgy", err)
	}
	if z := szallasOf(t, h, "cecil", d.ID); z.SajatAgyID != first.ID {
		t.Fatalf("Cecil's Agy = %q, want the first", z.SajatAgyID)
	}
	if err := h.App.ReleaseFoglalas(ctx, "cecil", d.ID, app.FoglalasInput{}); err != nil {
		t.Fatalf("ReleaseFoglalas: %v", err)
	}
	claimAgy(t, h, "cecil", d.ID, second.ID)
	expectSzallasLayout(t, szallasOf(t, h, "anna", d.ID), "Szoba[|cecil]")

	// Leaving twice is fine.
	for range 2 {
		if err := h.App.ReleaseFoglalas(ctx, "cecil", d.ID, app.FoglalasInput{}); err != nil {
			t.Fatalf("ReleaseFoglalas: %v", err)
		}
	}
	expectSzallasLayout(t, szallasOf(t, h, "anna", d.ID), "Szoba[|]")
	// Nobody else leaves for you.
	claimAgy(t, h, "cecil", d.ID, first.ID)
	if err := h.App.ReleaseFoglalas(ctx, "bela", d.ID, app.FoglalasInput{FelhasznaloID: "cecil"}); !errors.Is(err, app.ErrNotPermitted) {
		t.Fatalf("releasing someone else's: err = %v, want ErrNotPermitted", err)
	}
}

func TestTheSzervezoMovesAndRemovesPeople(t *testing.T) {
	h, d := szallasTrip(t)
	ctx := context.Background()
	szoba := addSzoba(t, h, d.ID, "Szoba")
	first := addAgy(t, h, d.ID, szoba.ID, 1)
	second := addAgy(t, h, d.ID, szoba.ID, 2)
	claimAgy(t, h, "bela", d.ID, first.ID)
	claimAgy(t, h, "cecil", d.ID, second.ID)

	// Only the Szervezo moves.
	if err := h.App.MoveAlvo(ctx, "bela", d.ID, second.ID, app.FoglalasInput{FelhasznaloID: "bela"}); !errors.Is(err, app.ErrNotPermitted) {
		t.Fatalf("a sleeper moving: err = %v, want ErrNotPermitted", err)
	}
	// A sleeper, and someone without a bed, are moved onto an Agy with room.
	if err := h.App.MoveAlvo(ctx, "anna", d.ID, second.ID, app.FoglalasInput{FelhasznaloID: "bela"}); err != nil {
		t.Fatalf("moving a sleeper: %v", err)
	}
	expectSzallasLayout(t, szallasOf(t, h, "anna", d.ID), "Szoba[|bela,cecil]")
	if err := h.App.MoveAlvo(ctx, "anna", d.ID, first.ID, app.FoglalasInput{FelhasznaloID: "dora"}); err != nil {
		t.Fatalf("seating someone without a bed: %v", err)
	}
	expectSzallasLayout(t, szallasOf(t, h, "anna", d.ID), "Szoba[dora|bela,cecil]")

	// Onto a full Agy is refused, and nobody loses what they had.
	if err := h.App.MoveAlvo(ctx, "anna", d.ID, second.ID, app.FoglalasInput{FelhasznaloID: "dora"}); !errors.Is(err, app.ErrAgyFull) {
		t.Fatalf("moving onto a full Agy: err = %v, want ErrAgyFull", err)
	}
	if err := h.App.MoveAlvo(ctx, "anna", d.ID, first.ID, app.FoglalasInput{FelhasznaloID: "cecil"}); !errors.Is(err, app.ErrAgyFull) {
		t.Fatalf("moving onto a full Agy: err = %v, want ErrAgyFull", err)
	}
	expectSzallasLayout(t, szallasOf(t, h, "anna", d.ID), "Szoba[dora|bela,cecil]")
	// Moving onto the Agy someone is in changes nothing.
	if err := h.App.MoveAlvo(ctx, "anna", d.ID, first.ID, app.FoglalasInput{FelhasznaloID: "dora"}); err != nil {
		t.Fatalf("moving in place: %v", err)
	}

	// Removing, by the Szervezo only.
	if err := h.App.ReleaseFoglalas(ctx, "anna", d.ID, app.FoglalasInput{FelhasznaloID: "cecil"}); err != nil {
		t.Fatalf("removing: %v", err)
	}
	expectSzallasLayout(t, szallasOf(t, h, "anna", d.ID), "Szoba[dora|bela]")

	// Emil said nem and Dani isn't a Tag.
	for _, who := range []string{"emil", "dani"} {
		err := h.App.MoveAlvo(ctx, "anna", d.ID, second.ID, app.FoglalasInput{FelhasznaloID: who})
		if !errors.Is(err, app.ErrNotResztvevo) {
			t.Fatalf("seating %s: err = %v, want ErrNotResztvevo", who, err)
		}
	}
}

func TestMasholAlszikClearsTheSpotAndTakingASpotClearsTheMark(t *testing.T) {
	h, d := szallasTrip(t)
	ctx := context.Background()
	szoba := addSzoba(t, h, d.ID, "Szoba")
	agy := addAgy(t, h, d.ID, szoba.ID, 1)
	claimAgy(t, h, "bela", d.ID, agy.ID)

	if err := h.App.SetMasholAlszik(ctx, "bela", d.ID, app.MasholAlszikInput{On: true}); err != nil {
		t.Fatalf("SetMasholAlszik: %v", err)
	}
	z := szallasOf(t, h, "bela", d.ID)
	expectSzallasLayout(t, z, "Szoba[]")
	if !z.SajatMasholAlszik || z.SajatAgyID != "" || len(z.MasholAlszik) != 1 || z.MasholAlszik[0].Holder.FelhasznaloID != "bela" {
		t.Fatalf("Bela sees %+v", z)
	}
	// The spot is free again, and Bela doesn't count as missing a bed.
	claimAgy(t, h, "cecil", d.ID, agy.ID)
	if got := agyNelkulIDs(szallasOf(t, h, "anna", d.ID)); fmt.Sprint(got) != "[anna dora]" {
		t.Fatalf("without a bed = %v", got)
	}

	// Taking a spot clears the mark; so does the Szervezo's move.
	if err := h.App.ReleaseFoglalas(ctx, "cecil", d.ID, app.FoglalasInput{}); err != nil {
		t.Fatalf("ReleaseFoglalas: %v", err)
	}
	claimAgy(t, h, "bela", d.ID, agy.ID)
	if z := szallasOf(t, h, "bela", d.ID); z.SajatMasholAlszik || len(z.MasholAlszik) != 0 || z.SajatAgyID != agy.ID {
		t.Fatalf("Bela after taking a spot: %+v", z)
	}
	if err := h.App.SetMasholAlszik(ctx, "dora", d.ID, app.MasholAlszikInput{On: true}); err != nil {
		t.Fatalf("SetMasholAlszik: %v", err)
	}
	double := addAgy(t, h, d.ID, szoba.ID, 2)
	if err := h.App.MoveAlvo(ctx, "anna", d.ID, double.ID, app.FoglalasInput{FelhasznaloID: "dora"}); err != nil {
		t.Fatalf("MoveAlvo: %v", err)
	}
	if z := szallasOf(t, h, "dora", d.ID); z.SajatMasholAlszik || len(z.MasholAlszik) != 0 {
		t.Fatalf("Dora after being moved: %+v", z)
	}

	// Unmarking, and only Resztvevok may mark.
	if err := h.App.SetMasholAlszik(ctx, "bela", d.ID, app.MasholAlszikInput{On: true}); err != nil {
		t.Fatalf("SetMasholAlszik: %v", err)
	}
	if err := h.App.SetMasholAlszik(ctx, "bela", d.ID, app.MasholAlszikInput{On: false}); err != nil {
		t.Fatalf("unmarking: %v", err)
	}
	if got := agyNelkulIDs(szallasOf(t, h, "anna", d.ID)); fmt.Sprint(got) != "[anna bela cecil]" {
		t.Fatalf("without a bed = %v", got)
	}
	if err := h.App.SetMasholAlszik(ctx, "emil", d.ID, app.MasholAlszikInput{On: true}); !errors.Is(err, app.ErrNotResztvevo) {
		t.Fatalf("a non-Resztvevo: err = %v, want ErrNotResztvevo", err)
	}
}

func TestTheCountWithoutABedExcludesSleepersAndMasholAlszik(t *testing.T) {
	h, d := szallasTrip(t)
	ctx := context.Background()
	if got := agyNelkulIDs(szallasOf(t, h, "anna", d.ID)); fmt.Sprint(got) != "[anna bela cecil dora]" {
		t.Fatalf("without a bed = %v, want every Resztvevo (not Emil)", got)
	}
	szoba := addSzoba(t, h, d.ID, "Szoba")
	agy := addAgy(t, h, d.ID, szoba.ID, 2)
	claimAgy(t, h, "anna", d.ID, agy.ID)
	claimAgy(t, h, "bela", d.ID, agy.ID)
	if err := h.App.SetMasholAlszik(ctx, "cecil", d.ID, app.MasholAlszikInput{On: true}); err != nil {
		t.Fatalf("SetMasholAlszik: %v", err)
	}
	if got := agyNelkulIDs(szallasOf(t, h, "bela", d.ID)); fmt.Sprint(got) != "[dora]" {
		t.Fatalf("without a bed = %v, want Dora", got)
	}
}

func TestDeletingASzobaFreesItsSpotsAndSendsEachSleeperADM(t *testing.T) {
	h, d := szallasTrip(t)
	ctx := context.Background()
	felso := addSzoba(t, h, d.ID, "Felső")
	also := addSzoba(t, h, d.ID, "Alsó")
	a1 := addAgy(t, h, d.ID, felso.ID, 2)
	a2 := addAgy(t, h, d.ID, felso.ID, 1)
	a3 := addAgy(t, h, d.ID, also.ID, 1)
	claimAgy(t, h, "bela", d.ID, a1.ID)
	claimAgy(t, h, "cecil", d.ID, a1.ID)
	claimAgy(t, h, "dora", d.ID, a2.ID)
	claimAgy(t, h, "anna", d.ID, a3.ID)

	if err := h.App.DeleteSzoba(ctx, "bela", d.ID, felso.ID); !errors.Is(err, app.ErrNotPermitted) {
		t.Fatalf("a sleeper deleting: err = %v, want ErrNotPermitted", err)
	}
	if err := h.App.DeleteSzoba(ctx, "anna", d.ID, felso.ID); err != nil {
		t.Fatalf("DeleteSzoba: %v", err)
	}

	z := szallasOf(t, h, "anna", d.ID)
	expectSzallasLayout(t, z, "Alsó[anna]")
	if got := agyNelkulIDs(z); fmt.Sprint(got) != "[bela cecil dora]" {
		t.Fatalf("without a bed = %v, want the former sleepers", got)
	}
	want := gateway.SzallasDM{SzobaName: "Felső", Title: "Társasjáték", Link: esemenyURL(d.ID)}
	dms := szallasDMs(h)
	if len(dms) != 3 {
		t.Fatalf("szallas DMs = %+v, want one per sleeper", dms)
	}
	for n, who := range []string{"discord-bela", "discord-cecil", "discord-dora"} {
		if dms[n].DiscordID != who || *dms[n].Szallas != want {
			t.Fatalf("DM %d = %+v to %s, want %+v to %s", n, dms[n].Szallas, dms[n].DiscordID, want, who)
		}
	}
	if err := h.App.DeleteSzoba(ctx, "anna", d.ID, felso.ID); !errors.Is(err, app.ErrNotFound) {
		t.Fatalf("deleting it again: err = %v, want ErrNotFound", err)
	}
	// The Szervezo is told too when they slept there.
	if err := h.App.DeleteSzoba(ctx, "anna", d.ID, also.ID); err != nil {
		t.Fatalf("DeleteSzoba: %v", err)
	}
	if dms := szallasDMs(h); len(dms) != 4 || dms[3].DiscordID != "discord-anna" {
		t.Fatalf("szallas DMs = %+v, want a fourth one to Anna", dms)
	}
}

func TestDeletingASzobaWhileSzallasIsOffSendsNoDMs(t *testing.T) {
	h, d := szallasTrip(t)
	szoba := addSzoba(t, h, d.ID, "Szoba")
	agy := addAgy(t, h, d.ID, szoba.ID, 1)
	claimAgy(t, h, "bela", d.ID, agy.ID)
	switchKiegeszito(t, h, "anna", d.ID, domain.Szallas, false)

	// Deleting needs the Kiegeszito on, so nothing is sent by it; and a
	// Szoba can't vanish while hidden.
	if err := h.App.DeleteSzoba(context.Background(), "anna", d.ID, szoba.ID); !errors.Is(err, app.ErrKiegeszitoOff) {
		t.Fatalf("DeleteSzoba while off: err = %v, want ErrKiegeszitoOff", err)
	}
	if n := len(szallasDMs(h)); n != 0 {
		t.Fatalf("%d szallas DMs, want none while Szallas is off", n)
	}
}

func TestDeletingAnAgyFreesItsSpotsAndEditingKeepsTheFemohelyAtOrAboveTheSleepers(t *testing.T) {
	h, d := szallasTrip(t)
	ctx := context.Background()
	szoba := addSzoba(t, h, d.ID, "Szoba")
	double := addAgy(t, h, d.ID, szoba.ID, 2)
	claimAgy(t, h, "bela", d.ID, double.ID)
	claimAgy(t, h, "cecil", d.ID, double.ID)

	if err := h.App.UpdateAgy(ctx, "anna", d.ID, double.ID, app.AgyInput{Capacity: 1}); !errors.Is(err, app.ErrAgyCapacityBelowAlvok) {
		t.Fatalf("femohely below the sleepers: err = %v, want ErrAgyCapacityBelowAlvok", err)
	}
	if err := h.App.ReleaseFoglalas(ctx, "anna", d.ID, app.FoglalasInput{FelhasznaloID: "cecil"}); err != nil {
		t.Fatalf("ReleaseFoglalas: %v", err)
	}
	if err := h.App.UpdateAgy(ctx, "anna", d.ID, double.ID, app.AgyInput{Capacity: 1, Label: "kis ágy"}); err != nil {
		t.Fatalf("UpdateAgy: %v", err)
	}
	if err := h.App.DeleteAgy(ctx, "bela", d.ID, double.ID); !errors.Is(err, app.ErrNotPermitted) {
		t.Fatalf("a sleeper deleting the Agy: err = %v, want ErrNotPermitted", err)
	}
	if err := h.App.DeleteAgy(ctx, "anna", d.ID, double.ID); err != nil {
		t.Fatalf("DeleteAgy: %v", err)
	}
	z := szallasOf(t, h, "anna", d.ID)
	expectSzallasLayout(t, z, "Szoba[]")
	if got := agyNelkulIDs(z); fmt.Sprint(got) != "[anna bela cecil dora]" {
		t.Fatalf("without a bed = %v", got)
	}
	if err := h.App.DeleteAgy(ctx, "anna", d.ID, double.ID); !errors.Is(err, app.ErrNotFound) {
		t.Fatalf("deleting it again: err = %v, want ErrNotFound", err)
	}
}

func TestWhenASleeperDropsOutTheirSpotAndMarkAreFreedWithoutDMsToOthers(t *testing.T) {
	h, d := szallasTrip(t)
	ctx := context.Background()
	szoba := addSzoba(t, h, d.ID, "Emeleti")
	agy := addAgy(t, h, d.ID, szoba.ID, 2)
	claimAgy(t, h, "bela", d.ID, agy.ID)
	claimAgy(t, h, "cecil", d.ID, agy.ID)
	if err := h.App.SetMasholAlszik(ctx, "dora", d.ID, app.MasholAlszikInput{On: true}); err != nil {
		t.Fatalf("SetMasholAlszik: %v", err)
	}

	answer(t, h, "bela", d, 0, domain.Nem)
	answer(t, h, "dora", d, 0, domain.Talan)

	z := szallasOf(t, h, "anna", d.ID)
	expectSzallasLayout(t, z, "Emeleti[cecil]")
	if len(z.MasholAlszik) != 0 {
		t.Fatalf("máshol alszik = %+v, want the mark cleared", z.MasholAlszik)
	}
	drop := dropOutDMs(h)
	if len(drop) != 2 || drop[0].DropOut.Felszabadult != "1 ágy (Emeleti)" || drop[1].DropOut.Felszabadult != "" {
		t.Fatalf("drop-out DMs = %+v", drop)
	}
	if n := len(szallasDMs(h)); n != 0 {
		t.Fatalf("%d szallas DMs, want none for people leaving", n)
	}
	// Dora's mark is gone when she rejoins.
	answer(t, h, "dora", d, 0, domain.Igen)
	if z := szallasOf(t, h, "dora", d.ID); z.SajatMasholAlszik {
		t.Fatalf("Dora is still máshol alszik")
	}
}

func TestADropOutWhileSzallasIsOffChangesSilently(t *testing.T) {
	h, d := szallasTrip(t)
	szoba := addSzoba(t, h, d.ID, "Szoba")
	agy := addAgy(t, h, d.ID, szoba.ID, 1)
	claimAgy(t, h, "bela", d.ID, agy.ID)
	switchKiegeszito(t, h, "anna", d.ID, domain.Szallas, false)

	answer(t, h, "bela", d, 0, domain.Nem)

	if dms := dropOutDMs(h); len(dms) != 1 || dms[0].DropOut.Felszabadult != "" {
		t.Fatalf("drop-out DMs = %+v, want one that mentions nothing freed", dms)
	}
	switchKiegeszito(t, h, "anna", d.ID, domain.Szallas, true)
	expectSzallasLayout(t, szallasOf(t, h, "anna", d.ID), "Szoba[]")
}

func TestASleeperLeavingTheCsoportLosesTheirSpot(t *testing.T) {
	h, d := szallasTrip(t)
	e, err := h.Store.FindEsemeny(context.Background(), d.ID)
	if err != nil {
		t.Fatalf("FindEsemeny: %v", err)
	}
	szoba := addSzoba(t, h, d.ID, "Szoba")
	agy := addAgy(t, h, d.ID, szoba.ID, 2)
	claimAgy(t, h, "bela", d.ID, agy.ID)
	claimAgy(t, h, "cecil", d.ID, agy.ID)

	if err := h.App.LeaveCsoport(context.Background(), "bela", e.CsoportID); err != nil {
		t.Fatalf("LeaveCsoport: %v", err)
	}

	expectSzallasLayout(t, szallasOf(t, h, "anna", d.ID), "Szoba[cecil]")
	if n := len(szallasDMs(h)); n != 0 {
		t.Fatalf("%d szallas DMs, want none", n)
	}
}

func TestReConfirmingFreesTheSpotOfASleeperWhoIsNoResztvevoAnyMore(t *testing.T) {
	h, d := szallasTrip(t)
	ctx := context.Background()
	szoba := addSzoba(t, h, d.ID, "Szoba")
	agy := addAgy(t, h, d.ID, szoba.ID, 2)
	claimAgy(t, h, "bela", d.ID, agy.ID)
	claimAgy(t, h, "cecil", d.ID, agy.ID)
	if err := h.App.SetMasholAlszik(ctx, "dora", d.ID, app.MasholAlszikInput{On: true}); err != nil {
		t.Fatalf("SetMasholAlszik: %v", err)
	}

	// Claims are frozen while Egyeztetes is open again. Everyone is in for
	// the second Idopont, but Bela said nem to the first.
	reopen(t, h, "anna", d.ID)
	answer(t, h, "bela", d, 0, domain.Nem)
	for _, who := range []string{"anna", "bela", "cecil", "dora"} {
		answer(t, h, who, d, 1, domain.Igen)
	}
	if err := h.App.ReleaseFoglalas(ctx, "cecil", d.ID, app.FoglalasInput{}); !errors.Is(err, app.ErrClaimingClosed) {
		t.Fatalf("leaving while frozen: err = %v, want ErrClaimingClosed", err)
	}
	expectSzallasLayout(t, szallasOf(t, h, "anna", d.ID), "Szoba[bela,cecil]")
	// Confirm the first one again: Bela is no Resztvevo of it.
	confirm(t, h, "anna", d, 0)

	expectSzallasLayout(t, szallasOf(t, h, "anna", d.ID), "Szoba[cecil]")
	if z := szallasOf(t, h, "dora", d.ID); !z.SajatMasholAlszik {
		t.Fatalf("Dora is still a Resztvevo, so her mark stays")
	}
	if n := len(dropOutDMs(h)); n != 0 {
		t.Fatalf("%d drop-out DMs, want none for a re-confirmation", n)
	}
	if n := len(szallasDMs(h)); n != 0 {
		t.Fatalf("%d szallas DMs, want none", n)
	}
}

func TestTheEsemenyMessageSummarisesSzallas(t *testing.T) {
	h := apptest.New(t)
	ctx := context.Background()
	c := csoportOf(t, h, "bela", "cecil")
	linkCsatorna(t, h, c.ID)
	e := propose(t, h, "anna", c.ID, 1)
	d := detail(t, h, "anna", e.ID)
	lastSummary := func() string {
		t.Helper()
		edits := h.Discord.Edits()
		if len(edits) == 0 {
			t.Fatalf("no edits")
		}
		return edits[len(edits)-1].Message.KiegeszitoSummary
	}

	switchKiegeszito(t, h, "anna", e.ID, domain.Szallas, true)
	answer(t, h, "anna", d, 0, domain.Igen)
	answer(t, h, "bela", d, 0, domain.Igen)
	answer(t, h, "cecil", d, 0, domain.Igen)
	confirm(t, h, "anna", d, 0)
	if got := lastSummary(); got != "" {
		t.Fatalf("summary with nothing set up = %q, want none", got)
	}

	szoba := addSzoba(t, h, e.ID, "Szoba")
	if got := lastSummary(); got != "🛏 3 főnek nincs ágya" {
		t.Fatalf("summary = %q, want 3 without a bed", got)
	}
	agy := addAgy(t, h, e.ID, szoba.ID, 2)
	claimAgy(t, h, "bela", e.ID, agy.ID)
	if got := lastSummary(); got != "🛏 2 főnek nincs ágya" {
		t.Fatalf("summary = %q, want 2 without a bed", got)
	}
	claimAgy(t, h, "cecil", e.ID, agy.ID)
	if err := h.App.SetMasholAlszik(ctx, "anna", e.ID, app.MasholAlszikInput{On: true}); err != nil {
		t.Fatalf("SetMasholAlszik: %v", err)
	}
	if got := lastSummary(); got != "🛏 mindenkinek van ágya" {
		t.Fatalf("summary = %q, want everyone placed", got)
	}
	// Only a mark, no Szoba: still "set up".
	if err := h.App.DeleteSzoba(ctx, "anna", e.ID, szoba.ID); err != nil {
		t.Fatalf("DeleteSzoba: %v", err)
	}
	if got := lastSummary(); got != "🛏 2 főnek nincs ágya" {
		t.Fatalf("summary = %q, want 2 without a bed", got)
	}
	switchKiegeszito(t, h, "anna", e.ID, domain.Szallas, false)
	if got := lastSummary(); got != "" {
		t.Fatalf("summary once switched off = %q, want none", got)
	}
}

func TestSzallasIsOneOfTheSwitchableKiegeszitok(t *testing.T) {
	h := apptest.New(t)
	c := csoportOf(t, h, "bela")
	e := propose(t, h, "anna", c.ID, 1)
	d := detail(t, h, "anna", e.ID)
	if k := kiegeszitoOf(t, d.Kiegeszitok, domain.Szallas); k.Enabled || !k.Switchable {
		t.Fatalf("szallas = %+v, want off and switchable", k)
	}
	switchKiegeszito(t, h, "anna", e.ID, domain.Szallas, true)
	d = detail(t, h, "anna", e.ID)
	if k := kiegeszitoOf(t, d.Kiegeszitok, domain.Szallas); !k.Enabled {
		t.Fatalf("szallas = %+v, want on", k)
	}
}
