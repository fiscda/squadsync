package app_test

import (
	"context"
	"errors"
	"testing"

	"squadsync-be/internal/app"
	"squadsync-be/internal/apptest"
	"squadsync-be/internal/domain"
)

// uzemanyagTrip is autokTrip (Resztvevok anna, bela, cecil, dora) with
// Koltsegek on, and Bela driving an Auto with Cecil and Dora in it.
func uzemanyagTrip(t *testing.T) (*apptest.Harness, app.EsemenyDetail, domain.Auto) {
	t.Helper()
	h, d := autokTrip(t)
	switchKiegeszito(t, h, "anna", d.ID, domain.Koltsegek, true)
	auto := autoOffer(t, h, "bela", d.ID, 3)
	autoClaim(t, h, "cecil", d.ID, auto.ID)
	autoClaim(t, h, "dora", d.ID, auto.ID)
	return h, d, auto
}

func TestAFuelKiadasIsSplitAmongTheSoforAndThePassengers(t *testing.T) {
	h, d, auto := uzemanyagTrip(t)
	ctx := context.Background()
	k, err := h.App.AddUzemanyagKiadas(ctx, "cecil", d.ID, auto.ID, app.UzemanyagInput{Amount: 1000, PayerID: "bela"})
	if err != nil {
		t.Fatalf("AddUzemanyagKiadas: %v", err)
	}
	allas := kiadasAllas(t, koltsegek(t, h, "anna", d.ID), k.ID)
	if got := reszek(allas); got != "bela=334 cecil=333 dora=333" {
		t.Fatalf("split = %s, want bela=334 cecil=333 dora=333 (Anna isn't in the Auto)", got)
	}
	if allas.Description != "Üzemanyag – bela autója" || allas.PayerID != "bela" || allas.CreatedBy != "cecil" ||
		allas.Allapot != domain.Kifizetve || !allas.Egyenlo {
		t.Fatalf("kiadas = %+v", allas.Kiadas)
	}

	// A description of their own, and the caller as the default payer.
	k, err = h.App.AddUzemanyagKiadas(ctx, "dora", d.ID, auto.ID, app.UzemanyagInput{Amount: 5000, Description: "Benzin Győr"})
	if err != nil {
		t.Fatalf("AddUzemanyagKiadas: %v", err)
	}
	allas = kiadasAllas(t, koltsegek(t, h, "anna", d.ID), k.ID)
	if allas.Description != "Benzin Győr" || allas.PayerID != "dora" {
		t.Fatalf("kiadas = %+v", allas.Kiadas)
	}
}

func TestAFuelKiadasCanBeSplitAmongEveryResztvevo(t *testing.T) {
	h, d, auto := uzemanyagTrip(t)
	k, err := h.App.AddUzemanyagKiadas(context.Background(), "bela", d.ID, auto.ID, app.UzemanyagInput{Amount: 1001, Mindenki: true})
	if err != nil {
		t.Fatalf("AddUzemanyagKiadas: %v", err)
	}
	if got := reszek(kiadasAllas(t, koltsegek(t, h, "anna", d.ID), k.ID)); got != "anna=251 bela=250 cecil=250 dora=250" {
		t.Fatalf("split = %s", got)
	}
}

func TestASoforOnlyAutoSplitsToTheSofor(t *testing.T) {
	h, d := autokTrip(t)
	switchKiegeszito(t, h, "anna", d.ID, domain.Koltsegek, true)
	auto := autoOffer(t, h, "bela", d.ID, 2)
	k, err := h.App.AddUzemanyagKiadas(context.Background(), "bela", d.ID, auto.ID, app.UzemanyagInput{Amount: 700})
	if err != nil {
		t.Fatalf("AddUzemanyagKiadas: %v", err)
	}
	if got := reszek(kiadasAllas(t, koltsegek(t, h, "anna", d.ID), k.ID)); got != "bela=700" {
		t.Fatalf("split = %s, want bela=700", got)
	}
}

func TestAFuelKiadasNeedsKoltsegekOn(t *testing.T) {
	h, d := autokTrip(t)
	auto := autoOffer(t, h, "bela", d.ID, 2)
	_, err := h.App.AddUzemanyagKiadas(context.Background(), "bela", d.ID, auto.ID, app.UzemanyagInput{Amount: 700})
	if !errors.Is(err, app.ErrAutoKoltsegekOff) {
		t.Fatalf("err = %v, want ErrAutoKoltsegekOff", err)
	}
}

func TestAFuelKiadasRespectsTheElszamolasLock(t *testing.T) {
	h, d, auto := uzemanyagTrip(t)
	ctx := context.Background()
	if _, err := h.App.AddUzemanyagKiadas(ctx, "bela", d.ID, auto.ID, app.UzemanyagInput{Amount: 900}); err != nil {
		t.Fatalf("AddUzemanyagKiadas: %v", err)
	}
	setLezajlott(t, h, d)
	confirmKiadasok(t, h, d.ID, "anna", "bela", "cecil", "dora")
	_, err := h.App.AddUzemanyagKiadas(ctx, "bela", d.ID, auto.ID, app.UzemanyagInput{Amount: 900})
	if !errors.Is(err, app.ErrKiadasokLocked) {
		t.Fatalf("err = %v, want ErrKiadasokLocked", err)
	}
}

func TestAFuelKiadasIsRefusedForNonResztvevokAndBadPayers(t *testing.T) {
	h, d, auto := uzemanyagTrip(t)
	ctx := context.Background()
	// Emil is a Tag who said nem.
	if _, err := h.App.AddUzemanyagKiadas(ctx, "emil", d.ID, auto.ID, app.UzemanyagInput{Amount: 900}); !errors.Is(err, app.ErrKiadasNotResztvevo) {
		t.Fatalf("Emil: err = %v, want ErrKiadasNotResztvevo", err)
	}
	if _, err := h.App.AddUzemanyagKiadas(ctx, "bela", d.ID, auto.ID, app.UzemanyagInput{Amount: 900, PayerID: "emil"}); !errors.Is(err, app.ErrKiadasNotResztvevo) {
		t.Fatalf("payer Emil: err = %v, want ErrKiadasNotResztvevo", err)
	}
	// A stranger sees nothing.
	if _, err := h.App.AddUzemanyagKiadas(ctx, "zoli", d.ID, auto.ID, app.UzemanyagInput{Amount: 900}); !errors.Is(err, app.ErrNotFound) {
		t.Fatalf("stranger: err = %v, want ErrNotFound", err)
	}
	if _, err := h.App.AddUzemanyagKiadas(ctx, "bela", d.ID, "nope", app.UzemanyagInput{Amount: 900}); !errors.Is(err, app.ErrNotFound) {
		t.Fatalf("no Auto: err = %v, want ErrNotFound", err)
	}
	if _, err := h.App.AddUzemanyagKiadas(ctx, "bela", d.ID, auto.ID, app.UzemanyagInput{Amount: 0}); !errors.Is(err, app.ErrInvalidInput) {
		t.Fatalf("no amount: err = %v, want ErrInvalidInput", err)
	}
	if n := len(koltsegek(t, h, "anna", d.ID).Kiadasok); n != 0 {
		t.Fatalf("%d kiadasok, want none", n)
	}
}
