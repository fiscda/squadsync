package app_test

import (
	"context"
	"errors"
	"fmt"
	"math/rand/v2"
	"strings"
	"sync"
	"testing"
	"time"

	"squadsync-be/internal/app"
	"squadsync-be/internal/apptest"
	"squadsync-be/internal/domain"
	"squadsync-be/internal/gateway"
)

// lezajlottTrip is tripWithKoltsegek once Lezajlott: Anna (the Szervezo),
// Bela and Cecil are the Resztvevok, Dani a Tag who said nem.
func lezajlottTrip(t *testing.T) (*apptest.Harness, app.EsemenyDetail) {
	t.Helper()
	h, d := tripWithKoltsegek(t)
	setLezajlott(t, h, d)
	return h, d
}

func elszamolasOf(t *testing.T, h *apptest.Harness, felhasznaloID, esemenyID string) *app.Elszamolas {
	t.Helper()
	return koltsegek(t, h, felhasznaloID, esemenyID).Elszamolas
}

func confirmKiadasok(t *testing.T, h *apptest.Harness, esemenyID string, who ...string) {
	t.Helper()
	for _, f := range who {
		if err := h.App.ConfirmKiadasokRecorded(context.Background(), f, esemenyID); err != nil {
			t.Fatalf("ConfirmKiadasokRecorded(%s): %v", f, err)
		}
	}
}

// names shows people as "name name ...".
func names(list []app.ElszamolasSzemely) string {
	parts := []string{}
	for _, p := range list {
		parts = append(parts, p.Name)
	}
	return strings.Join(parts, " ")
}

// utalasok shows the Utalasok as "from>to:amount ...".
func utalasok(e *app.Elszamolas) string {
	parts := []string{}
	for _, u := range e.Utalasok {
		parts = append(parts, fmt.Sprintf("%s>%s:%d", u.FromID, u.ToID, u.Amount))
	}
	return strings.Join(parts, " ")
}

// elszamolasDMs are the result DMs sent to one Discord account.
func elszamolasDMs(h *apptest.Harness, discordID string) []gateway.ElszamolasDM {
	var out []gateway.ElszamolasDM
	for _, dm := range h.Discord.DMsTo(discordID) {
		if dm.Elszamolas != nil {
			out = append(out, *dm.Elszamolas)
		}
	}
	return out
}

// expectBalanced checks that the Utalasok settle every balance exactly,
// between different people, with positive amounts and at most n-1 of them
// for n people with a non-zero balance.
func expectBalanced(t *testing.T, k app.Koltsegek) {
	t.Helper()
	if k.Elszamolas == nil {
		t.Fatalf("no elszamolas")
	}
	got := map[string]int{}
	for _, u := range k.Elszamolas.Utalasok {
		if u.Amount < 1 || u.FromID == u.ToID {
			t.Fatalf("bad utalas %+v", u.Utalas)
		}
		got[u.FromID] -= u.Amount
		got[u.ToID] += u.Amount
	}
	nonZero := 0
	for _, b := range k.Egyenlegek {
		if got[b.FelhasznaloID] != b.Netto {
			t.Fatalf("%s: utalasok give %d, balance is %d (utalasok %s, balances %s)",
				b.FelhasznaloID, got[b.FelhasznaloID], b.Netto, utalasok(k.Elszamolas), egyenlegek(k))
		}
		delete(got, b.FelhasznaloID)
		if b.Netto != 0 {
			nonZero++
		}
	}
	for id, v := range got {
		if v != 0 {
			t.Fatalf("%s gets %d without a balance", id, v)
		}
	}
	if nonZero > 0 && len(k.Elszamolas.Utalasok) > nonZero-1 {
		t.Fatalf("%d utalasok for %d people with a balance, want at most %d", len(k.Elszamolas.Utalasok), nonZero, nonZero-1)
	}
}

func TestConfirmingOpensAtLezajlottAndOnlyForResztvevok(t *testing.T) {
	h, d := tripWithKoltsegek(t)
	ctx := context.Background()
	addKiadas(t, h, "anna", d.ID, app.KiadasInput{Description: "Bor", Amount: 900})

	if e := elszamolasOf(t, h, "bela", d.ID); e == nil || e.Allapot != app.ElszamolasLezajlasElott || e.CanConfirm {
		t.Fatalf("elszamolas before Lezajlott = %+v, want lezajlas_elott without confirming", e)
	}
	if err := h.App.ConfirmKiadasokRecorded(ctx, "bela", d.ID); !errors.Is(err, app.ErrElszamolasNotOpen) {
		t.Fatalf("confirming in Veglegesitve: err = %v, want ErrElszamolasNotOpen", err)
	}

	setLezajlott(t, h, d)
	if err := h.App.ConfirmKiadasokRecorded(ctx, "dani", d.ID); !errors.Is(err, app.ErrElszamolasNotResztvevo) || !errors.Is(err, app.ErrNotResztvevo) {
		t.Fatalf("Dani (nem) confirming: err = %v, want ErrElszamolasNotResztvevo", err)
	}
	addFelhasznalo(t, h, "eve", "eve")
	if err := h.App.ConfirmKiadasokRecorded(ctx, "eve", d.ID); !errors.Is(err, app.ErrNotFound) {
		t.Fatalf("a non-Tag confirming: err = %v, want ErrNotFound", err)
	}
	if e := elszamolasOf(t, h, "dani", d.ID); e.CanConfirm || e.Allapot != app.ElszamolasMegerosites {
		t.Fatalf("Dani's view = %+v, want megerosites without confirming", e)
	}

	confirmKiadasok(t, h, d.ID, "bela", "bela") // twice changes nothing
	e := elszamolasOf(t, h, "bela", d.ID)
	if names(e.Megerositettek) != "bela" || names(e.Hianyzok) != "Anna cecil" || !e.Megerositette || e.CanConfirm || !e.CanUnconfirm || e.CanClose {
		t.Fatalf("after Bela confirmed = %+v", e)
	}
	if e := elszamolasOf(t, h, "anna", d.ID); !e.CanConfirm || !e.CanClose || e.Megerositette {
		t.Fatalf("Anna's view = %+v, want her to confirm or close", e)
	}
}

func TestNoElszamolasWithoutKiadasokOrForAnElmaradtEsemeny(t *testing.T) {
	ctx := context.Background()

	t.Run("no Kiadas", func(t *testing.T) {
		h, d := lezajlottTrip(t)
		if e := elszamolasOf(t, h, "anna", d.ID); e != nil {
			t.Fatalf("elszamolas without a Kiadas = %+v, want none", e)
		}
		if err := h.App.ConfirmKiadasokRecorded(ctx, "bela", d.ID); !errors.Is(err, app.ErrElszamolasNotOpen) {
			t.Fatalf("confirming: err = %v, want ErrElszamolasNotOpen", err)
		}
		if err := h.App.CloseElszamolas(ctx, "anna", d.ID); !errors.Is(err, app.ErrElszamolasNotOpen) {
			t.Fatalf("closing: err = %v, want ErrElszamolasNotOpen", err)
		}
		// Recorded after the trip, there is one.
		addKiadas(t, h, "bela", d.ID, app.KiadasInput{Description: "Pizza", Amount: 3000})
		if e := elszamolasOf(t, h, "anna", d.ID); e == nil || e.Allapot != app.ElszamolasMegerosites {
			t.Fatalf("elszamolas = %+v, want megerosites", e)
		}
	})

	t.Run("Elmaradt", func(t *testing.T) {
		h, d := tripWithKoltsegek(t)
		addKiadas(t, h, "bela", d.ID, app.KiadasInput{Description: "Előleg", Amount: 3000})
		cancelEsemeny(t, h, "anna", d.ID)
		if e := elszamolasOf(t, h, "anna", d.ID); e != nil {
			t.Fatalf("elszamolas once Elmaradt = %+v, want none", e)
		}
		if err := h.App.ConfirmKiadasokRecorded(ctx, "bela", d.ID); !errors.Is(err, app.ErrElszamolasNotOpen) {
			t.Fatalf("confirming once Elmaradt: err = %v, want ErrElszamolasNotOpen", err)
		}
		if err := h.App.CloseElszamolas(ctx, "anna", d.ID); !errors.Is(err, app.ErrElszamolasNotOpen) {
			t.Fatalf("closing once Elmaradt: err = %v, want ErrElszamolasNotOpen", err)
		}
	})
}

func TestThePayerOfATervezettKiadasCantConfirm(t *testing.T) {
	h, d := lezajlottTrip(t)
	ctx := context.Background()
	cabin := addKiadas(t, h, "anna", d.ID, app.KiadasInput{Description: "Szállás", Amount: 90000, PayerID: "bela", Allapot: domain.Tervezett})

	if err := h.App.ConfirmKiadasokRecorded(ctx, "bela", d.ID); !errors.Is(err, app.ErrElszamolasTervezett) {
		t.Fatalf("Bela confirming with a planned Kiadas: err = %v, want ErrElszamolasTervezett", err)
	}
	if e := elszamolasOf(t, h, "bela", d.ID); e.Megerositette || len(e.TervezettFizeto) != 1 || e.TervezettFizeto[0] != "Szállás" {
		t.Fatalf("Bela's view = %+v, want him blocked by Szállás", e)
	}
	// Others aren't blocked by it.
	confirmKiadasok(t, h, d.ID, "anna")

	if err := h.App.UpdateKiadas(ctx, "bela", d.ID, cabin.ID, app.KiadasInput{Amount: 87000, Allapot: domain.Kifizetve}); err != nil {
		t.Fatalf("marking it paid: %v", err)
	}
	confirmKiadasok(t, h, d.ID, "bela")

	// Making someone who confirmed the payer of a planned one takes their
	// confirmation back.
	addKiadas(t, h, "cecil", d.ID, app.KiadasInput{Description: "Hajó", Amount: 20000, PayerID: "anna", Allapot: domain.Tervezett})
	if e := elszamolasOf(t, h, "anna", d.ID); e.Megerositette || names(e.Megerositettek) != "bela" {
		t.Fatalf("after Anna became the payer of a planned Kiadas = %+v, want her unconfirmed", e)
	}
}

func TestAConfirmationCanBeTakenBackUntilTheCalculation(t *testing.T) {
	h, d := lezajlottTrip(t)
	ctx := context.Background()
	addKiadas(t, h, "anna", d.ID, app.KiadasInput{Description: "Bor", Amount: 900})

	confirmKiadasok(t, h, d.ID, "bela")
	if err := h.App.UnconfirmKiadasokRecorded(ctx, "bela", d.ID); err != nil {
		t.Fatalf("UnconfirmKiadasokRecorded: %v", err)
	}
	if e := elszamolasOf(t, h, "bela", d.ID); e.Megerositette || names(e.Hianyzok) != "Anna bela cecil" || !e.CanConfirm {
		t.Fatalf("after Mégsem = %+v", e)
	}
	// Anna and Cecil confirm: still missing Bela, so nothing is calculated.
	confirmKiadasok(t, h, d.ID, "anna", "cecil")
	if e := elszamolasOf(t, h, "bela", d.ID); e.Allapot != app.ElszamolasMegerosites {
		t.Fatalf("allapot = %s, want megerosites", e.Allapot)
	}
	confirmKiadasok(t, h, d.ID, "bela")
	if err := h.App.UnconfirmKiadasokRecorded(ctx, "bela", d.ID); !errors.Is(err, app.ErrElszamolasKiszamolva) {
		t.Fatalf("Mégsem after the calculation: err = %v, want ErrElszamolasKiszamolva", err)
	}
}

func TestTheLastConfirmationCalculatesOnceAndDMsEveryone(t *testing.T) {
	h, d := lezajlottTrip(t)
	ctx := context.Background()
	addKiadas(t, h, "anna", d.ID, app.KiadasInput{Description: "Bor", Amount: 900}) // 300 each
	addKiadas(t, h, "cecil", d.ID, app.KiadasInput{
		Description: "Benzin", Amount: 600, Split: &app.SplitInput{Holders: holders("anna", "cecil")},
	}) // 300 each

	confirmKiadasok(t, h, d.ID, "anna", "bela")
	if len(elszamolasDMs(h, "discord-anna")) != 0 {
		t.Fatalf("result DMs before the last confirmation")
	}
	confirmKiadasok(t, h, d.ID, "cecil")

	k := koltsegek(t, h, "bela", d.ID)
	if got := egyenlegek(k); got != "anna:900/600/300 bela:0/300/-300 cecil:600/600/0" {
		t.Fatalf("balances = %s", got)
	}
	e := k.Elszamolas
	if e.Allapot != app.ElszamolasKiszamolva || e.KiszamolvaAt == nil || e.LezartaName != "" || utalasok(e) != "bela>anna:300" {
		t.Fatalf("elszamolas = %+v (utalasok %s)", e, utalasok(e))
	}
	if names(e.Megerositettek) != "Anna bela cecil" || len(e.Hianyzok) != 0 || e.CanConfirm || e.CanUnconfirm || e.CanClose {
		t.Fatalf("elszamolas = %+v", e)
	}
	expectBalanced(t, k)

	// It runs once.
	if err := h.App.ConfirmKiadasokRecorded(ctx, "cecil", d.ID); !errors.Is(err, app.ErrElszamolasKiszamolva) {
		t.Fatalf("confirming again: err = %v, want ErrElszamolasKiszamolva", err)
	}
	if err := h.App.CloseElszamolas(ctx, "anna", d.ID); !errors.Is(err, app.ErrElszamolasKiszamolva) {
		t.Fatalf("closing it afterwards: err = %v, want ErrElszamolasKiszamolva", err)
	}

	// One DM each, with what they pay or get; Cecil is square. Dani took
	// no part and gets nothing.
	anna, bela, cecil := elszamolasDMs(h, "discord-anna"), elszamolasDMs(h, "discord-bela"), elszamolasDMs(h, "discord-cecil")
	if len(anna) != 1 || len(bela) != 1 || len(cecil) != 1 || len(elszamolasDMs(h, "discord-dani")) != 0 {
		t.Fatalf("result DMs: anna %d, bela %d, cecil %d, want one each", len(anna), len(bela), len(cecil))
	}
	u := e.Utalasok[0]
	if len(bela[0].Fizetendo) != 1 || bela[0].Fizetendo[0] != (gateway.ElszamolasUtalas{UtalasID: u.ID, Name: "Anna", Amount: 300}) || len(bela[0].Kapando) != 0 {
		t.Fatalf("Bela's DM = %+v", bela[0])
	}
	if len(anna[0].Kapando) != 1 || anna[0].Kapando[0].Name != "bela" || len(anna[0].Fizetendo) != 0 {
		t.Fatalf("Anna's DM = %+v", anna[0])
	}
	if len(cecil[0].Fizetendo) != 0 || len(cecil[0].Kapando) != 0 {
		t.Fatalf("Cecil's DM = %+v, want him square", cecil[0])
	}
	if bela[0].Title != "Társasjáték" || bela[0].CsoportName != "Péntek esték" || bela[0].Link != apptest.WebURL+"/esemenyek/"+d.ID || bela[0].EsemenyID != d.ID {
		t.Fatalf("Bela's DM = %+v", bela[0])
	}
}

func TestOnlyTheSzervezoClosesTheElszamolasEarly(t *testing.T) {
	h, d := lezajlottTrip(t)
	ctx := context.Background()
	addKiadas(t, h, "cecil", d.ID, app.KiadasInput{Description: "Bor", Amount: 900})
	addKiadas(t, h, "bela", d.ID, app.KiadasInput{Description: "Szállás", Amount: 60000, Allapot: domain.Tervezett})
	confirmKiadasok(t, h, d.ID, "cecil")

	for who, want := range map[string]error{"bela": app.ErrNotSzervezo, "dani": app.ErrNotSzervezo, "cecil": app.ErrNotSzervezo} {
		if err := h.App.CloseElszamolas(ctx, who, d.ID); !errors.Is(err, want) {
			t.Fatalf("%s closing: err = %v, want %v", who, err, want)
		}
	}
	if err := h.App.CloseElszamolas(ctx, "anna", d.ID); err != nil {
		t.Fatalf("CloseElszamolas: %v", err)
	}
	k := koltsegek(t, h, "anna", d.ID)
	// The planned Szállás doesn't count.
	if k.Elszamolas.Allapot != app.ElszamolasKiszamolva || k.Elszamolas.LezartaName != "Anna" || utalasok(k.Elszamolas) != "anna>cecil:300 bela>cecil:300" {
		t.Fatalf("elszamolas = %+v (utalasok %s)", k.Elszamolas, utalasok(k.Elszamolas))
	}
	if names(k.Elszamolas.Megerositettek) != "cecil" || len(k.Elszamolas.Hianyzok) != 0 || k.Tervezett != 1 {
		t.Fatalf("koltsegek = %+v", k)
	}
	expectBalanced(t, k)
	for _, f := range []string{"anna", "bela", "cecil"} {
		if n := len(elszamolasDMs(h, "discord-"+f)); n != 1 {
			t.Fatalf("%s got %d result DMs, want 1", f, n)
		}
	}
}

// crowdTrip is a Lezajlott Esemeny with Koltsegek on and the given
// Resztvevok (Anna, the Szervezo, and the others), every one of them
// saying igen.
func crowdTrip(t *testing.T, others ...string) (*apptest.Harness, app.EsemenyDetail) {
	t.Helper()
	h := apptest.New(t)
	c := csoportOf(t, h, others...)
	e := propose(t, h, "anna", c.ID, 1)
	switchKiegeszito(t, h, "anna", e.ID, domain.Koltsegek, true)
	d := detail(t, h, "anna", e.ID)
	for _, who := range append([]string{"anna"}, others...) {
		answer(t, h, who, d, 0, domain.Igen)
	}
	confirm(t, h, "anna", d, 0)
	setLezajlott(t, h, d)
	return h, d
}

func TestTheUtalasokSettleEveryBalanceExactly(t *testing.T) {
	ctx := context.Background()

	t.Run("everyone square", func(t *testing.T) {
		h, d := crowdTrip(t, "bela", "cecil")
		addKiadas(t, h, "anna", d.ID, app.KiadasInput{Description: "Bor", Amount: 900})
		addKiadas(t, h, "bela", d.ID, app.KiadasInput{Description: "Sör", Amount: 900})
		addKiadas(t, h, "cecil", d.ID, app.KiadasInput{Description: "Chips", Amount: 900})
		confirmKiadasok(t, h, d.ID, "anna", "bela", "cecil")
		k := koltsegek(t, h, "anna", d.ID)
		if len(k.Elszamolas.Utalasok) != 0 || k.Elszamolas.Allapot != app.ElszamolasElszamolva {
			t.Fatalf("elszamolas = %+v, want no utalas and elszamolva at once", k.Elszamolas)
		}
		expectBalanced(t, k)
		for _, f := range []string{"anna", "bela", "cecil"} {
			dms := elszamolasDMs(h, "discord-"+f)
			if len(dms) != 1 || len(dms[0].Fizetendo)+len(dms[0].Kapando) != 0 {
				t.Fatalf("%s's result DMs = %+v, want one saying they're square", f, dms)
			}
		}
	})

	t.Run("one creditor, many debtors", func(t *testing.T) {
		h, d := crowdTrip(t, "bela", "cecil", "dani", "emil")
		addKiadas(t, h, "anna", d.ID, app.KiadasInput{Description: "Szállás", Amount: 100001})
		confirmKiadasok(t, h, d.ID, "anna", "bela", "cecil", "dani", "emil")
		k := koltsegek(t, h, "anna", d.ID)
		// 100001 among five: Anna's own share is the extra forint's.
		if got := utalasok(k.Elszamolas); got != "bela>anna:20000 cecil>anna:20000 dani>anna:20000 emil>anna:20000" {
			t.Fatalf("utalasok = %s", got)
		}
		expectBalanced(t, k)
		if dms := elszamolasDMs(h, "discord-anna"); len(dms) != 1 || len(dms[0].Kapando) != 4 {
			t.Fatalf("Anna's result DMs = %+v, want one listing four", dms)
		}
	})

	t.Run("several creditors and debtors", func(t *testing.T) {
		h, d := crowdTrip(t, "bela", "cecil", "dani", "emil")
		custom := func(amounts ...int) *app.SplitInput {
			return &app.SplitInput{Holders: holders("anna", "bela", "cecil", "dani", "emil"), Custom: amounts}
		}
		addKiadas(t, h, "anna", d.ID, app.KiadasInput{Description: "Szállás", Amount: 50000})
		addKiadas(t, h, "bela", d.ID, app.KiadasInput{Description: "Kaja", Amount: 17000, Split: custom(1000, 5000, 3000, 4000, 4000)})
		addKiadas(t, h, "cecil", d.ID, app.KiadasInput{Description: "Benzin", Amount: 12001, Split: &app.SplitInput{Holders: holders("cecil", "dani", "emil")}})
		addKiadas(t, h, "emil", d.ID, app.KiadasInput{Description: "Jegyek", Amount: 3333})
		confirmKiadasok(t, h, d.ID, "anna", "bela", "cecil", "dani", "emil")
		expectBalanced(t, koltsegek(t, h, "anna", d.ID))
	})

	t.Run("random", func(t *testing.T) {
		people := []string{"anna", "bela", "cecil", "dani", "emil", "feri"}
		for seed := range uint64(8) {
			t.Run(fmt.Sprint(seed), func(t *testing.T) {
				r := rand.New(rand.NewPCG(seed, 22))
				h, d := crowdTrip(t, people[1:]...)
				for n := range 2 + r.IntN(6) {
					var split []string
					for _, p := range people {
						if r.IntN(3) > 0 {
							split = append(split, p)
						}
					}
					if len(split) == 0 {
						split = people[:1]
					}
					payer := people[r.IntN(len(people))]
					addKiadas(t, h, payer, d.ID, app.KiadasInput{
						Description: fmt.Sprintf("Kiadás %d", n), Amount: 1 + r.IntN(50000),
						Split: &app.SplitInput{Holders: holders(split...)},
					})
				}
				if err := h.App.CloseElszamolas(ctx, "anna", d.ID); err != nil {
					t.Fatalf("CloseElszamolas: %v", err)
				}
				expectBalanced(t, koltsegek(t, h, "anna", d.ID))
			})
		}
	})
}

func TestKiadasokAreLockedOnceCalculated(t *testing.T) {
	h, d := lezajlottTrip(t)
	ctx := context.Background()
	k := addKiadas(t, h, "anna", d.ID, app.KiadasInput{Description: "Bor", Amount: 900})
	confirmKiadasok(t, h, d.ID, "anna", "bela", "cecil")

	in := app.KiadasInput{Description: "Bor", Amount: 1200}
	if _, err := h.App.AddKiadas(ctx, "bela", d.ID, in); !errors.Is(err, app.ErrKiadasokLocked) {
		t.Fatalf("AddKiadas once calculated: err = %v, want ErrKiadasokLocked", err)
	}
	if err := h.App.UpdateKiadas(ctx, "anna", d.ID, k.ID, in); !errors.Is(err, app.ErrKiadasokLocked) {
		t.Fatalf("UpdateKiadas once calculated: err = %v, want ErrKiadasokLocked", err)
	}
	if err := h.App.DeleteKiadas(ctx, "anna", d.ID, k.ID); !errors.Is(err, app.ErrKiadasokLocked) {
		t.Fatalf("DeleteKiadas once calculated: err = %v, want ErrKiadasokLocked", err)
	}
	got := koltsegek(t, h, "anna", d.ID)
	if got.CanAdd || got.Kiadasok[0].CanEdit || got.Kiadasok[0].Amount != 900 {
		t.Fatalf("koltsegek once calculated = %+v", got)
	}
	// Still can't be switched off.
	if err := h.App.SetKiegeszito(ctx, "anna", d.ID, domain.Koltsegek, false); !errors.Is(err, app.ErrKiegeszitoInUse) {
		t.Fatalf("switching Koltsegek off: err = %v, want ErrKiegeszitoInUse", err)
	}
}

func TestTheCalculationAndAKiadasEditSerialise(t *testing.T) {
	for round := range 5 {
		t.Run(fmt.Sprint(round), func(t *testing.T) {
			h, d := lezajlottTrip(t)
			ctx := context.Background()
			k := addKiadas(t, h, "bela", d.ID, app.KiadasInput{Description: "Szállás", Amount: 900})
			confirmKiadasok(t, h, d.ID, "anna", "bela")

			var wg sync.WaitGroup
			errs := make(chan error, 20)
			wg.Add(1)
			go func() {
				defer wg.Done()
				errs <- h.App.ConfirmKiadasokRecorded(ctx, "cecil", d.ID)
			}()
			for n := range 10 {
				wg.Add(1)
				go func() {
					defer wg.Done()
					err := h.App.UpdateKiadas(ctx, "bela", d.ID, k.ID, app.KiadasInput{Description: "Szállás", Amount: 3000 + 300*n})
					if errors.Is(err, app.ErrKiadasokLocked) {
						err = nil
					}
					errs <- err
				}()
			}
			wg.Wait()
			close(errs)
			for err := range errs {
				if err != nil {
					t.Fatalf("concurrent write: %v", err)
				}
			}
			// Whatever got in before the calculation is in it; nothing
			// changed after it.
			got := koltsegek(t, h, "anna", d.ID)
			if got.Elszamolas.Allapot != app.ElszamolasKiszamolva {
				t.Fatalf("allapot = %s", got.Elszamolas.Allapot)
			}
			expectBalanced(t, got)
		})
	}
}

func TestMarkingUtalasokMegvoltUntilElszamolva(t *testing.T) {
	h, d := lezajlottTrip(t)
	ctx := context.Background()
	addKiadas(t, h, "anna", d.ID, app.KiadasInput{Description: "Bor", Amount: 900})
	confirmKiadasok(t, h, d.ID, "anna", "bela", "cecil")
	e := elszamolasOf(t, h, "anna", d.ID)
	if utalasok(e) != "bela>anna:300 cecil>anna:300" {
		t.Fatalf("utalasok = %s", utalasok(e))
	}
	belaU, cecilU := e.Utalasok[0].ID, e.Utalasok[1].ID
	lastSummary := func() string {
		t.Helper()
		edits := h.Discord.Edits()
		return edits[len(edits)-1].Message.KiegeszitoSummary
	}
	if got := lastSummary(); got != "🧾 1 Kiadás · 💸 0/2 utalás megvolt" {
		t.Fatalf("summary = %q", got)
	}

	// Cecil isn't a party of Bela's, Dani of neither.
	if err := h.App.SetUtalasMegvolt(ctx, "cecil", d.ID, belaU, true); !errors.Is(err, app.ErrNotPermitted) {
		t.Fatalf("Cecil marking Bela's: err = %v, want ErrNotPermitted", err)
	}
	if err := h.App.SetUtalasMegvolt(ctx, "dani", d.ID, cecilU, true); !errors.Is(err, app.ErrNotPermitted) {
		t.Fatalf("Dani marking: err = %v, want ErrNotPermitted", err)
	}
	if err := h.App.SetUtalasMegvolt(ctx, "anna", d.ID, "nope", true); !errors.Is(err, app.ErrNotFound) {
		t.Fatalf("an unknown utalas: err = %v, want ErrNotFound", err)
	}
	if v := elszamolasOf(t, h, "cecil", d.ID); v.Utalasok[0].CanMark || !v.Utalasok[1].CanMark {
		t.Fatalf("Cecil's CanMark = %v %v, want only his own", v.Utalasok[0].CanMark, v.Utalasok[1].CanMark)
	}

	// The payer marks it, the receiver takes it back, the Szervezo marks
	// it again.
	for _, step := range []struct {
		who     string
		megvolt bool
	}{{"bela", true}, {"anna", false}, {"bela", false}, {"anna", true}} {
		if err := h.App.SetUtalasMegvolt(ctx, step.who, d.ID, belaU, step.megvolt); err != nil {
			t.Fatalf("%s marking %v: %v", step.who, step.megvolt, err)
		}
	}
	v := elszamolasOf(t, h, "bela", d.ID)
	if u := v.Utalasok[0]; u.MegvoltAt == nil || u.MegvoltBy != "anna" || u.MegvoltByName != "Anna" || v.Allapot != app.ElszamolasKiszamolva {
		t.Fatalf("Bela's utalas = %+v, allapot %s", u.Utalas, v.Allapot)
	}
	if got := lastSummary(); got != "🧾 1 Kiadás · 💸 1/2 utalás megvolt" {
		t.Fatalf("summary = %q", got)
	}

	// The receiver marks the last one: Elszamolva, and final.
	if err := h.App.SetUtalasMegvolt(ctx, "anna", d.ID, cecilU, true); err != nil {
		t.Fatalf("SetUtalasMegvolt: %v", err)
	}
	v = elszamolasOf(t, h, "anna", d.ID)
	if v.Allapot != app.ElszamolasElszamolva || v.Utalasok[0].CanMark || v.CanReopen {
		t.Fatalf("elszamolas = %+v, want elszamolva with nothing to change", v)
	}
	if got := lastSummary(); got != "🧾 1 Kiadás · Elszámolva ✓" {
		t.Fatalf("summary = %q", got)
	}
	if err := h.App.SetUtalasMegvolt(ctx, "cecil", d.ID, cecilU, false); !errors.Is(err, app.ErrElszamolva) {
		t.Fatalf("taking one back once elszamolva: err = %v, want ErrElszamolva", err)
	}
	if err := h.App.SetUtalasMegvolt(ctx, "cecil", d.ID, cecilU, true); err != nil {
		t.Fatalf("marking again once elszamolva: %v", err)
	}
}

func TestReopeningOnlyWhileNoUtalasIsMegvolt(t *testing.T) {
	h, d := lezajlottTrip(t)
	ctx := context.Background()
	k := addKiadas(t, h, "anna", d.ID, app.KiadasInput{Description: "Bor", Amount: 900})

	if err := h.App.ReopenElszamolas(ctx, "anna", d.ID); !errors.Is(err, app.ErrElszamolasNotKiszamolva) {
		t.Fatalf("reopening before the calculation: err = %v, want ErrElszamolasNotKiszamolva", err)
	}
	confirmKiadasok(t, h, d.ID, "anna", "bela", "cecil")
	if err := h.App.ReopenElszamolas(ctx, "bela", d.ID); !errors.Is(err, app.ErrNotSzervezo) {
		t.Fatalf("Bela reopening: err = %v, want ErrNotSzervezo", err)
	}
	if e := elszamolasOf(t, h, "anna", d.ID); !e.CanReopen {
		t.Fatalf("Anna can't reopen: %+v", e)
	}
	if err := h.App.ReopenElszamolas(ctx, "anna", d.ID); err != nil {
		t.Fatalf("ReopenElszamolas: %v", err)
	}
	e := elszamolasOf(t, h, "bela", d.ID)
	if e.Allapot != app.ElszamolasMegerosites || len(e.Utalasok) != 0 || len(e.Megerositettek) != 0 || names(e.Hianyzok) != "Anna bela cecil" {
		t.Fatalf("after reopening = %+v, want everyone to confirm again", e)
	}
	// The Kiadasok unlock.
	if err := h.App.UpdateKiadas(ctx, "anna", d.ID, k.ID, app.KiadasInput{Description: "Bor", Amount: 1200}); err != nil {
		t.Fatalf("editing once reopened: %v", err)
	}
	confirmKiadasok(t, h, d.ID, "anna", "bela", "cecil")
	e = elszamolasOf(t, h, "anna", d.ID)
	if utalasok(e) != "bela>anna:400 cecil>anna:400" {
		t.Fatalf("recalculated utalasok = %s", utalasok(e))
	}
	if n := len(elszamolasDMs(h, "discord-bela")); n != 2 {
		t.Fatalf("Bela got %d result DMs, want one per calculation", n)
	}

	if err := h.App.SetUtalasMegvolt(ctx, "bela", d.ID, e.Utalasok[0].ID, true); err != nil {
		t.Fatalf("SetUtalasMegvolt: %v", err)
	}
	if err := h.App.ReopenElszamolas(ctx, "anna", d.ID); !errors.Is(err, app.ErrElszamolasMegvolt) {
		t.Fatalf("reopening after a megvolt: err = %v, want ErrElszamolasMegvolt", err)
	}
	if e := elszamolasOf(t, h, "anna", d.ID); e.CanReopen || e.Allapot != app.ElszamolasKiszamolva {
		t.Fatalf("elszamolas = %+v", e)
	}
}

// elszamolasEmlekeztetok are the discord ids that got the reminder to
// confirm, in order.
func elszamolasEmlekeztetok(h *apptest.Harness) []string {
	var out []string
	for _, dm := range h.Discord.DMs() {
		if dm.ElszamolasEmlekezteto != nil {
			out = append(out, dm.DiscordID)
		}
	}
	return out
}

func TestTheUnconfirmedGetOneReminderThreeDaysAfterLezajlott(t *testing.T) {
	h, d := tripWithKoltsegek(t)
	runAt(t, h, firstEnd) // Lezajlott
	if got := allapotOf(t, h, d.ID); got != domain.Lezajlott {
		t.Fatalf("allapot = %s", got)
	}
	addKiadas(t, h, "anna", d.ID, app.KiadasInput{Description: "Bor", Amount: 900})
	confirmKiadasok(t, h, d.ID, "bela")

	runAt(t, h, firstEnd.Add(72*time.Hour-time.Minute))
	expectLines(t, "reminders before 3 days", elszamolasEmlekeztetok(h))
	runAt(t, h, firstEnd.Add(72*time.Hour))
	expectLines(t, "reminders", elszamolasEmlekeztetok(h), "discord-anna", "discord-cecil")
	for _, dm := range h.Discord.DMs() {
		if r := dm.ElszamolasEmlekezteto; r != nil && (r.Title != "Társasjáték" || r.CsoportName != "Péntek esték" || r.Link != apptest.WebURL+"/esemenyek/"+d.ID) {
			t.Fatalf("reminder = %+v", r)
		}
	}
	runAt(t, h, firstEnd.Add(10*24*time.Hour))
	expectLines(t, "reminders later", elszamolasEmlekeztetok(h), "discord-anna", "discord-cecil")
}

func TestNoReminderOnceCalculatedOrWithoutKiadasok(t *testing.T) {
	t.Run("calculated", func(t *testing.T) {
		h, d := tripWithKoltsegek(t)
		runAt(t, h, firstEnd)
		addKiadas(t, h, "anna", d.ID, app.KiadasInput{Description: "Bor", Amount: 900})
		confirmKiadasok(t, h, d.ID, "bela")
		if err := h.App.CloseElszamolas(context.Background(), "anna", d.ID); err != nil {
			t.Fatalf("CloseElszamolas: %v", err)
		}
		runAt(t, h, firstEnd.Add(72*time.Hour))
		expectLines(t, "reminders", elszamolasEmlekeztetok(h))
	})

	t.Run("no Kiadas", func(t *testing.T) {
		h, _ := tripWithKoltsegek(t)
		runAt(t, h, firstEnd)
		runAt(t, h, firstEnd.Add(72*time.Hour))
		expectLines(t, "reminders", elszamolasEmlekeztetok(h))
	})

	t.Run("Koltsegek switched on after the trip", func(t *testing.T) {
		h := apptest.New(t)
		c := csoportOf(t, h, "bela")
		e := propose(t, h, "anna", c.ID, 1)
		d := detail(t, h, "anna", e.ID)
		answer(t, h, "anna", d, 0, domain.Igen)
		answer(t, h, "bela", d, 0, domain.Igen)
		confirm(t, h, "anna", d, 0)
		runAt(t, h, firstEnd)
		switchKiegeszito(t, h, "anna", d.ID, domain.Koltsegek, true)
		addKiadas(t, h, "bela", d.ID, app.KiadasInput{Description: "Pizza", Amount: 4000})
		runAt(t, h, firstEnd.Add(72*time.Hour))
		expectLines(t, "reminders", elszamolasEmlekeztetok(h), "discord-anna", "discord-bela")
	})
}
