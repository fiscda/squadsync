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

// autokTrip has Anna (the Szervezo) propose an Esemeny with two Idopontok
// in a Csoport with Bela, Cecil, Dora and Emil, switch Autok on, and
// confirm the first Idopont with Anna, Bela, Cecil, Dora and Emil as
// Resztvevok except Emil, who says nem.
func autokTrip(t *testing.T) (*apptest.Harness, app.EsemenyDetail) {
	t.Helper()
	h := apptest.New(t)
	c := csoportOf(t, h, "bela", "cecil", "dora", "emil")
	e := propose(t, h, "anna", c.ID, 2)
	switchKiegeszito(t, h, "anna", e.ID, domain.Autok, true)
	d := detail(t, h, "anna", e.ID)
	for _, who := range []string{"anna", "bela", "cecil", "dora"} {
		answer(t, h, who, d, 0, domain.Igen)
	}
	answer(t, h, "emil", d, 0, domain.Nem)
	confirm(t, h, "anna", d, 0)
	return h, d
}

func autokOf(t *testing.T, h *apptest.Harness, felhasznaloID, esemenyID string) app.Autok {
	t.Helper()
	a, err := h.App.Autok(context.Background(), felhasznaloID, esemenyID)
	if err != nil {
		t.Fatalf("Autok(%s): %v", felhasznaloID, err)
	}
	return a
}

func autoOffer(t *testing.T, h *apptest.Harness, felhasznaloID, esemenyID string, seats int) domain.Auto {
	t.Helper()
	a, err := h.App.OfferAuto(context.Background(), felhasznaloID, esemenyID, app.AutoInput{Seats: seats, Note: "indulás 8:00, Keleti"})
	if err != nil {
		t.Fatalf("OfferAuto(%s, %d): %v", felhasznaloID, seats, err)
	}
	return a
}

func autoClaim(t *testing.T, h *apptest.Harness, felhasznaloID, esemenyID, autoID string) {
	t.Helper()
	if err := h.App.ClaimUles(context.Background(), felhasznaloID, esemenyID, autoID, app.UtasInput{}); err != nil {
		t.Fatalf("ClaimUles(%s): %v", felhasznaloID, err)
	}
}

// autoLayout shows each Auto as "sofor:utas,utas", in order.
func autoLayout(a app.Autok) []string {
	out := []string{}
	for _, auto := range a.Autok {
		s := auto.SoforID + ":"
		for n, u := range auto.Utasok {
			if n > 0 {
				s += ","
			}
			s += u.Holder.FelhasznaloID
		}
		out = append(out, s)
	}
	return out
}

func expectAutoLayout(t *testing.T, a app.Autok, want ...string) {
	t.Helper()
	if got := autoLayout(a); fmt.Sprint(got) != fmt.Sprint(want) {
		t.Fatalf("autok = %v, want %v", got, want)
	}
}

func helyNelkulIDs(a app.Autok) []string {
	out := []string{}
	for _, r := range a.HelyNelkul {
		out = append(out, r.FelhasznaloID)
	}
	return out
}

func autoDMs(h *apptest.Harness) []gatewayfake.DM {
	var out []gatewayfake.DM
	for _, dm := range h.Discord.DMs() {
		if dm.Auto != nil {
			out = append(out, dm)
		}
	}
	return out
}

func TestAResztvevoOffersAnAutoAndBecomesItsSofor(t *testing.T) {
	h, d := autokTrip(t)
	ctx := context.Background()

	auto := autoOffer(t, h, "bela", d.ID, 3)
	if auto.SoforID != "bela" || auto.Seats != 3 || auto.Note != "indulás 8:00, Keleti" {
		t.Fatalf("auto = %+v", auto)
	}
	a := autokOf(t, h, "cecil", d.ID)
	if len(a.Autok) != 1 || a.Autok[0].SoforName != "bela" || a.Autok[0].Szabad != 3 || a.Autok[0].CanEdit || a.Autok[0].CanDelete {
		t.Fatalf("Cecil sees %+v", a.Autok)
	}
	if a := autokOf(t, h, "bela", d.ID); a.SajatAutoID != auto.ID || !a.Autok[0].CanEdit || !a.Autok[0].CanDelete || a.CanOffer {
		t.Fatalf("Bela sees %+v", a)
	}

	// One Auto per Sofor.
	if _, err := h.App.OfferAuto(ctx, "bela", d.ID, app.AutoInput{Seats: 2}); !errors.Is(err, app.ErrAlreadySofor) {
		t.Fatalf("a second Auto: err = %v, want ErrAlreadySofor", err)
	}
	// Bad input.
	for _, bad := range []app.AutoInput{{Seats: 0}, {Seats: 9}, {Seats: -1}, {Seats: 2, Note: strings.Repeat("x", 101)}} {
		if _, err := h.App.OfferAuto(ctx, "cecil", d.ID, bad); !errors.Is(err, app.ErrInvalidInput) {
			t.Fatalf("OfferAuto(%+v): err = %v, want ErrInvalidInput", bad, err)
		}
	}
	// Only Resztvevok, and only once Veglegesitve.
	if _, err := h.App.OfferAuto(ctx, "emil", d.ID, app.AutoInput{Seats: 2}); !errors.Is(err, app.ErrNotResztvevo) {
		t.Fatalf("a non-Resztvevo: err = %v, want ErrNotResztvevo", err)
	}
	if _, err := h.App.OfferAuto(ctx, "dani", d.ID, app.AutoInput{Seats: 2}); !errors.Is(err, app.ErrNotFound) {
		t.Fatalf("a non-Tag: err = %v, want ErrNotFound", err)
	}
	reopen(t, h, "anna", d.ID)
	if _, err := h.App.OfferAuto(ctx, "cecil", d.ID, app.AutoInput{Seats: 2}); !errors.Is(err, app.ErrClaimingClosed) {
		t.Fatalf("offering in Egyeztetes: err = %v, want ErrClaimingClosed", err)
	}
	// Switched off, it is hidden.
	switchKiegeszito(t, h, "anna", d.ID, domain.Autok, false)
	if _, err := h.App.Autok(ctx, "bela", d.ID); !errors.Is(err, app.ErrKiegeszitoOff) {
		t.Fatalf("Autok while off: err = %v, want ErrKiegeszitoOff", err)
	}
}

func TestASoforHoldsNoSeatAndIsNotSajatUton(t *testing.T) {
	h, d := autokTrip(t)
	ctx := context.Background()
	cecilsAuto := autoOffer(t, h, "cecil", d.ID, 3)
	autoClaim(t, h, "bela", d.ID, cecilsAuto.ID)

	// Offering removes Bela's own seat.
	autoOffer(t, h, "bela", d.ID, 2)
	a := autokOf(t, h, "anna", d.ID)
	expectAutoLayout(t, a, "cecil:", "bela:")

	if err := h.App.ClaimUles(ctx, "bela", d.ID, cecilsAuto.ID, app.UtasInput{}); !errors.Is(err, app.ErrSoforCantRide) {
		t.Fatalf("a Sofor claiming a seat: err = %v, want ErrSoforCantRide", err)
	}
	if err := h.App.SetSajatUton(ctx, "bela", d.ID, app.SajatUtonInput{On: true}); !errors.Is(err, app.ErrSoforCantRide) {
		t.Fatalf("a Sofor saját úton: err = %v, want ErrSoforCantRide", err)
	}
	if err := h.App.MoveUtas(ctx, "anna", d.ID, cecilsAuto.ID, app.UtasInput{FelhasznaloID: "bela"}); !errors.Is(err, app.ErrSoforCantRide) {
		t.Fatalf("the Szervezo seating a Sofor: err = %v, want ErrSoforCantRide", err)
	}

	// Offering also clears the saját úton mark.
	if err := h.App.SetSajatUton(ctx, "dora", d.ID, app.SajatUtonInput{On: true}); err != nil {
		t.Fatalf("SetSajatUton: %v", err)
	}
	autoOffer(t, h, "dora", d.ID, 1)
	if a := autokOf(t, h, "dora", d.ID); a.SajatUtonVan || len(a.SajatUton) != 0 {
		t.Fatalf("Dora still saját úton after offering: %+v", a.SajatUton)
	}
}

func TestSeatsCantBeOverbooked(t *testing.T) {
	h, d := autokTrip(t)
	ctx := context.Background()
	auto := autoOffer(t, h, "bela", d.ID, 1)

	autoClaim(t, h, "cecil", d.ID, auto.ID)
	if err := h.App.ClaimUles(ctx, "dora", d.ID, auto.ID, app.UtasInput{}); !errors.Is(err, app.ErrAutoFull) {
		t.Fatalf("claiming a full Auto: err = %v, want ErrAutoFull", err)
	}
	// Claiming the seat you hold changes nothing.
	autoClaim(t, h, "cecil", d.ID, auto.ID)
	a := autokOf(t, h, "anna", d.ID)
	expectAutoLayout(t, a, "bela:cecil")
	if a.Autok[0].Szabad != 0 {
		t.Fatalf("szabad = %d, want 0", a.Autok[0].Szabad)
	}
}

func TestTwoPeopleRacingForTheLastSeatOnlyOneGetsIt(t *testing.T) {
	h, d := autokTrip(t)
	ctx := context.Background()
	auto := autoOffer(t, h, "bela", d.ID, 1)

	var wg sync.WaitGroup
	errs := make([]error, 3)
	for n, who := range []string{"cecil", "dora", "anna"} {
		wg.Add(1)
		go func() {
			defer wg.Done()
			errs[n] = h.App.ClaimUles(ctx, who, d.ID, auto.ID, app.UtasInput{})
		}()
	}
	wg.Wait()

	won := 0
	for _, err := range errs {
		switch {
		case err == nil:
			won++
		case !errors.Is(err, app.ErrAutoFull):
			t.Fatalf("unexpected error: %v", err)
		}
	}
	if won != 1 {
		t.Fatalf("%d claims won the last seat, want 1 (%v)", won, errs)
	}
	if a := autokOf(t, h, "anna", d.ID); len(a.Autok[0].Utasok) != 1 {
		t.Fatalf("utasok = %+v, want one", a.Autok[0].Utasok)
	}
}

func TestOneSeatPerPersonSoMovingMeansLeavingFirst(t *testing.T) {
	h, d := autokTrip(t)
	ctx := context.Background()
	belas := autoOffer(t, h, "bela", d.ID, 2)
	doras := autoOffer(t, h, "dora", d.ID, 2)

	autoClaim(t, h, "cecil", d.ID, belas.ID)
	if err := h.App.ClaimUles(ctx, "cecil", d.ID, doras.ID, app.UtasInput{}); !errors.Is(err, app.ErrAlreadySeated) {
		t.Fatalf("a second seat: err = %v, want ErrAlreadySeated", err)
	}
	if a := autokOf(t, h, "cecil", d.ID); a.SajatUlesAutoID != belas.ID {
		t.Fatalf("Cecil's seat = %q, want Bela's Auto", a.SajatUlesAutoID)
	}

	if err := h.App.ReleaseUles(ctx, "cecil", d.ID, app.UtasInput{}); err != nil {
		t.Fatalf("ReleaseUles: %v", err)
	}
	expectAutoLayout(t, autokOf(t, h, "anna", d.ID), "bela:", "dora:")
	autoClaim(t, h, "cecil", d.ID, doras.ID)
	expectAutoLayout(t, autokOf(t, h, "anna", d.ID), "bela:", "dora:cecil")

	// Leaving twice is fine.
	for range 2 {
		if err := h.App.ReleaseUles(ctx, "cecil", d.ID, app.UtasInput{}); err != nil {
			t.Fatalf("ReleaseUles: %v", err)
		}
	}
	// A seat in an unknown Auto, and one for a Vendeg before #13.
	if err := h.App.ClaimUles(ctx, "cecil", d.ID, "no-such-auto", app.UtasInput{}); !errors.Is(err, app.ErrNotFound) {
		t.Fatalf("an unknown Auto: err = %v, want ErrNotFound", err)
	}
	if err := h.App.ClaimUles(ctx, "cecil", d.ID, belas.ID, app.UtasInput{VendegID: "v1"}); !errors.Is(err, app.ErrNotFound) {
		t.Fatalf("a Vendeg before #13: err = %v, want ErrNotFound", err)
	}
	// Only Resztvevok claim.
	if err := h.App.ClaimUles(ctx, "emil", d.ID, belas.ID, app.UtasInput{}); !errors.Is(err, app.ErrNotResztvevo) {
		t.Fatalf("a non-Resztvevo: err = %v, want ErrNotResztvevo", err)
	}
	// Nobody claims for someone else.
	if err := h.App.ClaimUles(ctx, "cecil", d.ID, belas.ID, app.UtasInput{FelhasznaloID: "dora"}); !errors.Is(err, app.ErrNotPermitted) {
		t.Fatalf("claiming for someone else: err = %v, want ErrNotPermitted", err)
	}
}

func TestOnlyTheSzervezoMovesAndRemovesPeople(t *testing.T) {
	h, d := autokTrip(t)
	ctx := context.Background()
	belas := autoOffer(t, h, "bela", d.ID, 1)
	doras := autoOffer(t, h, "dora", d.ID, 2)
	autoClaim(t, h, "cecil", d.ID, belas.ID)

	// Anna moves Cecil, who has a seat elsewhere, in one step.
	if err := h.App.MoveUtas(ctx, "anna", d.ID, doras.ID, app.UtasInput{FelhasznaloID: "cecil"}); err != nil {
		t.Fatalf("MoveUtas: %v", err)
	}
	expectAutoLayout(t, autokOf(t, h, "anna", d.ID), "bela:", "dora:cecil")
	// Into a full Auto: refused, and Cecil keeps their seat.
	if err := h.App.MoveUtas(ctx, "anna", d.ID, belas.ID, app.UtasInput{FelhasznaloID: "anna"}); err != nil {
		t.Fatalf("seating Anna: %v", err)
	}
	if err := h.App.MoveUtas(ctx, "anna", d.ID, belas.ID, app.UtasInput{FelhasznaloID: "cecil"}); !errors.Is(err, app.ErrAutoFull) {
		t.Fatalf("moving into a full Auto: err = %v, want ErrAutoFull", err)
	}
	expectAutoLayout(t, autokOf(t, h, "anna", d.ID), "bela:anna", "dora:cecil")

	// Nobody else moves or removes people.
	if err := h.App.MoveUtas(ctx, "bela", d.ID, doras.ID, app.UtasInput{FelhasznaloID: "anna"}); !errors.Is(err, app.ErrNotPermitted) {
		t.Fatalf("a Sofor moving: err = %v, want ErrNotPermitted", err)
	}
	if err := h.App.MoveUtas(ctx, "cecil", d.ID, doras.ID, app.UtasInput{}); !errors.Is(err, app.ErrNotPermitted) {
		t.Fatalf("moving oneself: err = %v, want ErrNotPermitted", err)
	}
	if err := h.App.ReleaseUles(ctx, "bela", d.ID, app.UtasInput{FelhasznaloID: "cecil"}); !errors.Is(err, app.ErrNotPermitted) {
		t.Fatalf("removing someone: err = %v, want ErrNotPermitted", err)
	}

	// The Szervezo removes a passenger, and seats someone without a seat.
	if err := h.App.ReleaseUles(ctx, "anna", d.ID, app.UtasInput{FelhasznaloID: "cecil"}); err != nil {
		t.Fatalf("the Szervezo removing: %v", err)
	}
	if err := h.App.MoveUtas(ctx, "anna", d.ID, doras.ID, app.UtasInput{FelhasznaloID: "emil"}); !errors.Is(err, app.ErrNotResztvevo) {
		t.Fatalf("seating a non-Resztvevo: err = %v, want ErrNotResztvevo", err)
	}
	expectAutoLayout(t, autokOf(t, h, "anna", d.ID), "bela:anna", "dora:")
	if a := autokOf(t, h, "anna", d.ID); !a.IsSzervezo || autokOf(t, h, "bela", d.ID).IsSzervezo {
		t.Fatalf("IsSzervezo wrong")
	}
}

func TestSajatUtonMeansNoSeatIsNeeded(t *testing.T) {
	h, d := autokTrip(t)
	ctx := context.Background()
	belas := autoOffer(t, h, "bela", d.ID, 2)

	// Anna, Cecil and Dora are Resztvevok without a seat; Bela drives.
	if got := helyNelkulIDs(autokOf(t, h, "anna", d.ID)); fmt.Sprint(got) != "[anna cecil dora]" {
		t.Fatalf("without a seat = %v", got)
	}
	autoClaim(t, h, "cecil", d.ID, belas.ID)
	if err := h.App.SetSajatUton(ctx, "dora", d.ID, app.SajatUtonInput{On: true}); err != nil {
		t.Fatalf("SetSajatUton: %v", err)
	}
	a := autokOf(t, h, "anna", d.ID)
	if got := helyNelkulIDs(a); fmt.Sprint(got) != "[anna]" {
		t.Fatalf("without a seat = %v, want only Anna", got)
	}
	if len(a.SajatUton) != 1 || a.SajatUton[0].Holder.FelhasznaloID != "dora" {
		t.Fatalf("saját úton = %+v", a.SajatUton)
	}

	// Marking saját úton removes a seat; taking a seat removes the mark.
	if err := h.App.SetSajatUton(ctx, "cecil", d.ID, app.SajatUtonInput{On: true}); err != nil {
		t.Fatalf("SetSajatUton: %v", err)
	}
	expectAutoLayout(t, autokOf(t, h, "anna", d.ID), "bela:")
	autoClaim(t, h, "dora", d.ID, belas.ID)
	a = autokOf(t, h, "dora", d.ID)
	if a.SajatUtonVan || a.SajatUlesAutoID != belas.ID {
		t.Fatalf("Dora after taking a seat: %+v", a)
	}

	// Unmarking, and only Resztvevok may mark.
	if err := h.App.SetSajatUton(ctx, "cecil", d.ID, app.SajatUtonInput{On: false}); err != nil {
		t.Fatalf("unmarking: %v", err)
	}
	if got := helyNelkulIDs(autokOf(t, h, "anna", d.ID)); fmt.Sprint(got) != "[anna cecil]" {
		t.Fatalf("without a seat = %v", got)
	}
	if err := h.App.SetSajatUton(ctx, "emil", d.ID, app.SajatUtonInput{On: true}); !errors.Is(err, app.ErrNotResztvevo) {
		t.Fatalf("a non-Resztvevo: err = %v, want ErrNotResztvevo", err)
	}
}

func TestWhenTheSoforDropsOutTheAutoGoesAndEachPassengerGetsADM(t *testing.T) {
	h, d := autokTrip(t)
	belas := autoOffer(t, h, "bela", d.ID, 3)
	autoClaim(t, h, "cecil", d.ID, belas.ID)
	autoClaim(t, h, "dora", d.ID, belas.ID)

	answer(t, h, "bela", d, 0, domain.Nem)

	a := autokOf(t, h, "anna", d.ID)
	expectAutoLayout(t, a)
	if got := helyNelkulIDs(a); fmt.Sprint(got) != "[anna cecil dora]" {
		t.Fatalf("without a seat = %v, want the passengers seatless", got)
	}
	want := gateway.AutoDM{SoforName: "bela", Title: "Társasjáték", Link: esemenyURL(d.ID)}
	dms := autoDMs(h)
	if len(dms) != 2 {
		t.Fatalf("auto DMs = %+v, want one per passenger", dms)
	}
	for n, who := range []string{"discord-cecil", "discord-dora"} {
		if dms[n].DiscordID != who || *dms[n].Auto != want {
			t.Fatalf("DM %d = %+v to %s, want %+v to %s", n, dms[n].Auto, dms[n].DiscordID, want, who)
		}
	}
	// The Szervezo hears that the Auto was freed.
	drop := dropOutDMs(h)
	if len(drop) != 1 || drop[0].DropOut.Felszabadult != "1 autó (2 utas hely nélkül)" {
		t.Fatalf("drop-out DMs = %+v", drop)
	}
}

func TestAPassengerDroppingOutFreesTheirSeatAndTheirMark(t *testing.T) {
	h, d := autokTrip(t)
	belas := autoOffer(t, h, "bela", d.ID, 2)
	autoClaim(t, h, "cecil", d.ID, belas.ID)
	if err := h.App.SetSajatUton(context.Background(), "dora", d.ID, app.SajatUtonInput{On: true}); err != nil {
		t.Fatalf("SetSajatUton: %v", err)
	}

	answer(t, h, "cecil", d, 0, domain.Talan)
	answer(t, h, "dora", d, 0, domain.Nem)

	a := autokOf(t, h, "anna", d.ID)
	expectAutoLayout(t, a, "bela:")
	if len(a.SajatUton) != 0 {
		t.Fatalf("saját úton = %+v, want the mark cleared", a.SajatUton)
	}
	drop := dropOutDMs(h)
	if len(drop) != 2 || drop[0].DropOut.Felszabadult != "1 hely bela autójában" || drop[1].DropOut.Felszabadult != "" {
		t.Fatalf("drop-out DMs = %+v", drop)
	}
	if n := len(autoDMs(h)); n != 0 {
		t.Fatalf("%d auto DMs, want none for passengers leaving", n)
	}
	// Dora's mark is gone when she rejoins.
	answer(t, h, "dora", d, 0, domain.Igen)
	if a := autokOf(t, h, "dora", d.ID); a.SajatUtonVan {
		t.Fatalf("Dora is still saját úton")
	}
}

func TestADropOutWhileAutokIsOffChangesSilently(t *testing.T) {
	h, d := autokTrip(t)
	belas := autoOffer(t, h, "bela", d.ID, 2)
	autoClaim(t, h, "cecil", d.ID, belas.ID)
	switchKiegeszito(t, h, "anna", d.ID, domain.Autok, false)

	answer(t, h, "bela", d, 0, domain.Nem)

	if dms := autoDMs(h); len(dms) != 0 {
		t.Fatalf("auto DMs = %+v, want none while Autok is off", dms)
	}
	if dms := dropOutDMs(h); len(dms) != 1 || dms[0].DropOut.Felszabadult != "" {
		t.Fatalf("drop-out DMs = %+v, want one that mentions nothing freed", dms)
	}
	switchKiegeszito(t, h, "anna", d.ID, domain.Autok, true)
	expectAutoLayout(t, autokOf(t, h, "anna", d.ID))
}

func TestASoforLeavingTheCsoportTakesTheAutoWithThem(t *testing.T) {
	h, d := autokTrip(t)
	c, err := h.Store.FindEsemeny(context.Background(), d.ID)
	if err != nil {
		t.Fatalf("FindEsemeny: %v", err)
	}
	belas := autoOffer(t, h, "bela", d.ID, 2)
	autoClaim(t, h, "cecil", d.ID, belas.ID)

	if err := h.App.LeaveCsoport(context.Background(), "bela", c.CsoportID); err != nil {
		t.Fatalf("LeaveCsoport: %v", err)
	}

	expectAutoLayout(t, autokOf(t, h, "anna", d.ID))
	dms := autoDMs(h)
	if len(dms) != 1 || dms[0].DiscordID != "discord-cecil" || dms[0].Auto.SoforName != "bela" {
		t.Fatalf("auto DMs = %+v, want one to Cecil", dms)
	}
}

func TestReConfirmingRemovesTheAutoOfASoforWhoIsNoResztvevoAnyMore(t *testing.T) {
	h, d := autokTrip(t)
	ctx := context.Background()
	belas := autoOffer(t, h, "bela", d.ID, 2)
	doras := autoOffer(t, h, "dora", d.ID, 2)
	autoClaim(t, h, "cecil", d.ID, belas.ID)
	autoClaim(t, h, "anna", d.ID, doras.ID)

	// Claims are frozen while Egyeztetes is open again.
	reopen(t, h, "anna", d.ID)
	answer(t, h, "bela", d, 0, domain.Nem)
	answer(t, h, "bela", d, 1, domain.Igen)
	answer(t, h, "cecil", d, 1, domain.Igen)
	answer(t, h, "dora", d, 1, domain.Igen)
	answer(t, h, "anna", d, 1, domain.Igen)
	if err := h.App.ReleaseUles(ctx, "cecil", d.ID, app.UtasInput{}); !errors.Is(err, app.ErrClaimingClosed) {
		t.Fatalf("leaving while frozen: err = %v, want ErrClaimingClosed", err)
	}
	if got := autoLayout(autokOf(t, h, "anna", d.ID)); fmt.Sprint(got) != "[bela:cecil dora:anna]" {
		t.Fatalf("frozen autok = %v", got)
	}
	// Everyone but Bela is a Resztvevo of the second Idopont, as is Bela.
	// Confirm the first one again: Bela said nem to it.
	confirm(t, h, "anna", d, 0)

	expectAutoLayout(t, autokOf(t, h, "anna", d.ID), "dora:anna")
	dms := autoDMs(h)
	if len(dms) != 1 || dms[0].DiscordID != "discord-cecil" {
		t.Fatalf("auto DMs = %+v, want one to Cecil", dms)
	}
	if n := len(dropOutDMs(h)); n != 0 {
		t.Fatalf("%d drop-out DMs, want none for a re-confirmation", n)
	}
}

func TestDeletingAnAutoSendsEachPassengerADM(t *testing.T) {
	h, d := autokTrip(t)
	ctx := context.Background()
	belas := autoOffer(t, h, "bela", d.ID, 2)
	doras := autoOffer(t, h, "dora", d.ID, 1)
	autoClaim(t, h, "cecil", d.ID, belas.ID)
	autoClaim(t, h, "anna", d.ID, doras.ID)

	if err := h.App.DeleteAuto(ctx, "cecil", d.ID, belas.ID); !errors.Is(err, app.ErrNotPermitted) {
		t.Fatalf("a passenger deleting: err = %v, want ErrNotPermitted", err)
	}
	if err := h.App.DeleteAuto(ctx, "bela", d.ID, belas.ID); err != nil {
		t.Fatalf("the Sofor deleting: %v", err)
	}
	dms := autoDMs(h)
	want := gateway.AutoDM{SoforName: "bela", Title: "Társasjáték", Deleted: true, Link: esemenyURL(d.ID)}
	if len(dms) != 1 || dms[0].DiscordID != "discord-cecil" || *dms[0].Auto != want {
		t.Fatalf("auto DMs = %+v, want %+v to Cecil", dms, want)
	}
	// Bela may offer a new one.
	autoOffer(t, h, "bela", d.ID, 2)

	// The Szervezo deletes any Auto, even one with her as passenger.
	if err := h.App.DeleteAuto(ctx, "anna", d.ID, doras.ID); err != nil {
		t.Fatalf("the Szervezo deleting: %v", err)
	}
	if dms := autoDMs(h); len(dms) != 2 || dms[1].DiscordID != "discord-anna" {
		t.Fatalf("auto DMs = %+v, want a second one to Anna, a passenger", dms)
	}
	if err := h.App.DeleteAuto(ctx, "anna", d.ID, doras.ID); !errors.Is(err, app.ErrNotFound) {
		t.Fatalf("deleting it again: err = %v, want ErrNotFound", err)
	}
	expectAutoLayout(t, autokOf(t, h, "anna", d.ID), "bela:")
}

func TestEditingAnAutoKeepsItsSeatsAtOrAboveThePassengers(t *testing.T) {
	h, d := autokTrip(t)
	ctx := context.Background()
	belas := autoOffer(t, h, "bela", d.ID, 3)
	autoClaim(t, h, "cecil", d.ID, belas.ID)
	autoClaim(t, h, "dora", d.ID, belas.ID)

	if err := h.App.UpdateAuto(ctx, "bela", d.ID, belas.ID, app.AutoInput{Seats: 1}); !errors.Is(err, app.ErrAutoSeatsBelowUtasok) {
		t.Fatalf("seats below the passengers: err = %v, want ErrAutoSeatsBelowUtasok", err)
	}
	if err := h.App.UpdateAuto(ctx, "bela", d.ID, belas.ID, app.AutoInput{Seats: 2, Note: " 9:00, Nyugati "}); err != nil {
		t.Fatalf("UpdateAuto: %v", err)
	}
	a := autokOf(t, h, "bela", d.ID)
	if a.Autok[0].Seats != 2 || a.Autok[0].Note != "9:00, Nyugati" || a.Autok[0].Szabad != 0 {
		t.Fatalf("auto = %+v", a.Autok[0])
	}
	if err := h.App.UpdateAuto(ctx, "bela", d.ID, belas.ID, app.AutoInput{Seats: 9}); !errors.Is(err, app.ErrInvalidInput) {
		t.Fatalf("too many seats: err = %v, want ErrInvalidInput", err)
	}
	// Only the Sofor edits, not even the Szervezo.
	for _, who := range []string{"anna", "cecil"} {
		if err := h.App.UpdateAuto(ctx, who, d.ID, belas.ID, app.AutoInput{Seats: 4}); !errors.Is(err, app.ErrNotPermitted) {
			t.Fatalf("%s editing: err = %v, want ErrNotPermitted", who, err)
		}
	}
}

func TestAutoUtasaiGivesTheSoforAndPassengers(t *testing.T) {
	h, d := autokTrip(t)
	ctx := context.Background()
	belas := autoOffer(t, h, "bela", d.ID, 3)
	autoClaim(t, h, "dora", d.ID, belas.ID)
	autoClaim(t, h, "cecil", d.ID, belas.ID)

	u, err := h.App.AutoUtasai(ctx, "anna", d.ID, belas.ID)
	if err != nil {
		t.Fatalf("AutoUtasai: %v", err)
	}
	want := []domain.ClaimHolder{{FelhasznaloID: "bela"}, {FelhasznaloID: "cecil"}, {FelhasznaloID: "dora"}}
	if got := u.Holders(); fmt.Sprint(got) != fmt.Sprint(want) {
		t.Fatalf("holders = %v, want %v", got, want)
	}
	if _, err := h.App.AutoUtasai(ctx, "anna", d.ID, "no-such-auto"); !errors.Is(err, app.ErrNotFound) {
		t.Fatalf("an unknown Auto: err = %v, want ErrNotFound", err)
	}
	if _, err := h.App.AutoUtasai(ctx, "dani", d.ID, belas.ID); !errors.Is(err, app.ErrNotFound) {
		t.Fatalf("a non-Tag: err = %v, want ErrNotFound", err)
	}
}

func TestTheEsemenyMessageSummarisesAutok(t *testing.T) {
	h := apptest.New(t)
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

	switchKiegeszito(t, h, "anna", e.ID, domain.Autok, true)
	if got := lastSummary(); got != "" {
		t.Fatalf("summary with nothing set up = %q, want none", got)
	}
	answer(t, h, "anna", d, 0, domain.Igen)
	answer(t, h, "bela", d, 0, domain.Igen)
	answer(t, h, "cecil", d, 0, domain.Igen)
	confirm(t, h, "anna", d, 0)
	if got := lastSummary(); got != "" {
		t.Fatalf("summary with nothing set up = %q, want none", got)
	}

	belas := autoOffer(t, h, "bela", e.ID, 1)
	if got := lastSummary(); got != "🚗 2 főnek nincs helye" {
		t.Fatalf("summary = %q, want 2 without a seat", got)
	}
	autoClaim(t, h, "cecil", e.ID, belas.ID)
	if got := lastSummary(); got != "🚗 1 főnek nincs helye" {
		t.Fatalf("summary = %q, want 1 without a seat", got)
	}
	if err := h.App.SetSajatUton(context.Background(), "anna", e.ID, app.SajatUtonInput{On: true}); err != nil {
		t.Fatalf("SetSajatUton: %v", err)
	}
	if got := lastSummary(); got != "🚗 mindenkinek van helye" {
		t.Fatalf("summary = %q, want everyone placed", got)
	}
	// Only a mark, nobody driving: still "set up".
	if err := h.App.DeleteAuto(context.Background(), "bela", e.ID, belas.ID); err != nil {
		t.Fatalf("DeleteAuto: %v", err)
	}
	if got := lastSummary(); got != "🚗 2 főnek nincs helye" {
		t.Fatalf("summary = %q, want 2 without a seat", got)
	}
	switchKiegeszito(t, h, "anna", e.ID, domain.Autok, false)
	if got := lastSummary(); got != "" {
		t.Fatalf("summary once switched off = %q, want none", got)
	}
}
