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

// tripWithKoltsegek has Anna propose an Esemeny in a Csoport with Bela,
// Cecil and Dani and switch Koltsegek on. Anna, Bela and Cecil say igen
// and Dani nem, and Anna confirms: the Resztvevok are anna, bela, cecil.
func tripWithKoltsegek(t *testing.T) (*apptest.Harness, app.EsemenyDetail) {
	t.Helper()
	h := apptest.New(t)
	c := csoportOf(t, h, "bela", "cecil", "dani")
	e := propose(t, h, "anna", c.ID, 2)
	switchKiegeszito(t, h, "anna", e.ID, domain.Koltsegek, true)
	d := detail(t, h, "anna", e.ID)
	for _, who := range []string{"anna", "bela", "cecil"} {
		answer(t, h, who, d, 0, domain.Igen)
	}
	answer(t, h, "dani", d, 0, domain.Nem)
	confirm(t, h, "anna", d, 0)
	return h, d
}

func addKiadas(t *testing.T, h *apptest.Harness, felhasznaloID, esemenyID string, in app.KiadasInput) domain.Kiadas {
	t.Helper()
	k, err := h.App.AddKiadas(context.Background(), felhasznaloID, esemenyID, in)
	if err != nil {
		t.Fatalf("AddKiadas(%s, %+v): %v", felhasznaloID, in, err)
	}
	return k
}

func koltsegek(t *testing.T, h *apptest.Harness, felhasznaloID, esemenyID string) app.Koltsegek {
	t.Helper()
	k, err := h.App.Koltsegek(context.Background(), felhasznaloID, esemenyID)
	if err != nil {
		t.Fatalf("Koltsegek(%s): %v", felhasznaloID, err)
	}
	return k
}

func kiadasAllas(t *testing.T, k app.Koltsegek, id string) app.KiadasAllas {
	t.Helper()
	for _, allas := range k.Kiadasok {
		if allas.ID == id {
			return allas
		}
	}
	t.Fatalf("kiadas %s not in %+v", id, k.Kiadasok)
	return app.KiadasAllas{}
}

// reszek shows a Kiadas's split as "holder=amount ...".
func reszek(k app.KiadasAllas) string {
	parts := []string{}
	for _, r := range k.Reszek {
		parts = append(parts, fmt.Sprintf("%s=%d", r.Holder.FelhasznaloID, r.Amount))
	}
	return strings.Join(parts, " ")
}

// egyenlegek shows the balances as "name:paid/share/net ...".
func egyenlegek(k app.Koltsegek) string {
	parts := []string{}
	for _, e := range k.Egyenlegek {
		parts = append(parts, fmt.Sprintf("%s:%d/%d/%d", e.FelhasznaloID, e.Fizetett, e.Resz, e.Netto))
	}
	return strings.Join(parts, " ")
}

func holders(ids ...string) []domain.ClaimHolder {
	out := []domain.ClaimHolder{}
	for _, id := range ids {
		out = append(out, domain.ClaimHolder{FelhasznaloID: id})
	}
	return out
}

func setLezajlott(t *testing.T, h *apptest.Harness, d app.EsemenyDetail) {
	t.Helper()
	// Lezajlott is set by a timed job (#9); set it directly here.
	if err := h.Store.UpdateEsemenyAllapot(context.Background(), d.ID, domain.Lezajlott, d.Idopontok[0].ID); err != nil {
		t.Fatalf("UpdateEsemenyAllapot: %v", err)
	}
}

func TestAnEqualSplitRoundsInWholeForintsInIDOrder(t *testing.T) {
	h, d := tripWithKoltsegek(t)
	// The default split is every Resztvevo, equally; the extra forint goes
	// to the first in id order, whoever records it.
	k := addKiadas(t, h, "cecil", d.ID, app.KiadasInput{Description: "Bor", Amount: 1000})
	allas := kiadasAllas(t, koltsegek(t, h, "anna", d.ID), k.ID)
	if got := reszek(allas); got != "anna=334 bela=333 cecil=333" {
		t.Fatalf("split = %s, want anna=334 bela=333 cecil=333", got)
	}
	if !allas.Egyenlo || allas.Allapot != domain.Kifizetve || allas.PayerID != "cecil" || allas.CreatedBy != "cecil" {
		t.Fatalf("kiadas = %+v, want an equal, paid Kiadas by Cecil", allas.Kiadas)
	}

	// Two forints over, listed in any order: the first two in id order.
	k2 := addKiadas(t, h, "bela", d.ID, app.KiadasInput{
		Description: "Kaja", Amount: 1001, Split: &app.SplitInput{Holders: holders("cecil", "bela", "anna")},
	})
	if got := reszek(kiadasAllas(t, koltsegek(t, h, "anna", d.ID), k2.ID)); got != "anna=334 bela=334 cecil=333" {
		t.Fatalf("split = %s", got)
	}

	// Changing the amount recomputes it.
	if err := h.App.UpdateKiadas(context.Background(), "bela", d.ID, k2.ID, app.KiadasInput{Description: "Kaja", Amount: 10}); err != nil {
		t.Fatalf("UpdateKiadas: %v", err)
	}
	if got := reszek(kiadasAllas(t, koltsegek(t, h, "anna", d.ID), k2.ID)); got != "anna=4 bela=3 cecil=3" {
		t.Fatalf("split after the amount changed = %s", got)
	}
	// So does changing the split.
	if err := h.App.UpdateKiadas(context.Background(), "bela", d.ID, k2.ID, app.KiadasInput{
		Description: "Kaja", Amount: 10, Split: &app.SplitInput{Holders: holders("bela", "cecil")},
	}); err != nil {
		t.Fatalf("UpdateKiadas: %v", err)
	}
	if got := reszek(kiadasAllas(t, koltsegek(t, h, "anna", d.ID), k2.ID)); got != "bela=5 cecil=5" {
		t.Fatalf("split after it changed = %s", got)
	}
}

func TestCustomAmountsMustAddUpToTheTotal(t *testing.T) {
	h, d := tripWithKoltsegek(t)
	ctx := context.Background()
	split := func(custom ...int) *app.SplitInput {
		return &app.SplitInput{Holders: holders("bela", "anna"), Custom: custom}
	}

	if _, err := h.App.AddKiadas(ctx, "anna", d.ID, app.KiadasInput{Description: "Szállás", Amount: 1000, Split: split(600, 300)}); !errors.Is(err, app.ErrKiadasSplitSum) {
		t.Fatalf("custom amounts short of the total: err = %v, want ErrKiadasSplitSum", err)
	}
	if _, err := h.App.AddKiadas(ctx, "anna", d.ID, app.KiadasInput{Description: "Szállás", Amount: 1000, Split: split(600)}); !errors.Is(err, app.ErrInvalidInput) {
		t.Fatalf("too few custom amounts: err = %v, want ErrInvalidInput", err)
	}
	if _, err := h.App.AddKiadas(ctx, "anna", d.ID, app.KiadasInput{Description: "Szállás", Amount: 1000, Split: split(1100, -100)}); !errors.Is(err, app.ErrInvalidInput) {
		t.Fatalf("a negative custom amount: err = %v, want ErrInvalidInput", err)
	}
	k := addKiadas(t, h, "anna", d.ID, app.KiadasInput{Description: "Szállás", Amount: 1000, Split: split(700, 300)})
	allas := kiadasAllas(t, koltsegek(t, h, "anna", d.ID), k.ID)
	if got := reszek(allas); got != "anna=300 bela=700" || allas.Egyenlo {
		t.Fatalf("split = %s (equal %v), want anna=300 bela=700 custom", got, allas.Egyenlo)
	}

	// Keeping a custom split, a new amount must still add up.
	if err := h.App.UpdateKiadas(ctx, "anna", d.ID, k.ID, app.KiadasInput{Description: "Szállás", Amount: 1200}); !errors.Is(err, app.ErrKiadasSplitSum) {
		t.Fatalf("a new amount with the old custom split: err = %v, want ErrKiadasSplitSum", err)
	}
	if err := h.App.UpdateKiadas(ctx, "anna", d.ID, k.ID, app.KiadasInput{Description: "Szállás", Amount: 1200, Split: split(800, 400)}); err != nil {
		t.Fatalf("UpdateKiadas: %v", err)
	}
	if got := reszek(kiadasAllas(t, koltsegek(t, h, "anna", d.ID), k.ID)); got != "anna=400 bela=800" {
		t.Fatalf("split = %s", got)
	}
}

func TestBadKiadasInputIsRefused(t *testing.T) {
	h, d := tripWithKoltsegek(t)
	ctx := context.Background()
	for _, in := range []app.KiadasInput{
		{Description: " ", Amount: 100},
		{Description: strings.Repeat("x", 201), Amount: 100},
		{Description: "x", Amount: 0},
		{Description: "x", Amount: app.MaxKiadasAmount + 1},
		{Description: "x", Amount: 100, Allapot: "elfelejtve"},
		{Description: "x", Amount: 100, Split: &app.SplitInput{Holders: holders()}},
		{Description: "x", Amount: 100, Split: &app.SplitInput{Holders: holders("anna", "anna")}},
	} {
		if _, err := h.App.AddKiadas(ctx, "anna", d.ID, in); !errors.Is(err, app.ErrInvalidInput) {
			t.Errorf("AddKiadas(%+v): err = %v, want ErrInvalidInput", in, err)
		}
	}
	if _, err := h.App.AddKiadas(ctx, "anna", d.ID, app.KiadasInput{Description: "x", Amount: 100, PayerID: "dani"}); !errors.Is(err, app.ErrNotResztvevo) {
		t.Fatalf("a payer who isn't a Resztvevo: err = %v, want ErrNotResztvevo", err)
	}
	if _, err := h.App.AddKiadas(ctx, "anna", d.ID, app.KiadasInput{Description: "x", Amount: 100, Split: &app.SplitInput{Holders: holders("anna", "dani")}}); !errors.Is(err, app.ErrNotResztvevo) {
		t.Fatalf("splitting with a non-Resztvevo: err = %v, want ErrNotResztvevo", err)
	}
	vendeg := []domain.ClaimHolder{{VendegID: "v1"}}
	if _, err := h.App.AddKiadas(ctx, "anna", d.ID, app.KiadasInput{Description: "x", Amount: 100, Split: &app.SplitInput{Holders: vendeg}}); !errors.Is(err, app.ErrNotResztvevo) {
		t.Fatalf("splitting with an unknown Vendeg: err = %v, want ErrNotResztvevo", err)
	}
	if k := koltsegek(t, h, "anna", d.ID); len(k.Kiadasok) != 0 {
		t.Fatalf("kiadasok = %+v, want none", k.Kiadasok)
	}
}

func TestWhoMayAddEditAndDeleteKiadasok(t *testing.T) {
	h, d := tripWithKoltsegek(t)
	ctx := context.Background()

	// Only Resztvevok add; any of them may name another Resztvevo payer.
	if _, err := h.App.AddKiadas(ctx, "dani", d.ID, app.KiadasInput{Description: "x", Amount: 100}); !errors.Is(err, app.ErrNotResztvevo) {
		t.Fatalf("a non-Resztvevo adding: err = %v, want ErrNotResztvevo", err)
	}
	if k := koltsegek(t, h, "dani", d.ID); k.CanAdd {
		t.Fatalf("Dani sees CanAdd")
	}
	k := addKiadas(t, h, "cecil", d.ID, app.KiadasInput{Description: "Benzin", Amount: 9000, PayerID: "bela"})
	if k.PayerID != "bela" || k.CreatedBy != "cecil" {
		t.Fatalf("kiadas = %+v, want Bela paying, recorded by Cecil", k)
	}

	edit := func(who string) error {
		return h.App.UpdateKiadas(ctx, who, d.ID, k.ID, app.KiadasInput{Description: "Benzin (" + who + ")", Amount: 9000})
	}
	// The payer, the creator and the Szervezo may edit it.
	for _, who := range []string{"bela", "cecil", "anna"} {
		if err := edit(who); err != nil {
			t.Fatalf("%s editing: %v", who, err)
		}
		if !kiadasAllas(t, koltsegek(t, h, who, d.ID), k.ID).CanEdit {
			t.Fatalf("%s doesn't see CanEdit", who)
		}
	}
	// Someone else may not.
	other := addKiadas(t, h, "anna", d.ID, app.KiadasInput{Description: "Szállás", Amount: 3000})
	if err := h.App.UpdateKiadas(ctx, "bela", d.ID, other.ID, app.KiadasInput{Description: "Szállás", Amount: 1}); !errors.Is(err, app.ErrNotPermitted) {
		t.Fatalf("Bela editing Anna's: err = %v, want ErrNotPermitted", err)
	}
	if err := h.App.DeleteKiadas(ctx, "cecil", d.ID, other.ID); !errors.Is(err, app.ErrNotPermitted) {
		t.Fatalf("Cecil deleting Anna's: err = %v, want ErrNotPermitted", err)
	}
	if kiadasAllas(t, koltsegek(t, h, "bela", d.ID), other.ID).CanEdit {
		t.Fatalf("Bela sees CanEdit on Anna's Kiadas")
	}
	if err := h.App.DeleteKiadas(ctx, "dani", d.ID, k.ID); !errors.Is(err, app.ErrNotPermitted) {
		t.Fatalf("someone unrelated deleting: err = %v, want ErrNotPermitted", err)
	}
	if err := h.App.UpdateKiadas(ctx, "anna", d.ID, "nope", app.KiadasInput{Description: "x", Amount: 1}); !errors.Is(err, app.ErrNotFound) {
		t.Fatalf("editing an unknown Kiadas: err = %v, want ErrNotFound", err)
	}

	// The payer and the creator may delete it, the Szervezo too.
	if err := h.App.DeleteKiadas(ctx, "bela", d.ID, k.ID); err != nil {
		t.Fatalf("the payer deleting: %v", err)
	}
	if err := h.App.DeleteKiadas(ctx, "anna", d.ID, k.ID); !errors.Is(err, app.ErrNotFound) {
		t.Fatalf("deleting it again: err = %v, want ErrNotFound", err)
	}
	k3 := addKiadas(t, h, "cecil", d.ID, app.KiadasInput{Description: "Jég", Amount: 500, PayerID: "bela"})
	if err := h.App.DeleteKiadas(ctx, "cecil", d.ID, k3.ID); err != nil {
		t.Fatalf("the creator deleting: %v", err)
	}
	k4 := addKiadas(t, h, "cecil", d.ID, app.KiadasInput{Description: "Jég", Amount: 500})
	if err := h.App.DeleteKiadas(ctx, "anna", d.ID, k4.ID); err != nil {
		t.Fatalf("the Szervezo deleting: %v", err)
	}
	if got := koltsegek(t, h, "anna", d.ID).Kiadasok; len(got) != 1 || got[0].ID != other.ID {
		t.Fatalf("kiadasok = %+v, want only Anna's", got)
	}
}

func TestKiadasokAreRecordedOnlyInVeglegesitveAndLezajlott(t *testing.T) {
	ctx := context.Background()
	in := app.KiadasInput{Description: "Bor", Amount: 1000}

	t.Run("Egyeztetes", func(t *testing.T) {
		h := apptest.New(t)
		c := csoportOf(t, h, "bela")
		e := propose(t, h, "anna", c.ID, 1)
		switchKiegeszito(t, h, "anna", e.ID, domain.Koltsegek, true)
		if _, err := h.App.AddKiadas(ctx, "anna", e.ID, in); !errors.Is(err, app.ErrKoltsegekClosed) {
			t.Fatalf("AddKiadas in Egyeztetes: err = %v, want ErrKoltsegekClosed", err)
		}
		if k := koltsegek(t, h, "anna", e.ID); k.CanAdd || len(k.Kiadasok) != 0 {
			t.Fatalf("koltsegek in Egyeztetes = %+v", k)
		}
	})

	t.Run("Veglegesitve then reopened", func(t *testing.T) {
		h, d := tripWithKoltsegek(t)
		k := addKiadas(t, h, "bela", d.ID, in)
		reopen(t, h, "anna", d.ID)
		if err := h.App.UpdateKiadas(ctx, "bela", d.ID, k.ID, in); !errors.Is(err, app.ErrKoltsegekClosed) {
			t.Fatalf("UpdateKiadas once reopened: err = %v, want ErrKoltsegekClosed", err)
		}
		// Reopening never touches the Kiadasok.
		if got := koltsegek(t, h, "bela", d.ID); len(got.Kiadasok) != 1 || got.Kiadasok[0].CanEdit {
			t.Fatalf("koltsegek once reopened = %+v", got)
		}
	})

	t.Run("Lezajlott", func(t *testing.T) {
		h, d := tripWithKoltsegek(t)
		setLezajlott(t, h, d)
		k := addKiadas(t, h, "bela", d.ID, in)
		if err := h.App.UpdateKiadas(ctx, "bela", d.ID, k.ID, app.KiadasInput{Description: "Bor", Amount: 1200}); err != nil {
			t.Fatalf("UpdateKiadas once Lezajlott: %v", err)
		}
		if _, err := h.App.AddKiadas(ctx, "dani", d.ID, in); !errors.Is(err, app.ErrNotResztvevo) {
			t.Fatalf("a non-Resztvevo once Lezajlott: err = %v, want ErrNotResztvevo", err)
		}
		if !koltsegek(t, h, "cecil", d.ID).CanAdd {
			t.Fatalf("a Resztvevo can't add once Lezajlott")
		}
		if err := h.App.DeleteKiadas(ctx, "bela", d.ID, k.ID); err != nil {
			t.Fatalf("DeleteKiadas once Lezajlott: %v", err)
		}
	})

	t.Run("Elmaradt", func(t *testing.T) {
		h, d := tripWithKoltsegek(t)
		k := addKiadas(t, h, "bela", d.ID, in)
		cancelEsemeny(t, h, "anna", d.ID)
		if _, err := h.App.AddKiadas(ctx, "bela", d.ID, in); !errors.Is(err, app.ErrKoltsegekClosed) {
			t.Fatalf("AddKiadas once Elmaradt: err = %v, want ErrKoltsegekClosed", err)
		}
		if err := h.App.DeleteKiadas(ctx, "bela", d.ID, k.ID); !errors.Is(err, app.ErrKoltsegekClosed) {
			t.Fatalf("DeleteKiadas once Elmaradt: err = %v, want ErrKoltsegekClosed", err)
		}
		if got := koltsegek(t, h, "bela", d.ID); got.CanAdd || len(got.Kiadasok) != 1 || got.Kiadasok[0].CanEdit {
			t.Fatalf("koltsegek once Elmaradt = %+v", got)
		}
	})

	t.Run("switched off", func(t *testing.T) {
		h, d := tripWithKoltsegek(t)
		switchKiegeszito(t, h, "anna", d.ID, domain.Koltsegek, false)
		if _, err := h.App.AddKiadas(ctx, "bela", d.ID, in); !errors.Is(err, app.ErrKiegeszitoOff) {
			t.Fatalf("AddKiadas while off: err = %v, want ErrKiegeszitoOff", err)
		}
		if _, err := h.App.Koltsegek(ctx, "bela", d.ID); !errors.Is(err, app.ErrKiegeszitoOff) {
			t.Fatalf("Koltsegek while off: err = %v, want ErrKiegeszitoOff", err)
		}
	})
}

func TestTheSplitIsFixedWhenADropOutHappens(t *testing.T) {
	h, d := tripWithKoltsegek(t)
	ctx := context.Background()
	k := addKiadas(t, h, "bela", d.ID, app.KiadasInput{Description: "Szállás", Amount: 900, PayerID: "cecil"})

	answer(t, h, "cecil", d, 0, domain.Nem) // the payer drops out

	allas := kiadasAllas(t, koltsegek(t, h, "anna", d.ID), k.ID)
	if got := reszek(allas); got != "anna=300 bela=300 cecil=300" || allas.PayerID != "cecil" {
		t.Fatalf("after Cecil dropped out: split %s, payer %s; want it unchanged", got, allas.PayerID)
	}
	// An edit keeps the people already in it, but can't add a non-Resztvevo.
	if err := h.App.UpdateKiadas(ctx, "bela", d.ID, k.ID, app.KiadasInput{Description: "Szállás", Amount: 1000}); err != nil {
		t.Fatalf("editing a split with a drop-out: %v", err)
	}
	if got := reszek(kiadasAllas(t, koltsegek(t, h, "anna", d.ID), k.ID)); got != "anna=334 bela=333 cecil=333" {
		t.Fatalf("split = %s", got)
	}
	if err := h.App.UpdateKiadas(ctx, "bela", d.ID, k.ID, app.KiadasInput{
		Description: "Szállás", Amount: 1000, Split: &app.SplitInput{Holders: holders("anna", "cecil", "dani")},
	}); !errors.Is(err, app.ErrNotResztvevo) {
		t.Fatalf("adding Dani to the split: err = %v, want ErrNotResztvevo", err)
	}
	// A new Kiadas's default split leaves Cecil out; he can't be the payer.
	k2 := addKiadas(t, h, "bela", d.ID, app.KiadasInput{Description: "Kaja", Amount: 100})
	if got := reszek(kiadasAllas(t, koltsegek(t, h, "anna", d.ID), k2.ID)); got != "anna=50 bela=50" {
		t.Fatalf("new split = %s", got)
	}
	if err := h.App.UpdateKiadas(ctx, "bela", d.ID, k2.ID, app.KiadasInput{Description: "Kaja", Amount: 100, PayerID: "cecil"}); !errors.Is(err, app.ErrNotResztvevo) {
		t.Fatalf("making Cecil the payer: err = %v, want ErrNotResztvevo", err)
	}

	// Re-confirming releases claims, but never Kiadasok.
	reopen(t, h, "anna", d.ID)
	confirm(t, h, "anna", d, 1) // nobody said igen to the second Idopont
	if got := koltsegek(t, h, "anna", d.ID); len(got.Kiadasok) != 2 || reszek(got.Kiadasok[0]) != "anna=334 bela=333 cecil=333" {
		t.Fatalf("after re-confirming: %+v", got.Kiadasok)
	}
}

func TestBalancesCountOnlyPaidKiadasok(t *testing.T) {
	h, d := tripWithKoltsegek(t)
	ctx := context.Background()
	addKiadas(t, h, "anna", d.ID, app.KiadasInput{Description: "Bor", Amount: 900}) // anna pays, 300 each
	addKiadas(t, h, "bela", d.ID, app.KiadasInput{
		Description: "Benzin", Amount: 600, Split: &app.SplitInput{Holders: holders("bela", "cecil")},
	}) // bela pays, 300 each
	cabin := addKiadas(t, h, "anna", d.ID, app.KiadasInput{
		Description: "Szállás", Amount: 120000, PayerID: "cecil", Allapot: domain.Tervezett,
	})

	k := koltsegek(t, h, "bela", d.ID)
	if got := egyenlegek(k); got != "anna:900/300/600 bela:600/600/0 cecil:0/600/-600" {
		t.Fatalf("balances = %s", got)
	}
	if k.Tervezett != 1 {
		t.Fatalf("tervezett = %d, want 1", k.Tervezett)
	}

	// Paid with the real amount, it counts.
	if err := h.App.UpdateKiadas(ctx, "cecil", d.ID, cabin.ID, app.KiadasInput{Description: "Szállás", Amount: 90000, Allapot: domain.Kifizetve}); err != nil {
		t.Fatalf("marking it paid: %v", err)
	}
	k = koltsegek(t, h, "bela", d.ID)
	if got := egyenlegek(k); got != "anna:900/30300/-29400 bela:600/30600/-30000 cecil:90000/30600/59400" {
		t.Fatalf("balances = %s", got)
	}
	if k.Tervezett != 0 {
		t.Fatalf("tervezett = %d, want 0", k.Tervezett)
	}
	// And back to planned.
	if err := h.App.UpdateKiadas(ctx, "cecil", d.ID, cabin.ID, app.KiadasInput{Description: "Szállás", Amount: 90000, Allapot: domain.Tervezett}); err != nil {
		t.Fatalf("marking it planned: %v", err)
	}
	if got := egyenlegek(koltsegek(t, h, "bela", d.ID)); got != "anna:900/300/600 bela:600/600/0 cecil:0/600/-600" {
		t.Fatalf("balances = %s", got)
	}
}

func TestKoltsegekCantBeSwitchedOffWhileItHasKiadasok(t *testing.T) {
	h, d := tripWithKoltsegek(t)
	ctx := context.Background()
	k := addKiadas(t, h, "bela", d.ID, app.KiadasInput{Description: "Bor", Amount: 1000})

	if err := h.App.SetKiegeszito(ctx, "anna", d.ID, domain.Koltsegek, false); !errors.Is(err, app.ErrKiegeszitoInUse) {
		t.Fatalf("switching off with a Kiadas: err = %v, want ErrKiegeszitoInUse", err)
	}
	if !kiegeszitoOf(t, detail(t, h, "anna", d.ID).Kiegeszitok, domain.Koltsegek).Enabled {
		t.Fatalf("Koltsegek was switched off")
	}
	if err := h.App.DeleteKiadas(ctx, "bela", d.ID, k.ID); err != nil {
		t.Fatalf("DeleteKiadas: %v", err)
	}
	switchKiegeszito(t, h, "anna", d.ID, domain.Koltsegek, false)
}

func TestKoltsegekCanBeSwitchedOnOnceLezajlott(t *testing.T) {
	h := apptest.New(t)
	c := csoportOf(t, h, "bela")
	e := propose(t, h, "anna", c.ID, 1)
	d := detail(t, h, "anna", e.ID)
	answer(t, h, "bela", d, 0, domain.Igen)
	confirm(t, h, "anna", d, 0)
	setLezajlott(t, h, d)

	got := detail(t, h, "anna", d.ID).Kiegeszitok
	if k := kiegeszitoOf(t, got, domain.Koltsegek); !k.Switchable {
		t.Fatalf("koltsegek = %+v, want it switchable once Lezajlott", k)
	}
	if k := kiegeszitoOf(t, got, domain.Felszereles); k.Switchable {
		t.Fatalf("felszereles = %+v, want it frozen once Lezajlott", k)
	}
	switchKiegeszito(t, h, "anna", d.ID, domain.Koltsegek, true)
	addKiadas(t, h, "bela", d.ID, app.KiadasInput{Description: "Pizza", Amount: 5000})

	cancelled := apptest.New(t)
	c2 := csoportOf(t, cancelled, "bela")
	e2 := propose(t, cancelled, "anna", c2.ID, 1)
	cancelEsemeny(t, cancelled, "anna", e2.ID)
	if err := cancelled.App.SetKiegeszito(context.Background(), "anna", e2.ID, domain.Koltsegek, true); !errors.Is(err, app.ErrKiegeszitoLocked) {
		t.Fatalf("switching on once Elmaradt: err = %v, want ErrKiegeszitoLocked", err)
	}
}

func TestTheEsemenyMessageCountsKiadasok(t *testing.T) {
	h := apptest.New(t)
	c := csoportOf(t, h, "bela")
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

	switchKiegeszito(t, h, "anna", e.ID, domain.Koltsegek, true)
	if got := lastSummary(); got != "" {
		t.Fatalf("summary without Kiadasok = %q, want none", got)
	}
	answer(t, h, "bela", d, 0, domain.Igen)
	confirm(t, h, "anna", d, 0)
	k := addKiadas(t, h, "bela", d.ID, app.KiadasInput{Description: "Bor", Amount: 1000})
	if got := lastSummary(); got != "🧾 1 Kiadás" {
		t.Fatalf("summary = %q, want 🧾 1 Kiadás", got)
	}
	addKiadas(t, h, "bela", d.ID, app.KiadasInput{Description: "Sör", Amount: 2000, Allapot: domain.Tervezett})
	if got := lastSummary(); got != "🧾 2 Kiadás" {
		t.Fatalf("summary = %q, want 🧾 2 Kiadás", got)
	}
	if err := h.App.DeleteKiadas(context.Background(), "bela", d.ID, k.ID); err != nil {
		t.Fatalf("DeleteKiadas: %v", err)
	}
	if got := lastSummary(); got != "🧾 1 Kiadás" {
		t.Fatalf("summary = %q, want 🧾 1 Kiadás", got)
	}
}

func TestThePayerCreatorAndSzervezoEditEvenIfNotResztvevok(t *testing.T) {
	ctx := context.Background()

	t.Run("a payer who dropped out", func(t *testing.T) {
		h, d := tripWithKoltsegek(t)
		k := addKiadas(t, h, "anna", d.ID, app.KiadasInput{Description: "Szállás", Amount: 900, PayerID: "cecil"})
		answer(t, h, "cecil", d, 0, domain.Nem)
		if !kiadasAllas(t, koltsegek(t, h, "cecil", d.ID), k.ID).CanEdit {
			t.Fatalf("the payer who dropped out doesn't see CanEdit")
		}
		if koltsegek(t, h, "cecil", d.ID).CanAdd {
			t.Fatalf("a drop-out sees CanAdd")
		}
		if err := h.App.UpdateKiadas(ctx, "cecil", d.ID, k.ID, app.KiadasInput{Description: "Szállás", Amount: 1200}); err != nil {
			t.Fatalf("the payer who dropped out editing: %v", err)
		}
		if err := h.App.DeleteKiadas(ctx, "cecil", d.ID, k.ID); err != nil {
			t.Fatalf("the payer who dropped out deleting: %v", err)
		}
		if _, err := h.App.AddKiadas(ctx, "cecil", d.ID, app.KiadasInput{Description: "x", Amount: 1}); !errors.Is(err, app.ErrKiadasNotResztvevo) {
			t.Fatalf("a drop-out adding: err = %v, want ErrKiadasNotResztvevo", err)
		}
	})

	t.Run("a creator who dropped out", func(t *testing.T) {
		h, d := tripWithKoltsegek(t)
		k := addKiadas(t, h, "bela", d.ID, app.KiadasInput{Description: "Bor", Amount: 900, PayerID: "anna"})
		answer(t, h, "bela", d, 0, domain.Nem)
		if err := h.App.UpdateKiadas(ctx, "bela", d.ID, k.ID, app.KiadasInput{Description: "Vörösbor", Amount: 900}); err != nil {
			t.Fatalf("the creator who dropped out editing: %v", err)
		}
		if err := h.App.DeleteKiadas(ctx, "bela", d.ID, k.ID); err != nil {
			t.Fatalf("the creator who dropped out deleting: %v", err)
		}
	})

	t.Run("a Szervezo who said nem", func(t *testing.T) {
		h, d := tripWithKoltsegek(t)
		k := addKiadas(t, h, "bela", d.ID, app.KiadasInput{Description: "Bor", Amount: 900})
		answer(t, h, "anna", d, 0, domain.Nem)
		if err := h.App.UpdateKiadas(ctx, "anna", d.ID, k.ID, app.KiadasInput{Description: "Bor", Amount: 600}); err != nil {
			t.Fatalf("the Szervezo who said nem editing: %v", err)
		}
		if got := reszek(kiadasAllas(t, koltsegek(t, h, "anna", d.ID), k.ID)); got != "anna=200 bela=200 cecil=200" {
			t.Fatalf("split = %s", got)
		}
	})

	t.Run("after a re-confirmation with no Resztvevok", func(t *testing.T) {
		h, d := tripWithKoltsegek(t)
		k := addKiadas(t, h, "bela", d.ID, app.KiadasInput{Description: "Bor", Amount: 900})
		reopen(t, h, "anna", d.ID)
		confirm(t, h, "anna", d, 1) // nobody said igen to it
		if err := h.App.UpdateKiadas(ctx, "bela", d.ID, k.ID, app.KiadasInput{Description: "Bor", Amount: 1000}); err != nil {
			t.Fatalf("the payer editing the amount: %v", err)
		}
		if got := reszek(kiadasAllas(t, koltsegek(t, h, "bela", d.ID), k.ID)); got != "anna=334 bela=333 cecil=333" {
			t.Fatalf("split = %s", got)
		}
		if err := h.App.UpdateKiadas(ctx, "bela", d.ID, k.ID, app.KiadasInput{
			Description: "Bor", Amount: 1000, Split: &app.SplitInput{Holders: holders("anna", "bela", "cecil", "dani")},
		}); !errors.Is(err, app.ErrKiadasNotResztvevo) {
			t.Fatalf("adding a non-Resztvevo to the split: err = %v, want ErrKiadasNotResztvevo", err)
		}
		if err := h.App.UpdateKiadas(ctx, "bela", d.ID, k.ID, app.KiadasInput{Description: "Bor", Amount: 1000, PayerID: "dani"}); !errors.Is(err, app.ErrKiadasNotResztvevo) {
			t.Fatalf("a non-Resztvevo payer: err = %v, want ErrKiadasNotResztvevo", err)
		}
	})
}

func TestAnEmptyDescriptionKeepsTheCurrentOneOnEdit(t *testing.T) {
	h, d := tripWithKoltsegek(t)
	k := addKiadas(t, h, "bela", d.ID, app.KiadasInput{Description: "Szállás", Amount: 900, Allapot: domain.Tervezett})
	if err := h.App.UpdateKiadas(context.Background(), "bela", d.ID, k.ID, app.KiadasInput{Amount: 1200, Allapot: domain.Kifizetve}); err != nil {
		t.Fatalf("UpdateKiadas: %v", err)
	}
	got := kiadasAllas(t, koltsegek(t, h, "bela", d.ID), k.ID)
	if got.Description != "Szállás" || got.Amount != 1200 || got.Allapot != domain.Kifizetve {
		t.Fatalf("kiadas = %+v", got.Kiadas)
	}
	if _, err := h.App.AddKiadas(context.Background(), "bela", d.ID, app.KiadasInput{Amount: 1}); !errors.Is(err, app.ErrInvalidInput) {
		t.Fatalf("adding without a description: err = %v, want ErrInvalidInput", err)
	}
}
