package app_test

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	"squadsync-be/internal/app"
	"squadsync-be/internal/apptest"
	"squadsync-be/internal/domain"
)

// The fake clock starts on 2026-03-02 09:00 UTC, which is 10:00 in
// Budapest: today is 2026-03-02, and the window ends on 2027-03-02.
const (
	today     = "2026-03-02"
	lastDay   = "2027-03-02"
	yesterday = "2026-03-01"
	tooLate   = "2027-03-03"
)

func mark(t *testing.T, h *apptest.Harness, felhasznaloID, from, to string, allapot domain.RaeresAllapot) {
	t.Helper()
	if err := h.App.SetRaeres(context.Background(), felhasznaloID, from, to, allapot); err != nil {
		t.Fatalf("SetRaeres(%s, %s..%s, %q): %v", felhasznaloID, from, to, allapot, err)
	}
}

func heatmap(t *testing.T, h *apptest.Harness, felhasznaloID, csoportID, from, to string) []app.RaeresDay {
	t.Helper()
	days, err := h.App.CsoportHeatmap(context.Background(), felhasznaloID, csoportID, from, to)
	if err != nil {
		t.Fatalf("CsoportHeatmap: %v", err)
	}
	return days
}

func TestOnlyTheRollingWindowCanBeMarked(t *testing.T) {
	h := apptest.New(t)
	addFelhasznalo(t, h, "anna", "Anna")
	ctx := context.Background()

	// Both ends of the window are fine.
	mark(t, h, "anna", today, today, domain.Szabad)
	mark(t, h, "anna", lastDay, lastDay, domain.Foglalt)

	for name, r := range map[string][2]string{
		"yesterday":                    {yesterday, yesterday},
		"the day after the window":     {tooLate, tooLate},
		"a range starting in the past": {yesterday, "2026-03-05"},
		"a range ending after":         {"2027-03-01", tooLate},
		"a range backwards":            {"2026-03-05", "2026-03-04"},
		"a malformed day":              {"2026-3-5", "2026-03-06"},
		"an impossible day":            {"2026-02-30", "2026-03-06"},
	} {
		err := h.App.SetRaeres(ctx, "anna", r[0], r[1], domain.Szabad)
		if !errors.Is(err, app.ErrInvalidInput) {
			t.Errorf("%s: SetRaeres = %v, want ErrInvalidInput", name, err)
		}
		err = h.App.SetRaeres(ctx, "anna", r[0], r[1], "")
		if !errors.Is(err, app.ErrInvalidInput) {
			t.Errorf("%s: clearing = %v, want ErrInvalidInput", name, err)
		}
	}
	if err := h.App.SetRaeres(ctx, "anna", today, today, "talan"); !errors.Is(err, app.ErrInvalidInput) {
		t.Errorf("an unknown allapot: SetRaeres = %v, want ErrInvalidInput", err)
	}

	// A refused range changed nothing.
	got, err := h.App.Raeres(ctx, "anna", today, "2027-12-31")
	if err != nil {
		t.Fatalf("Raeres: %v", err)
	}
	want := []domain.RaeresDay{{Day: today, Allapot: domain.Szabad}, {Day: lastDay, Allapot: domain.Foglalt}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Raeres = %+v, want %+v", got, want)
	}
}

func TestTheWindowStartsOnTodayInBudapest(t *testing.T) {
	h := apptest.New(t)
	addFelhasznalo(t, h, "anna", "Anna")
	// 23:30 UTC on 2 March is already 3 March in Budapest.
	h.Clock.Set(time.Date(2026, time.March, 2, 23, 30, 0, 0, time.UTC))

	if err := h.App.SetRaeres(context.Background(), "anna", today, today, domain.Szabad); !errors.Is(err, app.ErrInvalidInput) {
		t.Fatalf("marking 2 March = %v, want ErrInvalidInput", err)
	}
	mark(t, h, "anna", "2026-03-03", "2026-03-03", domain.Szabad)
	mark(t, h, "anna", "2027-03-03", "2027-03-03", domain.Szabad)
	if err := h.App.SetRaeres(context.Background(), "anna", "2027-03-04", "2027-03-04", domain.Szabad); !errors.Is(err, app.ErrInvalidInput) {
		t.Fatalf("marking beyond the window = %v, want ErrInvalidInput", err)
	}
}

func TestMarkingARangeOverwritesAndClears(t *testing.T) {
	h := apptest.New(t)
	addFelhasznalo(t, h, "anna", "Anna")
	ctx := context.Background()

	mark(t, h, "anna", "2026-03-10", "2026-03-14", domain.Szabad)
	mark(t, h, "anna", "2026-03-12", "2026-03-12", domain.Foglalt)
	mark(t, h, "anna", "2026-03-14", "2026-03-14", "")

	got, err := h.App.Raeres(ctx, "anna", "2026-03-01", "2026-03-31")
	if err != nil {
		t.Fatalf("Raeres: %v", err)
	}
	want := []domain.RaeresDay{
		{Day: "2026-03-10", Allapot: domain.Szabad},
		{Day: "2026-03-11", Allapot: domain.Szabad},
		{Day: "2026-03-12", Allapot: domain.Foglalt},
		{Day: "2026-03-13", Allapot: domain.Szabad},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Raeres = %+v, want %+v", got, want)
	}
}

func TestPastDaysAreNotShown(t *testing.T) {
	h := apptest.New(t)
	addFelhasznalo(t, h, "anna", "Anna")
	ctx := context.Background()
	mark(t, h, "anna", "2026-03-02", "2026-03-04", domain.Szabad)

	h.Clock.Advance(48 * time.Hour) // 4 March

	got, err := h.App.Raeres(ctx, "anna", "2026-03-01", "2026-03-31")
	if err != nil {
		t.Fatalf("Raeres: %v", err)
	}
	if want := []domain.RaeresDay{{Day: "2026-03-04", Allapot: domain.Szabad}}; !reflect.DeepEqual(got, want) {
		t.Fatalf("Raeres = %+v, want %+v", got, want)
	}
	// Nothing of a wholly past range is left, and it isn't an error to ask.
	got, err = h.App.Raeres(ctx, "anna", "2026-01-01", "2026-03-01")
	if err != nil || len(got) != 0 {
		t.Fatalf("Raeres of the past = %+v, %v, want nothing", got, err)
	}
}

func TestRaeresIsSharedAcrossCsoportok(t *testing.T) {
	h := apptest.New(t)
	ctx := context.Background()
	first := csoportOf(t, h, "bela") // Anna and Bela
	second, err := h.App.CreateCsoport(ctx, "anna", "Túrázók")
	if err != nil {
		t.Fatalf("CreateCsoport: %v", err)
	}

	mark(t, h, "anna", "2026-03-10", "2026-03-10", domain.Szabad)

	for _, c := range []domain.Csoport{first, second} {
		days := heatmap(t, h, "anna", c.ID, "2026-03-10", "2026-03-10")
		if len(days) != 1 || days[0].Szabad != 1 {
			t.Fatalf("heatmap of %s = %+v, want Anna counted once", c.Name, days)
		}
	}
}

func TestHeatmapCountsTagsOnly(t *testing.T) {
	h := apptest.New(t)
	ctx := context.Background()
	c := csoportOf(t, h, "bela", "cecil") // Anna, Bela and Cecil
	addFelhasznalo(t, h, "dora", "Dóra")  // not a Tag

	mark(t, h, "anna", "2026-03-10", "2026-03-12", domain.Szabad)
	mark(t, h, "bela", "2026-03-11", "2026-03-12", domain.Szabad)
	mark(t, h, "cecil", "2026-03-12", "2026-03-12", domain.Foglalt)
	mark(t, h, "dora", "2026-03-10", "2026-03-12", domain.Szabad)

	got := heatmap(t, h, "bela", c.ID, "2026-03-09", "2026-03-13")
	want := []app.RaeresDay{
		{Day: "2026-03-09", Szabad: 0, Osszes: 3},
		{Day: "2026-03-10", Szabad: 1, Osszes: 3},
		{Day: "2026-03-11", Szabad: 2, Osszes: 3},
		{Day: "2026-03-12", Szabad: 2, Osszes: 3},
		{Day: "2026-03-13", Szabad: 0, Osszes: 3},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("heatmap = %+v, want %+v", got, want)
	}

	// A Tag who leaves is no longer counted, nor is their Raeres.
	if err := h.App.LeaveCsoport(ctx, "bela", c.ID); err != nil {
		t.Fatalf("LeaveCsoport: %v", err)
	}
	got = heatmap(t, h, "anna", c.ID, "2026-03-11", "2026-03-11")
	if len(got) != 1 || got[0].Szabad != 1 || got[0].Osszes != 2 {
		t.Fatalf("heatmap after Bela left = %+v, want 1/2", got)
	}
}

func TestHeatmapIsCutToTheWindow(t *testing.T) {
	h := apptest.New(t)
	c := csoportOf(t, h)

	got := heatmap(t, h, "anna", c.ID, "2026-01-01", "2028-01-01")
	if len(got) != 366 || got[0].Day != today || got[365].Day != lastDay {
		t.Fatalf("heatmap spans %d days from %s to %s, want 366 from %s to %s", len(got), got[0].Day, got[len(got)-1].Day, today, lastDay)
	}
	if got := heatmap(t, h, "anna", c.ID, "2025-01-01", yesterday); len(got) != 0 {
		t.Fatalf("heatmap of the past = %+v, want none", got)
	}
}

func TestOnlyFellowTagsSeeAnything(t *testing.T) {
	h := apptest.New(t)
	ctx := context.Background()
	c := csoportOf(t, h, "bela")
	addFelhasznalo(t, h, "dora", "Dóra")
	if _, err := h.App.CreateCsoport(ctx, "dora", "Dóráék"); err != nil {
		t.Fatalf("CreateCsoport: %v", err)
	}
	e := propose(t, h, "anna", c.ID, 1)
	mark(t, h, "anna", "2026-03-10", "2026-03-10", domain.Szabad)

	// Dora is a Tag of another Csoport only, Edit of none, and nobody isn't known.
	addFelhasznalo(t, h, "edit", "Edit")
	for _, who := range []string{"dora", "edit", "nobody"} {
		if _, err := h.App.CsoportHeatmap(ctx, who, c.ID, "2026-03-10", "2026-03-10"); !errors.Is(err, app.ErrNotFound) {
			t.Errorf("CsoportHeatmap by %s = %v, want ErrNotFound", who, err)
		}
		if _, err := h.App.EsemenyRaeres(ctx, who, e.ID); !errors.Is(err, app.ErrNotFound) {
			t.Errorf("EsemenyRaeres by %s = %v, want ErrNotFound", who, err)
		}
		// Raeres is the caller's own only.
		got, err := h.App.Raeres(ctx, who, "2026-03-10", "2026-03-10")
		if err != nil || len(got) != 0 {
			t.Errorf("Raeres by %s = %+v, %v, want nothing", who, got, err)
		}
	}
	if _, err := h.App.CsoportHeatmap(ctx, "bela", c.ID, "2026-03-10", "2026-03-10"); err != nil {
		t.Errorf("CsoportHeatmap by a Tag: %v", err)
	}
}

func TestMarkingNeedsAKnownFelhasznalo(t *testing.T) {
	h := apptest.New(t)
	err := h.App.SetRaeres(context.Background(), "nobody", today, today, domain.Szabad)
	if !errors.Is(err, app.ErrFelhasznaloUnknown) {
		t.Fatalf("SetRaeres = %v, want ErrFelhasznaloUnknown", err)
	}
}

// esemenyWith proposes an Esemeny by Anna with the given Idopontok.
func esemenyWith(t *testing.T, h *apptest.Harness, csoportID string, idopontok ...app.IdopontInput) app.EsemenyDetail {
	t.Helper()
	e, err := h.App.ProposeEsemeny(context.Background(), "anna", csoportID, app.EsemenyInput{Title: "Túra", Idopontok: idopontok})
	if err != nil {
		t.Fatalf("ProposeEsemeny: %v", err)
	}
	return detail(t, h, "anna", e.ID)
}

// szabadAznap returns the X/Y of each Idopont of the Esemeny, as seen by
// the Felhasznalo.
func szabadAznap(t *testing.T, h *apptest.Harness, felhasznaloID, esemenyID string) [][2]int {
	t.Helper()
	list, err := h.App.EsemenyRaeres(context.Background(), felhasznaloID, esemenyID)
	if err != nil {
		t.Fatalf("EsemenyRaeres: %v", err)
	}
	out := make([][2]int, len(list))
	for n, i := range list {
		out[n] = [2]int{i.Szabad, i.Osszes}
	}
	return out
}

func TestSzabadAznapOfATimedIdopont(t *testing.T) {
	h := apptest.New(t)
	c := csoportOf(t, h, "bela", "cecil")
	d := esemenyWith(t, h, c.ID,
		app.IdopontInput{Start: "2026-12-05T20:00", End: "2026-12-05T23:00"},
		app.IdopontInput{Start: "2026-12-06T18:00"},
	)
	mark(t, h, "anna", "2026-12-05", "2026-12-06", domain.Szabad)
	mark(t, h, "bela", "2026-12-05", "2026-12-05", domain.Szabad)
	mark(t, h, "bela", "2026-12-06", "2026-12-06", domain.Foglalt)
	// Cecil marked nothing: unknown isn't free.

	if got, want := szabadAznap(t, h, "cecil", d.ID), [][2]int{{2, 3}, {1, 3}}; !reflect.DeepEqual(got, want) {
		t.Fatalf("szabad aznap = %v, want %v", got, want)
	}
}

func TestSzabadAznapOfAMultiDayIdopontCountsOnlyTagsFreeOnEveryDay(t *testing.T) {
	h := apptest.New(t)
	c := csoportOf(t, h, "bela", "cecil")
	d := esemenyWith(t, h, c.ID, app.IdopontInput{Start: "2026-12-04T18:00", End: "2026-12-06T12:00"})
	mark(t, h, "anna", "2026-12-04", "2026-12-06", domain.Szabad)
	mark(t, h, "bela", "2026-12-04", "2026-12-05", domain.Szabad) // not the 6th
	mark(t, h, "cecil", "2026-12-04", "2026-12-06", domain.Szabad)
	mark(t, h, "cecil", "2026-12-05", "2026-12-05", domain.Foglalt) // busy in between

	if got, want := szabadAznap(t, h, "anna", d.ID), [][2]int{{1, 3}}; !reflect.DeepEqual(got, want) {
		t.Fatalf("szabad aznap = %v, want %v", got, want)
	}
}

func TestSzabadAznapOfAWholeDayIdopontStopsAtItsLastDay(t *testing.T) {
	h := apptest.New(t)
	c := csoportOf(t, h, "bela")
	d := esemenyWith(t, h, c.ID,
		app.IdopontInput{Start: "2026-12-05", End: "2026-12-07", EgeszNapos: true},
		app.IdopontInput{Start: "2026-12-10", EgeszNapos: true},
	)
	// Anna is free on the three days but foglalt on the 8th, the midnight
	// after the range: that must not matter.
	mark(t, h, "anna", "2026-12-05", "2026-12-07", domain.Szabad)
	mark(t, h, "anna", "2026-12-08", "2026-12-08", domain.Foglalt)
	mark(t, h, "bela", "2026-12-05", "2026-12-06", domain.Szabad) // not the 7th
	mark(t, h, "bela", "2026-12-09", "2026-12-11", domain.Szabad) // around the single day
	mark(t, h, "anna", "2026-12-10", "2026-12-10", domain.Szabad)

	if got, want := szabadAznap(t, h, "bela", d.ID), [][2]int{{1, 2}, {2, 2}}; !reflect.DeepEqual(got, want) {
		t.Fatalf("szabad aznap = %v, want %v", got, want)
	}
}

func TestSzabadAznapUsesBudapestDaysAtMidnight(t *testing.T) {
	h := apptest.New(t)
	c := csoportOf(t, h, "bela")
	d := esemenyWith(t, h, c.ID,
		// 00:30 on the 5th in Budapest is still the 4th in UTC.
		app.IdopontInput{Start: "2026-12-05T00:30", End: "2026-12-05T02:00"},
		// 23:30 on the 5th in Budapest is the 5th in UTC too, but ending at
		// Budapest midnight doesn't reach into the 6th.
		app.IdopontInput{Start: "2026-12-05T23:30", End: "2026-12-06T00:00"},
		// Ending half an hour into the 6th does.
		app.IdopontInput{Start: "2026-12-05T23:30", End: "2026-12-06T00:30"},
	)
	mark(t, h, "anna", "2026-12-05", "2026-12-05", domain.Szabad)
	mark(t, h, "bela", "2026-12-04", "2026-12-04", domain.Szabad)
	mark(t, h, "bela", "2026-12-05", "2026-12-06", domain.Szabad)

	// Idopontok are in chronological order.
	if got, want := szabadAznap(t, h, "anna", d.ID), [][2]int{{2, 2}, {2, 2}, {1, 2}}; !reflect.DeepEqual(got, want) {
		t.Fatalf("szabad aznap = %v, want %v", got, want)
	}
}

func TestSzabadAznapOfAWholeDayIdopontAcrossTheAutumnClockChange(t *testing.T) {
	h := apptest.New(t)
	c := csoportOf(t, h, "bela")
	// The clocks go back on 25 October 2026, so that day has 25 hours.
	d := esemenyWith(t, h, c.ID, app.IdopontInput{Start: "2026-10-24", End: "2026-10-26", EgeszNapos: true})
	mark(t, h, "anna", "2026-10-24", "2026-10-26", domain.Szabad)
	mark(t, h, "bela", "2026-10-24", "2026-10-24", domain.Szabad)
	mark(t, h, "bela", "2026-10-26", "2026-10-26", domain.Szabad)

	if got, want := szabadAznap(t, h, "anna", d.ID), [][2]int{{1, 2}}; !reflect.DeepEqual(got, want) {
		t.Fatalf("szabad aznap = %v, want %v", got, want)
	}
}

func TestOwnRaeresHintOfAnIdopont(t *testing.T) {
	h := apptest.New(t)
	c := csoportOf(t, h)
	d := esemenyWith(t, h, c.ID,
		app.IdopontInput{Start: "2026-12-05T20:00"},
		app.IdopontInput{Start: "2026-12-06T20:00", End: "2026-12-07T10:00"},
		app.IdopontInput{Start: "2026-12-08T20:00", End: "2026-12-09T10:00"},
		app.IdopontInput{Start: "2026-12-10T20:00", End: "2026-12-11T10:00"},
	)
	mark(t, h, "anna", "2026-12-05", "2026-12-05", domain.Szabad)
	mark(t, h, "anna", "2026-12-06", "2026-12-07", domain.Szabad)
	mark(t, h, "anna", "2026-12-08", "2026-12-08", domain.Szabad)
	mark(t, h, "anna", "2026-12-09", "2026-12-09", domain.Foglalt)
	mark(t, h, "anna", "2026-12-10", "2026-12-10", domain.Szabad) // the 11th is unknown

	list, err := h.App.EsemenyRaeres(context.Background(), "anna", d.ID)
	if err != nil {
		t.Fatalf("EsemenyRaeres: %v", err)
	}
	got := make([]domain.RaeresAllapot, len(list))
	for n, i := range list {
		got[n] = i.Sajat
	}
	want := []domain.RaeresAllapot{domain.Szabad, domain.Szabad, domain.Foglalt, ""}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("own hints = %q, want %q", got, want)
	}
}

// editedEsemenyek returns the ids of the Esemenyek whose message was edited
// since the first n edits.
func editedEsemenyek(h *apptest.Harness, n int) map[string]bool {
	out := map[string]bool{}
	for _, e := range h.Discord.Edits()[n:] {
		out[e.Message.EsemenyID] = true
	}
	return out
}

func TestMarkingRaeresRefreshesTheMessagesOfEsemenyekOnThoseDays(t *testing.T) {
	h := apptest.New(t)
	ctx := context.Background()
	a := csoportOf(t, h, "bela")
	linkCsatorna(t, h, a.ID)
	// Anna's second Csoport, one of Bela's own that Anna isn't in, and one
	// of Anna's with a confirmed Esemeny.
	anna2, err := h.App.CreateCsoport(ctx, "anna", "Túrázók")
	if err != nil {
		t.Fatalf("CreateCsoport: %v", err)
	}
	bela2, err := h.App.CreateCsoport(ctx, "bela", "Bélaék")
	if err != nil {
		t.Fatalf("CreateCsoport: %v", err)
	}
	anna3, err := h.App.CreateCsoport(ctx, "anna", "Régiek")
	if err != nil {
		t.Fatalf("CreateCsoport: %v", err)
	}
	for who, link := range map[string]app.LinkCsatornaInput{
		"anna": {CsoportID: anna2.ID, GuildID: "guild-1", ChannelID: "chan-2"},
		"bela": {CsoportID: bela2.ID, GuildID: "guild-1", ChannelID: "chan-3"},
	} {
		if _, err := h.App.LinkCsatorna(ctx, who, link); err != nil {
			t.Fatalf("LinkCsatorna: %v", err)
		}
	}
	if _, err := h.App.LinkCsatorna(ctx, "anna", app.LinkCsatornaInput{CsoportID: anna3.ID, GuildID: "guild-1", ChannelID: "chan-4"}); err != nil {
		t.Fatalf("LinkCsatorna: %v", err)
	}
	dec1 := app.IdopontInput{Start: "2026-12-01T20:00", End: "2026-12-01T23:00"}
	inA := esemenyWith(t, h, a.ID, dec1, app.IdopontInput{Start: "2026-12-20T20:00"})
	inAnna2 := esemenyWith(t, h, anna2.ID, app.IdopontInput{Start: "2026-12-09", End: "2026-12-10", EgeszNapos: true})
	if _, err := h.App.ProposeEsemeny(ctx, "bela", bela2.ID, app.EsemenyInput{Title: "Béla", Idopontok: []app.IdopontInput{dec1}}); err != nil {
		t.Fatalf("ProposeEsemeny: %v", err)
	}
	confirmed := esemenyWith(t, h, anna3.ID, dec1)
	if err := h.App.ConfirmIdopont(ctx, "anna", confirmed.ID, confirmed.Idopontok[0].ID); err != nil {
		t.Fatalf("ConfirmIdopont: %v", err)
	}

	before := len(h.Discord.Edits())
	mark(t, h, "anna", "2026-12-01", "2026-12-01", domain.Szabad)
	// Only the Egyeztetes Esemeny of Anna's Csoport with an Idopont that
	// day: not the other days' Esemeny, not Bela's other Csoport, and not
	// the confirmed one.
	if got, want := editedEsemenyek(h, before), map[string]bool{inA.ID: true}; !reflect.DeepEqual(got, want) {
		t.Fatalf("edited after marking 1 December = %v, want %v", got, want)
	}

	before = len(h.Discord.Edits())
	mark(t, h, "anna", "2026-12-10", "2026-12-12", domain.Foglalt)
	if got, want := editedEsemenyek(h, before), map[string]bool{inAnna2.ID: true}; !reflect.DeepEqual(got, want) {
		t.Fatalf("edited after marking 10 to 12 December = %v, want %v", got, want)
	}

	// A day with no Idopont, and a day touching an Idopont only at its end
	// (the last Idopont of Anna's Túrázók ends at midnight of the 10th).
	before = len(h.Discord.Edits())
	mark(t, h, "anna", "2026-12-05", "2026-12-08", domain.Szabad)
	if got := editedEsemenyek(h, before); len(got) != 0 {
		t.Fatalf("edited after marking quiet days = %v, want none", got)
	}
}

func TestSzabadAznapIsAbsentForDaysOutsideTheWindow(t *testing.T) {
	h := apptest.New(t)
	c := csoportOf(t, h, "bela")
	// Today is 2 March 2026 and the window ends on 2 March 2027.
	d := esemenyWith(t, h, c.ID,
		app.IdopontInput{Start: "2026-02-20T20:00", End: "2026-02-21T22:00"},
		app.IdopontInput{Start: "2026-03-01T20:00", End: "2026-03-02T00:00"}, // ended before today
		app.IdopontInput{Start: "2026-03-01T20:00", End: "2026-03-02T09:00"}, // reaches into today
		app.IdopontInput{Start: "2027-03-03T10:00"},                          // beyond the window
		app.IdopontInput{Start: "2027-03-02T10:00"},                          // on its last day
	)
	list, err := h.App.EsemenyRaeres(context.Background(), "anna", d.ID)
	if err != nil {
		t.Fatalf("EsemenyRaeres: %v", err)
	}
	got := make([]bool, len(list))
	for n, i := range list {
		got[n] = i.NincsRaeres
	}
	if want := []bool{true, true, false, false, true}; !reflect.DeepEqual(got, want) {
		t.Fatalf("NincsRaeres = %v, want %v", got, want)
	}
}
