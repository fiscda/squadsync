package app_test

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"squadsync-be/internal/app"
	"squadsync-be/internal/apptest"
	"squadsync-be/internal/domain"
	"squadsync-be/internal/gateway"
)

func switchKiegeszito(t *testing.T, h *apptest.Harness, felhasznaloID, esemenyID string, kind domain.KiegeszitoKind, on bool) {
	t.Helper()
	if err := h.App.SetKiegeszito(context.Background(), felhasznaloID, esemenyID, kind, on); err != nil {
		t.Fatalf("SetKiegeszito(%s, %s, %v): %v", felhasznaloID, kind, on, err)
	}
}

func addTetel(t *testing.T, h *apptest.Harness, felhasznaloID, esemenyID, name string, quantity int) domain.Tetel {
	t.Helper()
	tetel, err := h.App.AddTetel(context.Background(), felhasznaloID, esemenyID, app.TetelInput{Name: name, Quantity: quantity})
	if err != nil {
		t.Fatalf("AddTetel(%s, %s): %v", felhasznaloID, name, err)
	}
	return tetel
}

func vallal(t *testing.T, h *apptest.Harness, felhasznaloID, esemenyID, tetelID string, quantity int) {
	t.Helper()
	if err := h.App.SetVallalas(context.Background(), felhasznaloID, esemenyID, tetelID, app.VallalasInput{Quantity: quantity}); err != nil {
		t.Fatalf("SetVallalas(%s, %d): %v", felhasznaloID, quantity, err)
	}
}

func felszereles(t *testing.T, h *apptest.Harness, felhasznaloID, esemenyID string) app.Felszereles {
	t.Helper()
	f, err := h.App.Felszereles(context.Background(), felhasznaloID, esemenyID)
	if err != nil {
		t.Fatalf("Felszereles(%s): %v", felhasznaloID, err)
	}
	return f
}

// vallalasok shows each Tetel's claims as "name:holder=quantity,...".
func vallalasok(f app.Felszereles) []string {
	out := []string{}
	for _, t := range f.Tetelek {
		s := t.Name + ":"
		for n, v := range t.Vallalasok {
			if n > 0 {
				s += ","
			}
			s += fmt.Sprintf("%s=%d", v.Holder.FelhasznaloID, v.Quantity)
		}
		out = append(out, s)
	}
	return out
}

func expectVallalasok(t *testing.T, f app.Felszereles, want ...string) {
	t.Helper()
	got := vallalasok(f)
	if fmt.Sprint(got) != fmt.Sprint(want) {
		t.Fatalf("vallalasok = %v, want %v", got, want)
	}
}

// tripWithFelszereles has Anna propose an Esemeny in a Csoport with Bela
// and Cecil, switch Felszereles on and add a Tetel of 3 tents.
func tripWithFelszereles(t *testing.T) (*apptest.Harness, app.EsemenyDetail, domain.Tetel) {
	t.Helper()
	h := apptest.New(t)
	c := csoportOf(t, h, "bela", "cecil")
	e := propose(t, h, "anna", c.ID, 2)
	switchKiegeszito(t, h, "anna", e.ID, domain.Felszereles, true)
	sator := addTetel(t, h, "anna", e.ID, "sátor", 3)
	return h, detail(t, h, "anna", e.ID), sator
}

func TestSwitchingAKiegeszitoOffHidesItsDataAndOnRestoresItUnchanged(t *testing.T) {
	h, d, sator := tripWithFelszereles(t)
	ctx := context.Background()
	answer(t, h, "bela", d, 0, domain.Igen)
	confirm(t, h, "anna", d, 0)
	vallal(t, h, "bela", d.ID, sator.ID, 2)
	before := felszereles(t, h, "bela", d.ID)

	switchKiegeszito(t, h, "anna", d.ID, domain.Felszereles, false)

	if _, err := h.App.Felszereles(ctx, "bela", d.ID); !errors.Is(err, app.ErrKiegeszitoOff) {
		t.Fatalf("Felszereles while off: err = %v, want ErrKiegeszitoOff", err)
	}
	if _, err := h.App.AddTetel(ctx, "bela", d.ID, app.TetelInput{Name: "bogrács"}); !errors.Is(err, app.ErrKiegeszitoOff) {
		t.Fatalf("AddTetel while off: err = %v, want ErrKiegeszitoOff", err)
	}
	if err := h.App.SetVallalas(ctx, "bela", d.ID, sator.ID, app.VallalasInput{Quantity: 1}); !errors.Is(err, app.ErrKiegeszitoOff) {
		t.Fatalf("SetVallalas while off: err = %v, want ErrKiegeszitoOff", err)
	}
	got := detail(t, h, "bela", d.ID).Kiegeszitok
	if len(got) != 1 || got[0] != (app.KiegeszitoAllapot{Kind: domain.Felszereles, Enabled: false, Switchable: true}) {
		t.Fatalf("kiegeszitok while off = %+v", got)
	}

	switchKiegeszito(t, h, "anna", d.ID, domain.Felszereles, true)

	after := felszereles(t, h, "bela", d.ID)
	if fmt.Sprintf("%+v", after) != fmt.Sprintf("%+v", before) {
		t.Fatalf("after switching back on = %+v, want it unchanged: %+v", after, before)
	}
	if got := detail(t, h, "bela", d.ID).Kiegeszitok; !got[0].Enabled {
		t.Fatalf("kiegeszitok = %+v, want Felszereles on", got)
	}
}

func TestOnlyTheSzervezoSwitchesKiegeszitok(t *testing.T) {
	h := apptest.New(t)
	ctx := context.Background()
	c := csoportOf(t, h, "bela")
	addFelhasznalo(t, h, "dani", "dani") // not a Tag
	e := propose(t, h, "anna", c.ID, 1)

	for who, want := range map[string]error{"bela": app.ErrNotSzervezo, "dani": app.ErrNotFound} {
		if err := h.App.SetKiegeszito(ctx, who, e.ID, domain.Felszereles, true); !errors.Is(err, want) {
			t.Errorf("SetKiegeszito by %s: err = %v, want %v", who, err, want)
		}
	}
	for _, kind := range []domain.KiegeszitoKind{"sorok", domain.Autok} { // Autok isn't built yet
		if err := h.App.SetKiegeszito(ctx, "anna", e.ID, kind, true); !errors.Is(err, app.ErrInvalidInput) {
			t.Errorf("SetKiegeszito(%s): err = %v, want ErrInvalidInput", kind, err)
		}
	}
	if got := detail(t, h, "anna", e.ID).Kiegeszitok; len(got) != 1 || got[0].Enabled {
		t.Fatalf("kiegeszitok = %+v, want Felszereles off", got)
	}
	// Switching to the state it is in changes nothing.
	switchKiegeszito(t, h, "anna", e.ID, domain.Felszereles, false)
	switchKiegeszito(t, h, "anna", e.ID, domain.Felszereles, true)
	switchKiegeszito(t, h, "anna", e.ID, domain.Felszereles, true)
	if got := detail(t, h, "anna", e.ID).Kiegeszitok; !got[0].Enabled {
		t.Fatalf("kiegeszitok = %+v, want Felszereles on", got)
	}
}

func TestNothingCanBeSwitchedOnceElmaradt(t *testing.T) {
	h, d, _ := tripWithFelszereles(t)
	cancelEsemeny(t, h, "anna", d.ID)

	for _, on := range []bool{false, true} {
		if err := h.App.SetKiegeszito(context.Background(), "anna", d.ID, domain.Felszereles, on); !errors.Is(err, app.ErrKiegeszitoLocked) {
			t.Fatalf("SetKiegeszito(%v) once Elmaradt: err = %v, want ErrKiegeszitoLocked", on, err)
		}
	}
	if got := detail(t, h, "anna", d.ID).Kiegeszitok; !got[0].Enabled || got[0].Switchable {
		t.Fatalf("kiegeszitok = %+v, want Felszereles on and not switchable", got)
	}
}

func TestAKiegeszitoCanVetoBeingSwitchedOff(t *testing.T) {
	h, d, _ := tripWithFelszereles(t)
	veto := fmt.Errorf("%w: van kiadás", app.ErrKiegeszitoInUse)
	h.App.VetoSwitchOff(domain.Felszereles, veto)

	err := h.App.SetKiegeszito(context.Background(), "anna", d.ID, domain.Felszereles, false)
	if !errors.Is(err, app.ErrKiegeszitoInUse) {
		t.Fatalf("switching off: err = %v, want the veto", err)
	}
	if got := detail(t, h, "anna", d.ID).Kiegeszitok; !got[0].Enabled {
		t.Fatalf("kiegeszitok = %+v, want Felszereles still on", got)
	}

	h.App.VetoSwitchOff(domain.Felszereles, nil)
	switchKiegeszito(t, h, "anna", d.ID, domain.Felszereles, false)
}

func TestSetupIsAllowedInEgyeztetesButClaimingOnlyInVeglegesitveByResztvevok(t *testing.T) {
	h, d, sator := tripWithFelszereles(t)
	ctx := context.Background()
	claim := func(who string) error {
		return h.App.SetVallalas(ctx, who, d.ID, sator.ID, app.VallalasInput{Quantity: 1})
	}

	// Any Tag sets up during Egyeztetes, and nobody claims yet.
	addTetel(t, h, "cecil", d.ID, "bogrács", 1)
	if f := felszereles(t, h, "cecil", d.ID); !f.CanAdd || f.CanClaim {
		t.Fatalf("in Egyeztetes: CanAdd %v CanClaim %v, want true false", f.CanAdd, f.CanClaim)
	}
	if err := claim("bela"); !errors.Is(err, app.ErrClaimingClosed) {
		t.Fatalf("claiming in Egyeztetes: err = %v, want ErrClaimingClosed", err)
	}

	answer(t, h, "bela", d, 0, domain.Igen)
	answer(t, h, "cecil", d, 0, domain.Nem)
	confirm(t, h, "anna", d, 0)

	if err := claim("bela"); err != nil {
		t.Fatalf("a Resztvevo claiming in Veglegesitve: %v", err)
	}
	for _, who := range []string{"cecil", "anna"} { // nem, and no answer at all
		if err := claim(who); !errors.Is(err, app.ErrNotResztvevo) {
			t.Fatalf("%s claiming: err = %v, want ErrNotResztvevo", who, err)
		}
	}
	if f := felszereles(t, h, "cecil", d.ID); f.CanClaim {
		t.Fatalf("a non-Resztvevo sees CanClaim")
	}
	if f := felszereles(t, h, "bela", d.ID); !f.CanClaim {
		t.Fatalf("a Resztvevo doesn't see CanClaim")
	}

	cancelEsemeny(t, h, "anna", d.ID)
	if err := claim("bela"); !errors.Is(err, app.ErrClaimingClosed) {
		t.Fatalf("claiming once Elmaradt: err = %v, want ErrClaimingClosed", err)
	}
	if _, err := h.App.AddTetel(ctx, "anna", d.ID, app.TetelInput{Name: "fa"}); !errors.Is(err, app.ErrSetupClosed) {
		t.Fatalf("setting up once Elmaradt: err = %v, want ErrSetupClosed", err)
	}
	if err := h.App.DeleteTetel(ctx, "anna", d.ID, sator.ID); !errors.Is(err, app.ErrSetupClosed) {
		t.Fatalf("deleting once Elmaradt: err = %v, want ErrSetupClosed", err)
	}
}

func TestADropOutReleasesTheirClaimsAndTheSzervezoHearsWhatWasFreed(t *testing.T) {
	h, d, sator := tripWithFelszereles(t)
	gaz := addTetel(t, h, "bela", d.ID, "gázpalack", 1)
	for _, who := range []string{"anna", "bela", "cecil"} {
		answer(t, h, who, d, 0, domain.Igen)
	}
	confirm(t, h, "anna", d, 0)
	vallal(t, h, "bela", d.ID, sator.ID, 2)
	vallal(t, h, "bela", d.ID, gaz.ID, 1)
	vallal(t, h, "anna", d.ID, sator.ID, 1)
	vallal(t, h, "cecil", d.ID, sator.ID, 0) // nothing left to claim

	answer(t, h, "bela", d, 0, domain.Nem)

	expectVallalasok(t, felszereles(t, h, "anna", d.ID), "sátor:anna=1", "gázpalack:")
	want := gateway.DropOut{
		Name: "bela", CsoportName: "Péntek esték", Title: "Társasjáték", Link: esemenyURL(d.ID),
		Felszabadult: "2 db sátor, 1 db gázpalack",
	}
	dms := dropOutDMs(h)
	if len(dms) != 1 || dms[0].DiscordID != "discord-anna" || *dms[0].DropOut != want {
		t.Fatalf("drop-out DMs = %+v, want %+v to Anna", dms, want)
	}

	// The Szervezo's own drop-out releases their claims too, without a DM.
	answer(t, h, "anna", d, 0, domain.Talan)
	expectVallalasok(t, felszereles(t, h, "cecil", d.ID), "sátor:", "gázpalack:")
	if n := len(dropOutDMs(h)); n != 1 {
		t.Fatalf("%d drop-out DMs, want no new one for the Szervezo", n)
	}
	if f := felszereles(t, h, "cecil", d.ID); f.Hianyzik != 4 {
		t.Fatalf("hianyzik = %d, want everything missing again", f.Hianyzik)
	}
}

func TestClaimsInASwitchedOffKiegeszitoAreReleasedButNotMentioned(t *testing.T) {
	h, d, sator := tripWithFelszereles(t)
	answer(t, h, "bela", d, 0, domain.Igen)
	confirm(t, h, "anna", d, 0)
	vallal(t, h, "bela", d.ID, sator.ID, 2)
	switchKiegeszito(t, h, "anna", d.ID, domain.Felszereles, false)

	answer(t, h, "bela", d, 0, domain.Nem)

	dms := dropOutDMs(h)
	if len(dms) != 1 || dms[0].DropOut.Felszabadult != "" {
		t.Fatalf("drop-out DMs = %+v, want one that mentions nothing freed", dms)
	}
	switchKiegeszito(t, h, "anna", d.ID, domain.Felszereles, true)
	expectVallalasok(t, felszereles(t, h, "anna", d.ID), "sátor:")
}

func TestReopeningFreezesClaimsAndReConfirmingDropsThoseOfNonResztvevok(t *testing.T) {
	h, d, sator := tripWithFelszereles(t)
	ctx := context.Background()
	answer(t, h, "bela", d, 0, domain.Igen)
	answer(t, h, "bela", d, 1, domain.Igen)
	answer(t, h, "cecil", d, 0, domain.Igen)
	answer(t, h, "cecil", d, 1, domain.Talan)
	confirm(t, h, "anna", d, 0)
	vallal(t, h, "bela", d.ID, sator.ID, 1)
	vallal(t, h, "cecil", d.ID, sator.ID, 2)

	reopen(t, h, "anna", d.ID)

	// Frozen: kept, but nobody can claim.
	expectVallalasok(t, felszereles(t, h, "anna", d.ID), "sátor:bela=1,cecil=2")
	if err := h.App.SetVallalas(ctx, "bela", d.ID, sator.ID, app.VallalasInput{Quantity: 0}); !errors.Is(err, app.ErrClaimingClosed) {
		t.Fatalf("changing a claim after reopening: err = %v, want ErrClaimingClosed", err)
	}
	// Setup goes on.
	addTetel(t, h, "cecil", d.ID, "bogrács", 1)

	confirm(t, h, "anna", d, 1) // Cecil only said talan

	expectVallalasok(t, felszereles(t, h, "anna", d.ID), "sátor:bela=1", "bogrács:")
	if dms := dropOutDMs(h); len(dms) != 0 {
		t.Fatalf("drop-out DMs = %+v, want none for a re-confirmation", dms)
	}
	vallal(t, h, "bela", d.ID, sator.ID, 3)
}
