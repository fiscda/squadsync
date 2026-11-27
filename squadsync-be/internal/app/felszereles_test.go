package app_test

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"squadsync-be/internal/app"
	"squadsync-be/internal/apptest"
	"squadsync-be/internal/domain"
)

// confirmedTrip is tripWithFelszereles, confirmed with Anna and Bela as
// Resztvevok and Cecil saying nem.
func confirmedTrip(t *testing.T) (*apptest.Harness, app.EsemenyDetail, domain.Tetel) {
	t.Helper()
	h, d, sator := tripWithFelszereles(t)
	answer(t, h, "anna", d, 0, domain.Igen)
	answer(t, h, "bela", d, 0, domain.Igen)
	answer(t, h, "cecil", d, 0, domain.Nem)
	confirm(t, h, "anna", d, 0)
	return h, d, sator
}

func tetelAllas(t *testing.T, f app.Felszereles, id string) app.TetelAllas {
	t.Helper()
	for _, allas := range f.Tetelek {
		if allas.ID == id {
			return allas
		}
	}
	t.Fatalf("tetel %s not in %+v", id, f.Tetelek)
	return app.TetelAllas{}
}

func TestPartialClaimsAndWhatIsStillMissing(t *testing.T) {
	h, d, sator := confirmedTrip(t)
	ctx := context.Background()
	gaz := addTetel(t, h, "bela", d.ID, "gázpalack", 0) // default quantity

	if gaz.Quantity != 1 {
		t.Fatalf("quantity = %d, want the default 1", gaz.Quantity)
	}
	f := felszereles(t, h, "anna", d.ID)
	if f.Hianyzik != 4 {
		t.Fatalf("hianyzik = %d, want 3 tents and 1 gas", f.Hianyzik)
	}

	vallal(t, h, "anna", d.ID, sator.ID, 2)
	allas := tetelAllas(t, felszereles(t, h, "anna", d.ID), sator.ID)
	if allas.Vallalt != 2 || allas.Hianyzik != 1 || allas.Sajat != 2 {
		t.Fatalf("after Anna claims 2 of 3: %+v", allas)
	}

	if err := h.App.SetVallalas(ctx, "bela", d.ID, sator.ID, app.VallalasInput{Quantity: 2}); !errors.Is(err, app.ErrVallalasTooMuch) {
		t.Fatalf("claiming more than missing: err = %v, want ErrVallalasTooMuch", err)
	}
	vallal(t, h, "bela", d.ID, sator.ID, 1)
	// Anna changes her claim within what is missing plus her own.
	if err := h.App.SetVallalas(ctx, "anna", d.ID, sator.ID, app.VallalasInput{Quantity: 3}); !errors.Is(err, app.ErrVallalasTooMuch) {
		t.Fatalf("raising a claim past the quantity: err = %v, want ErrVallalasTooMuch", err)
	}
	vallal(t, h, "anna", d.ID, sator.ID, 1)
	f = felszereles(t, h, "bela", d.ID)
	allas = tetelAllas(t, f, sator.ID)
	if allas.Vallalt != 2 || allas.Hianyzik != 1 || allas.Sajat != 1 || f.Hianyzik != 2 {
		t.Fatalf("after changes: %+v, total missing %d", allas, f.Hianyzik)
	}
	expectVallalasok(t, f, "sátor:anna=1,bela=1", "gázpalack:")

	// Removing a claim.
	vallal(t, h, "anna", d.ID, sator.ID, 0)
	expectVallalasok(t, felszereles(t, h, "anna", d.ID), "sátor:bela=1", "gázpalack:")

	for _, bad := range []int{-1} {
		if err := h.App.SetVallalas(ctx, "anna", d.ID, sator.ID, app.VallalasInput{Quantity: bad}); !errors.Is(err, app.ErrInvalidInput) {
			t.Fatalf("claiming %d: err = %v, want ErrInvalidInput", bad, err)
		}
	}
	if err := h.App.SetVallalas(ctx, "anna", d.ID, "no-such-tetel", app.VallalasInput{Quantity: 1}); !errors.Is(err, app.ErrNotFound) {
		t.Fatalf("claiming an unknown Tetel: err = %v, want ErrNotFound", err)
	}
	if err := h.App.SetVallalas(ctx, "anna", d.ID, sator.ID, app.VallalasInput{Quantity: 1, VendegID: "v1"}); !errors.Is(err, app.ErrNotFound) {
		t.Fatalf("claiming for a Vendeg before #13: err = %v, want ErrNotFound", err)
	}
}

func TestMindenkiHozzaTetelekAreRemindersNotClaims(t *testing.T) {
	h, d, _ := confirmedTrip(t)
	ctx := context.Background()
	halozsak, err := h.App.AddTetel(ctx, "bela", d.ID, app.TetelInput{Name: "hálózsák", MindenkiHozza: true})
	if err != nil {
		t.Fatalf("AddTetel: %v", err)
	}

	if err := h.App.SetVallalas(ctx, "bela", d.ID, halozsak.ID, app.VallalasInput{Quantity: 1}); !errors.Is(err, app.ErrTetelMindenkiHozza) {
		t.Fatalf("claiming a mindenki hozza Tetel: err = %v, want ErrTetelMindenkiHozza", err)
	}
	f := felszereles(t, h, "bela", d.ID)
	if fmt.Sprint(f.Emlekezteto) != "[hálózsák]" {
		t.Fatalf("Bela's reminders = %v, want [hálózsák]", f.Emlekezteto)
	}
	if f.Hianyzik != 3 || tetelAllas(t, f, halozsak.ID).Hianyzik != 0 {
		t.Fatalf("hianyzik = %d, want only the tents to count", f.Hianyzik)
	}
	if f := felszereles(t, h, "cecil", d.ID); len(f.Emlekezteto) != 0 {
		t.Fatalf("Cecil (not a Resztvevo) has reminders %v", f.Emlekezteto)
	}
}

func TestWhoMayAddEditAndDeleteTetelek(t *testing.T) {
	h, d, sator := confirmedTrip(t)
	ctx := context.Background()

	// Once Veglegesitve, only Resztvevok and the Szervezo add Tetelek.
	if _, err := h.App.AddTetel(ctx, "cecil", d.ID, app.TetelInput{Name: "bor"}); !errors.Is(err, app.ErrNotPermitted) {
		t.Fatalf("a non-Resztvevo adding: err = %v, want ErrNotPermitted", err)
	}
	if f := felszereles(t, h, "cecil", d.ID); f.CanAdd {
		t.Fatalf("Cecil sees CanAdd")
	}
	answer(t, h, "anna", d, 0, domain.Nem) // the Szervezo still organises
	gaz := addTetel(t, h, "anna", d.ID, "gázpalack", 1)
	bogracs := addTetel(t, h, "bela", d.ID, "bogrács", 1)

	for _, bad := range []app.TetelInput{{Name: "  "}, {Name: "x", Quantity: -1}, {Name: strings.Repeat("x", 101)}} {
		if _, err := h.App.AddTetel(ctx, "bela", d.ID, bad); !errors.Is(err, app.ErrInvalidInput) {
			t.Fatalf("AddTetel(%+v): err = %v, want ErrInvalidInput", bad, err)
		}
	}

	// Its adder and the Szervezo edit or delete it; nobody else.
	if err := h.App.UpdateTetel(ctx, "cecil", d.ID, bogracs.ID, app.TetelInput{Name: "nagy bogrács"}); !errors.Is(err, app.ErrNotPermitted) {
		t.Fatalf("someone else editing: err = %v, want ErrNotPermitted", err)
	}
	if err := h.App.DeleteTetel(ctx, "cecil", d.ID, bogracs.ID); !errors.Is(err, app.ErrNotPermitted) {
		t.Fatalf("someone else deleting: err = %v, want ErrNotPermitted", err)
	}
	if err := h.App.UpdateTetel(ctx, "bela", d.ID, bogracs.ID, app.TetelInput{Name: "nagy bogrács", Quantity: 2}); err != nil {
		t.Fatalf("its adder editing: %v", err)
	}
	if err := h.App.UpdateTetel(ctx, "anna", d.ID, bogracs.ID, app.TetelInput{Name: "bogrács", Quantity: 1}); err != nil {
		t.Fatalf("the Szervezo editing: %v", err)
	}
	f := felszereles(t, h, "bela", d.ID)
	if !tetelAllas(t, f, bogracs.ID).CanEdit || tetelAllas(t, f, gaz.ID).CanEdit {
		t.Fatalf("Bela's CanEdit: own %v, Anna's %v; want true false", tetelAllas(t, f, bogracs.ID).CanEdit, tetelAllas(t, f, gaz.ID).CanEdit)
	}
	if !tetelAllas(t, felszereles(t, h, "anna", d.ID), bogracs.ID).CanEdit {
		t.Fatalf("the Szervezo can't edit Bela's Tetel")
	}
	if err := h.App.DeleteTetel(ctx, "bela", d.ID, gaz.ID); !errors.Is(err, app.ErrNotPermitted) {
		t.Fatalf("Bela deleting Anna's Tetel: err = %v, want ErrNotPermitted", err)
	}
	if err := h.App.DeleteTetel(ctx, "anna", d.ID, bogracs.ID); err != nil {
		t.Fatalf("the Szervezo deleting: %v", err)
	}

	// Edits can't contradict the claims.
	vallal(t, h, "bela", d.ID, sator.ID, 2)
	if err := h.App.UpdateTetel(ctx, "anna", d.ID, sator.ID, app.TetelInput{Name: "sátor", Quantity: 1}); !errors.Is(err, app.ErrTetelClaimed) {
		t.Fatalf("lowering below the claims: err = %v, want ErrTetelClaimed", err)
	}
	if err := h.App.UpdateTetel(ctx, "anna", d.ID, sator.ID, app.TetelInput{Name: "sátor", Quantity: 3, MindenkiHozza: true}); !errors.Is(err, app.ErrTetelClaimed) {
		t.Fatalf("making a claimed Tetel mindenki hozza: err = %v, want ErrTetelClaimed", err)
	}
	if err := h.App.UpdateTetel(ctx, "anna", d.ID, sator.ID, app.TetelInput{Name: "sátor", Quantity: 2}); err != nil {
		t.Fatalf("lowering to the claims: %v", err)
	}
	if err := h.App.DeleteTetel(ctx, "anna", d.ID, sator.ID); err != nil {
		t.Fatalf("deleting a claimed Tetel: %v", err)
	}
	expectVallalasok(t, felszereles(t, h, "anna", d.ID), "gázpalack:")

	if err := h.App.DeleteTetel(ctx, "anna", d.ID, sator.ID); !errors.Is(err, app.ErrNotFound) {
		t.Fatalf("deleting it again: err = %v, want ErrNotFound", err)
	}
}

func TestTheEsemenyMessageSummarisesFelszereles(t *testing.T) {
	h := apptest.New(t)
	c := csoportOf(t, h, "bela")
	linkCsatorna(t, h, c.ID)
	e := propose(t, h, "anna", c.ID, 1)
	d := detail(t, h, "anna", e.ID)
	if m := h.Discord.Posts()[0].Message; m.KiegeszitoSummary != "" {
		t.Fatalf("summary without Kiegeszitok = %q, want none", m.KiegeszitoSummary)
	}
	lastSummary := func() string {
		t.Helper()
		edits := h.Discord.Edits()
		if len(edits) == 0 {
			t.Fatalf("no edits")
		}
		return edits[len(edits)-1].Message.KiegeszitoSummary
	}

	switchKiegeszito(t, h, "anna", e.ID, domain.Felszereles, true)
	if got := lastSummary(); got != "🎒 minden megvan" {
		t.Fatalf("summary with an empty list = %q", got)
	}
	sator := addTetel(t, h, "anna", e.ID, "sátor", 3)
	if got := lastSummary(); got != "🎒 3 hiányzik" {
		t.Fatalf("summary = %q, want 3 missing", got)
	}
	answer(t, h, "bela", d, 0, domain.Igen)
	confirm(t, h, "anna", d, 0)
	vallal(t, h, "bela", e.ID, sator.ID, 3)
	if got := lastSummary(); got != "🎒 minden megvan" {
		t.Fatalf("summary once all is claimed = %q", got)
	}
	answer(t, h, "bela", d, 0, domain.Nem)
	if got := lastSummary(); got != "🎒 3 hiányzik" {
		t.Fatalf("summary after the drop-out = %q, want the tents missing again", got)
	}
	switchKiegeszito(t, h, "anna", e.ID, domain.Felszereles, false)
	if got := lastSummary(); got != "" {
		t.Fatalf("summary once switched off = %q, want none", got)
	}
}
