package app_test

import (
	"context"
	"errors"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"squadsync-be/internal/app"
	"squadsync-be/internal/apptest"
	"squadsync-be/internal/domain"
	"squadsync-be/internal/gateway"
	"squadsync-be/internal/gateway/gatewayfake"
	"squadsync-be/internal/store"
)

// The first Idopont of idopontok(n): 2026-12-01 20:00–23:00 in Budapest
// (UTC+1 in December).
var (
	firstStart = time.Date(2026, time.December, 1, 19, 0, 0, 0, time.UTC)
	firstEnd   = time.Date(2026, time.December, 1, 22, 0, 0, 0, time.UTC)
)

// runAt moves the fake clock to t and runs the due jobs.
func runAt(t *testing.T, h *apptest.Harness, at time.Time) {
	t.Helper()
	h.Clock.Set(at)
	if err := h.App.RunDueJobs(context.Background()); err != nil {
		t.Fatalf("RunDueJobs: %v", err)
	}
}

func allapotOf(t *testing.T, h *apptest.Harness, esemenyID string) domain.Allapot {
	t.Helper()
	return detail(t, h, "anna", esemenyID).Allapot
}

// emlekeztetoDMs are the Emlekezteto DMs sent so far, as "discord id:
// idopont" lines.
func emlekeztetoDMs(h *apptest.Harness) []string {
	var out []string
	for _, dm := range h.Discord.DMs() {
		if dm.Emlekezteto != nil {
			out = append(out, dm.DiscordID+": "+dm.Emlekezteto.Idopont)
		}
	}
	return out
}

func hataridoDMs(h *apptest.Harness) []gatewayfake.DM {
	var out []gatewayfake.DM
	for _, dm := range h.Discord.DMs() {
		if dm.Hatarido != nil {
			out = append(out, dm)
		}
	}
	return out
}

func expectLines(t *testing.T, what string, got []string, want ...string) {
	t.Helper()
	if !slices.Equal(got, want) {
		t.Fatalf("%s = %q, want %q", what, got, want)
	}
}

func TestLezajlottAtTheEndOfTheConfirmedIdopont(t *testing.T) {
	h := apptest.New(t)
	c := csoportOf(t, h, "bela")
	e := propose(t, h, "anna", c.ID, 2)
	d := detail(t, h, "anna", e.ID)
	confirm(t, h, "anna", d, 0)

	runAt(t, h, firstEnd.Add(-time.Minute))
	if got := allapotOf(t, h, e.ID); got != domain.Veglegesitve {
		t.Fatalf("allapot before the end = %s", got)
	}
	editsBefore := len(h.Discord.Edits())

	runAt(t, h, firstEnd)
	got := detail(t, h, "anna", e.ID)
	if got.Allapot != domain.Lezajlott || got.ConfirmedIdopont != d.Idopontok[0].ID {
		t.Fatalf("after the end: %+v, want Lezajlott on the confirmed Idopont", got.Esemeny)
	}
	// Each DM'd Esemeny message is re-rendered: Lezajlott, with no buttons.
	edits := h.Discord.Edits()[editsBefore:]
	if len(edits) != 2 {
		t.Fatalf("edits = %d, want one per Esemeny message (Anna's and Bela's)", len(edits))
	}
	for _, ed := range edits {
		if ed.Message.Allapot != "Lezajlott" {
			t.Fatalf("edited allapot = %q, want Lezajlott", ed.Message.Allapot)
		}
		for _, l := range ed.Message.Idopontok {
			if l.Answerable {
				t.Fatalf("a Lezajlott message still has answer buttons: %+v", l)
			}
		}
	}
}

func TestLezajlottAtTheStartOfAnIdopontWithoutEnd(t *testing.T) {
	h := apptest.New(t)
	c := csoportOf(t, h, "bela")
	e, err := h.App.ProposeEsemeny(context.Background(), "anna", c.ID, app.EsemenyInput{
		Title:     "Mozi",
		Idopontok: []app.IdopontInput{{Start: "2026-12-05T18:00"}},
	})
	if err != nil {
		t.Fatalf("ProposeEsemeny: %v", err)
	}
	confirm(t, h, "anna", detail(t, h, "anna", e.ID), 0)
	start := time.Date(2026, time.December, 5, 17, 0, 0, 0, time.UTC)

	runAt(t, h, start.Add(-time.Minute))
	if got := allapotOf(t, h, e.ID); got != domain.Veglegesitve {
		t.Fatalf("allapot before the start = %s", got)
	}
	runAt(t, h, start)
	if got := allapotOf(t, h, e.ID); got != domain.Lezajlott {
		t.Fatalf("allapot at the start = %s, want Lezajlott", got)
	}
}

// remindedWorld has Anna (Szervezo, igen, but muted), Bela (igen), Cili
// (talán), Dani (nem) and Eva (no answer) on an Esemeny confirmed on its
// first Idopont.
func remindedWorld(t *testing.T) (*apptest.Harness, app.EsemenyDetail) {
	t.Helper()
	h := apptest.New(t)
	c := csoportOf(t, h, "bela", "cili", "dani", "eva")
	e := propose(t, h, "anna", c.ID, 2)
	d := detail(t, h, "anna", e.ID)
	answer(t, h, "anna", d, 0, domain.Igen)
	answer(t, h, "bela", d, 0, domain.Igen)
	answer(t, h, "cili", d, 0, domain.Talan)
	answer(t, h, "dani", d, 0, domain.Nem)
	answer(t, h, "eva", d, 1, domain.Igen) // the other Idopont
	if err := h.App.SetMuted(context.Background(), "anna", c.ID, true); err != nil {
		t.Fatalf("SetMuted: %v", err)
	}
	confirm(t, h, "anna", d, 0)
	return h, d
}

func TestEmlekeztetokGoToIgenAndTalanTagsAtTheDefaultOffsets(t *testing.T) {
	h, d := remindedWorld(t)
	label := "dec. 1. (kedd) 20:00–23:00"

	runAt(t, h, firstStart.Add(-24*time.Hour-time.Minute))
	expectLines(t, "emlekeztetok a day and a minute before", emlekeztetoDMs(h))

	runAt(t, h, firstStart.Add(-24*time.Hour))
	expectLines(t, "emlekeztetok a day before", emlekeztetoDMs(h),
		"discord-bela: "+label, "discord-cili: "+label)

	runAt(t, h, firstStart.Add(-time.Hour-time.Minute))
	expectLines(t, "emlekeztetok until an hour before", emlekeztetoDMs(h),
		"discord-bela: "+label, "discord-cili: "+label)

	runAt(t, h, firstStart.Add(-time.Hour))
	expectLines(t, "emlekeztetok an hour before", emlekeztetoDMs(h),
		"discord-bela: "+label, "discord-cili: "+label, "discord-bela: "+label, "discord-cili: "+label)

	var dm gatewayfake.DM
	for _, m := range h.Discord.DMs() {
		if m.Emlekezteto != nil {
			dm = m
		}
	}
	want := gateway.Emlekezteto{CsoportName: "Péntek esték", Title: "Társasjáték", Idopont: label, Link: apptest.WebURL + "/esemenyek/" + d.ID}
	if *dm.Emlekezteto != want {
		t.Fatalf("emlekezteto = %+v, want %+v", *dm.Emlekezteto, want)
	}
}

func TestRunningTheSameTickTwiceSendsNothingTwice(t *testing.T) {
	h, d := remindedWorld(t)
	h.Clock.Set(firstStart.Add(-24 * time.Hour))

	var wg sync.WaitGroup
	for range 4 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := h.App.RunDueJobs(context.Background()); err != nil {
				t.Errorf("RunDueJobs: %v", err)
			}
		}()
	}
	wg.Wait()
	runAt(t, h, firstStart.Add(-24*time.Hour))
	if got := emlekeztetoDMs(h); len(got) != 2 {
		t.Fatalf("emlekeztetok = %q, want one each for Bela and Cili", got)
	}

	runAt(t, h, firstStart.Add(-time.Hour))
	runAt(t, h, firstStart.Add(-time.Hour))
	runAt(t, h, firstEnd)
	runAt(t, h, firstEnd)
	if got := allapotOf(t, h, d.ID); got != domain.Lezajlott {
		t.Fatalf("allapot = %s", got)
	}
	if got := emlekeztetoDMs(h); len(got) != 4 {
		t.Fatalf("emlekeztetok = %q, want the two offsets once each", got)
	}
}

func TestReopeningCancelsAndReconfirmingReschedules(t *testing.T) {
	h, d := remindedWorld(t)
	reopen(t, h, "anna", d.ID)

	runAt(t, h, firstEnd)
	if len(emlekeztetoDMs(h)) != 0 {
		t.Fatalf("emlekeztetok after reopening = %q, want none", emlekeztetoDMs(h))
	}
	if got := allapotOf(t, h, d.ID); got != domain.Egyeztetes {
		t.Fatalf("allapot after the reopened Idopont's end = %s, want Egyeztetes", got)
	}

	// Confirmed again, on the second Idopont, a day later than the first.
	confirm(t, h, "anna", d, 1)
	second := firstStart.Add(24 * time.Hour)
	runAt(t, h, second.Add(-time.Hour))
	expectLines(t, "emlekeztetok of the second Idopont", emlekeztetoDMs(h),
		"discord-eva: dec. 2. (szerda) 20:00–23:00")
	runAt(t, h, second.Add(3*time.Hour))
	if got := allapotOf(t, h, d.ID); got != domain.Lezajlott {
		t.Fatalf("allapot after the second Idopont's end = %s, want Lezajlott", got)
	}
}

func TestReconfirmingTheSameIdopontDoesntRemindTwice(t *testing.T) {
	h, d := remindedWorld(t)
	runAt(t, h, firstStart.Add(-24*time.Hour))
	reopen(t, h, "anna", d.ID)
	confirm(t, h, "anna", d, 0)

	runAt(t, h, firstStart.Add(-time.Hour))
	if got := emlekeztetoDMs(h); len(got) != 4 {
		t.Fatalf("emlekeztetok = %q, want 24h and 1h once each for Bela and Cili", got)
	}
}

func TestElmaradtCancelsTheTimedJobs(t *testing.T) {
	h, d := remindedWorld(t)
	cancelEsemeny(t, h, "anna", d.ID)
	runAt(t, h, firstEnd.Add(time.Hour))
	if len(emlekeztetoDMs(h)) != 0 {
		t.Fatalf("emlekeztetok of an Elmaradt Esemeny = %q", emlekeztetoDMs(h))
	}
	if got := allapotOf(t, h, d.ID); got != domain.Elmaradt {
		t.Fatalf("allapot = %s, want Elmaradt to stay", got)
	}
}

func TestJobsMissedDuringDowntimeRunLate(t *testing.T) {
	h, d := remindedWorld(t)

	// Down from long before until 30 minutes before the start: only the
	// 1-hour Emlekezteto goes out, late, not both at once.
	runAt(t, h, firstStart.Add(-30*time.Minute))
	expectLines(t, "late emlekeztetok", emlekeztetoDMs(h),
		"discord-bela: dec. 1. (kedd) 20:00–23:00", "discord-cili: dec. 1. (kedd) 20:00–23:00")

	// Down again until the day after: Lezajlott happens late.
	runAt(t, h, firstEnd.Add(20*time.Hour))
	if got := allapotOf(t, h, d.ID); got != domain.Lezajlott {
		t.Fatalf("allapot after downtime = %s, want Lezajlott", got)
	}
}

func TestAnEmlekeztetoIsNotSentOnceTheIdopontStarted(t *testing.T) {
	h, d := remindedWorld(t)
	runAt(t, h, firstStart.Add(time.Minute))
	if len(emlekeztetoDMs(h)) != 0 {
		t.Fatalf("emlekeztetok after the start = %q, want none", emlekeztetoDMs(h))
	}
	if got := allapotOf(t, h, d.ID); got != domain.Veglegesitve {
		t.Fatalf("allapot = %s", got)
	}
}

func TestConfirmingShortlyBeforeTheStartSendsOnlyTheLastEmlekezteto(t *testing.T) {
	h := apptest.New(t)
	c := csoportOf(t, h, "bela")
	e := propose(t, h, "anna", c.ID, 1)
	d := detail(t, h, "anna", e.ID)
	answer(t, h, "bela", d, 0, domain.Igen)
	h.Clock.Set(firstStart.Add(-30 * time.Minute))
	confirm(t, h, "anna", d, 0)

	runAt(t, h, firstStart.Add(-30*time.Minute))
	expectLines(t, "emlekeztetok", emlekeztetoDMs(h), "discord-bela: dec. 1. (kedd) 20:00–23:00")
	runAt(t, h, firstStart.Add(-time.Minute))
	if len(emlekeztetoDMs(h)) != 1 {
		t.Fatalf("emlekeztetok = %q, want just the one", emlekeztetoDMs(h))
	}
}

func TestAnAdminChangesTheEmlekeztetokAndFutureOnesAreRescheduled(t *testing.T) {
	h, d := remindedWorld(t)
	ctx := context.Background()
	c, err := h.App.Csoport(ctx, "bela", d.CsoportID)
	if err != nil {
		t.Fatalf("Csoport: %v", err)
	}
	if !slices.Equal(c.Emlekeztetok, []time.Duration{24 * time.Hour, time.Hour}) {
		t.Fatalf("default emlekeztetok = %v, want 24h and 1h", c.Emlekeztetok)
	}

	if err := h.App.SetEmlekeztetok(ctx, "bela", d.CsoportID, []time.Duration{3 * time.Hour}); !errors.Is(err, app.ErrNotAdmin) {
		t.Fatalf("SetEmlekeztetok by a Tag: err = %v, want ErrNotAdmin", err)
	}
	if err := h.App.SetEmlekeztetok(ctx, "anna", d.CsoportID, []time.Duration{2 * time.Hour}); !errors.Is(err, app.ErrInvalidInput) {
		t.Fatalf("SetEmlekeztetok with 2h: err = %v, want ErrInvalidInput", err)
	}
	offsets := []time.Duration{15 * time.Minute, 3 * time.Hour, 15 * time.Minute}
	if err := h.App.SetEmlekeztetok(ctx, "anna", d.CsoportID, offsets); err != nil {
		t.Fatalf("SetEmlekeztetok: %v", err)
	}
	c, _ = h.App.Csoport(ctx, "bela", d.CsoportID)
	if !slices.Equal(c.Emlekeztetok, []time.Duration{3 * time.Hour, 15 * time.Minute}) {
		t.Fatalf("emlekeztetok = %v, want 3h and 15m", c.Emlekeztetok)
	}

	runAt(t, h, firstStart.Add(-time.Hour))
	if got := emlekeztetoDMs(h); len(got) != 2 {
		t.Fatalf("emlekeztetok until an hour before = %q, want only the 3-hour ones", got)
	}
	runAt(t, h, firstStart.Add(-15*time.Minute))
	if got := emlekeztetoDMs(h); len(got) != 4 {
		t.Fatalf("emlekeztetok = %q, want the 15-minute ones too", got)
	}

	if err := h.App.SetEmlekeztetok(ctx, "anna", d.CsoportID, nil); err != nil {
		t.Fatalf("SetEmlekeztetok(none): %v", err)
	}
	c, _ = h.App.Csoport(ctx, "bela", d.CsoportID)
	if len(c.Emlekeztetok) != 0 {
		t.Fatalf("emlekeztetok = %v, want none", c.Emlekeztetok)
	}
}

// hataridoWorld has Anna propose three Idopontok with a Hatarido of
// 2026-03-05 20:00 Budapest (19:00 UTC), answered by Bela and Cili.
func hataridoWorld(t *testing.T) (*apptest.Harness, app.EsemenyDetail, time.Time) {
	t.Helper()
	h := apptest.New(t)
	c := csoportOf(t, h, "bela", "cili")
	e, err := h.App.ProposeEsemeny(context.Background(), "anna", c.ID, app.EsemenyInput{
		Title:     "Társasjáték",
		Idopontok: idopontok(3),
		Hatarido:  "2026-03-05T20:00",
	})
	if err != nil {
		t.Fatalf("ProposeEsemeny: %v", err)
	}
	d := detail(t, h, "anna", e.ID)
	answer(t, h, "bela", d, 1, domain.Igen)
	answer(t, h, "cili", d, 1, domain.Igen)
	answer(t, h, "bela", d, 2, domain.Talan)
	answer(t, h, "cili", d, 2, domain.Nem)
	return h, d, time.Date(2026, time.March, 5, 19, 0, 0, 0, time.UTC)
}

func TestTheSzervezoGetsTheRankedHataridoDMWhenItPasses(t *testing.T) {
	h, d, hatarido := hataridoWorld(t)
	if got := detail(t, h, "bela", d.ID).Hatarido; got == nil || !got.Equal(hatarido) {
		t.Fatalf("hatarido = %v, want %v", got, hatarido)
	}

	runAt(t, h, hatarido.Add(-time.Minute))
	if len(hataridoDMs(h)) != 0 {
		t.Fatal("hatarido DM before the hatarido")
	}
	runAt(t, h, hatarido)
	runAt(t, h, hatarido.Add(time.Hour))
	dms := hataridoDMs(h)
	if len(dms) != 1 || dms[0].DiscordID != "discord-anna" {
		t.Fatalf("hatarido DMs = %+v, want one to Anna", dms)
	}
	got := *dms[0].Hatarido
	if got.EsemenyID != d.ID || got.Title != "Társasjáték" || got.CsoportName != "Péntek esték" || !got.ConfirmButtons ||
		got.Link != apptest.WebURL+"/esemenyek/"+d.ID {
		t.Fatalf("hatarido DM = %+v", got)
	}
	type line struct {
		id                     string
		helyezes, igen, ta, ne int
	}
	var lines []line
	for _, l := range got.Idopontok {
		lines = append(lines, line{l.ID, l.Helyezes, l.Igen, l.Talan, l.Nem})
	}
	want := []line{
		{d.Idopontok[1].ID, 1, 2, 0, 0},
		{d.Idopontok[2].ID, 2, 0, 1, 1},
		{d.Idopontok[0].ID, 0, 0, 0, 0},
	}
	if !slices.Equal(lines, want) {
		t.Fatalf("idopontok = %+v, want %+v", lines, want)
	}
	if got.Idopontok[0].Label != "dec. 2. (szerda) 20:00–23:00" {
		t.Fatalf("label = %q", got.Idopontok[0].Label)
	}

	// Voting stays open.
	answer(t, h, "anna", d, 0, domain.Igen)
}

func TestNoHataridoDMOnceTheEsemenyLeftEgyeztetes(t *testing.T) {
	h, d, hatarido := hataridoWorld(t)
	confirm(t, h, "anna", d, 1)
	runAt(t, h, hatarido)
	if len(hataridoDMs(h)) != 0 {
		t.Fatal("hatarido DM for a Veglegesitve Esemeny")
	}

	h2, d2, hatarido2 := hataridoWorld(t)
	cancelEsemeny(t, h2, "anna", d2.ID)
	runAt(t, h2, hatarido2)
	if len(hataridoDMs(h2)) != 0 {
		t.Fatal("hatarido DM for an Elmaradt Esemeny")
	}
}

func TestReopenedBeforeTheHataridoTheDMIsSent(t *testing.T) {
	h, d, hatarido := hataridoWorld(t)
	confirm(t, h, "anna", d, 1)
	reopen(t, h, "anna", d.ID)
	runAt(t, h, hatarido)
	if len(hataridoDMs(h)) != 1 {
		t.Fatalf("hatarido DMs = %d, want 1", len(hataridoDMs(h)))
	}
}

func TestTheSzervezoChangesAndClearsTheHatarido(t *testing.T) {
	h, d, hatarido := hataridoWorld(t)
	ctx := context.Background()

	if err := h.App.SetHatarido(ctx, "bela", d.ID, "2026-03-06T20:00"); !errors.Is(err, app.ErrNotSzervezo) {
		t.Fatalf("SetHatarido by a Tag: err = %v, want ErrNotSzervezo", err)
	}
	if err := h.App.SetHatarido(ctx, "anna", d.ID, "2026-03-01T20:00"); !errors.Is(err, app.ErrHataridoNotFuture) {
		t.Fatalf("SetHatarido in the past: err = %v, want ErrHataridoNotFuture", err)
	}
	if err := h.App.SetHatarido(ctx, "anna", d.ID, "2026-03-06T20:00"); err != nil {
		t.Fatalf("SetHatarido: %v", err)
	}
	later := hatarido.Add(24 * time.Hour)
	if got := detail(t, h, "anna", d.ID).Hatarido; got == nil || !got.Equal(later) {
		t.Fatalf("hatarido = %v, want %v", got, later)
	}
	runAt(t, h, hatarido)
	if len(hataridoDMs(h)) != 0 {
		t.Fatal("the old hatarido still sent a DM")
	}
	runAt(t, h, later)
	if len(hataridoDMs(h)) != 1 {
		t.Fatalf("hatarido DMs = %d, want 1 at the new hatarido", len(hataridoDMs(h)))
	}

	if err := h.App.SetHatarido(ctx, "anna", d.ID, "2026-03-08T20:00"); err != nil {
		t.Fatalf("SetHatarido: %v", err)
	}
	if err := h.App.SetHatarido(ctx, "anna", d.ID, ""); err != nil {
		t.Fatalf("clearing the hatarido: %v", err)
	}
	if got := detail(t, h, "anna", d.ID).Hatarido; got != nil {
		t.Fatalf("hatarido = %v, want none", got)
	}
	runAt(t, h, later.Add(7*24*time.Hour))
	if len(hataridoDMs(h)) != 1 {
		t.Fatal("a cleared hatarido sent a DM")
	}

	confirm(t, h, "anna", d, 0)
	if err := h.App.SetHatarido(ctx, "anna", d.ID, "2026-04-01T20:00"); !errors.Is(err, app.ErrInvalidTransition) {
		t.Fatalf("SetHatarido when Veglegesitve: err = %v, want ErrInvalidTransition", err)
	}
}

func TestAProposalWithAPastHataridoIsRefused(t *testing.T) {
	h := apptest.New(t)
	c := csoportOf(t, h)
	_, err := h.App.ProposeEsemeny(context.Background(), "anna", c.ID, app.EsemenyInput{
		Title: "Társasjáték", Idopontok: idopontok(1), Hatarido: "2026-03-02T09:00",
	})
	if !errors.Is(err, app.ErrHataridoNotFuture) {
		t.Fatalf("err = %v, want ErrHataridoNotFuture", err)
	}
	if list, _ := h.App.EsemenyekOfCsoport(context.Background(), "anna", c.ID); len(list) != 0 {
		t.Fatalf("esemenyek = %d, want none created", len(list))
	}
}

func TestWithMoreThanFiveIdopontokTheHataridoDMHasNoConfirmButtons(t *testing.T) {
	h := apptest.New(t)
	c := csoportOf(t, h)
	e, err := h.App.ProposeEsemeny(context.Background(), "anna", c.ID, app.EsemenyInput{
		Title: "Társasjáték", Idopontok: idopontok(6), Hatarido: "2026-03-05T20:00",
	})
	if err != nil {
		t.Fatalf("ProposeEsemeny: %v", err)
	}
	runAt(t, h, time.Date(2026, time.March, 6, 0, 0, 0, 0, time.UTC))
	dms := hataridoDMs(h)
	if len(dms) != 1 || dms[0].Hatarido.ConfirmButtons || len(dms[0].Hatarido.Idopontok) != 6 ||
		!strings.HasSuffix(dms[0].Hatarido.Link, e.ID) {
		t.Fatalf("hatarido DMs = %+v, want one listing all six with the web link instead of buttons", dms)
	}
}

func TestTheHataridoConfirmButtonConfirmsOnlyForTheSzervezo(t *testing.T) {
	h, d, hatarido := hataridoWorld(t)
	runAt(t, h, hatarido)
	ctx := context.Background()
	idopontID := hataridoDMs(h)[0].Hatarido.Idopontok[0].ID

	// The button calls ConfirmIdopont as whoever pressed it.
	if err := h.App.ConfirmIdopont(ctx, "bela", d.ID, idopontID); !errors.Is(err, app.ErrNotSzervezo) {
		t.Fatalf("confirm by a Tag: err = %v, want ErrNotSzervezo", err)
	}
	if err := h.App.ConfirmIdopont(ctx, "anna", d.ID, idopontID); err != nil {
		t.Fatalf("confirm by the Szervezo: %v", err)
	}
	if got := detail(t, h, "anna", d.ID); got.Allapot != domain.Veglegesitve || got.ConfirmedIdopont != idopontID {
		t.Fatalf("esemeny = %+v, want Veglegesitve on the first-placed Idopont", got.Esemeny)
	}
}

func TestAFailingJobIsRetriedWithBackoffThenGivenUp(t *testing.T) {
	h := apptest.New(t)
	ctx := context.Background()
	c := csoportOf(t, h)
	e := propose(t, h, "anna", c.ID, 1)
	// A job whose kind this build doesn't know always fails.
	if err := h.Store.InsertJob(ctx, store.Job{ID: "j1", Kind: "unknown", EsemenyID: e.ID, Payload: "{}", RunAt: apptest.Start, CreatedAt: apptest.Start}); err != nil {
		t.Fatalf("InsertJob: %v", err)
	}
	now := apptest.Start
	for attempt, wait := range []time.Duration{time.Minute, 5 * time.Minute, 25 * time.Minute, 125 * time.Minute} {
		runAt(t, h, now)
		j, err := h.Store.FindJob(ctx, "j1")
		if err != nil {
			t.Fatalf("FindJob: %v", err)
		}
		if j.Attempts != attempt+1 || j.DoneAt != nil || !j.RunAt.Equal(now.Add(wait)) || !strings.Contains(j.LastError, "unknown job kind") {
			t.Fatalf("after attempt %d: %+v, want a retry %s later", attempt+1, j, wait)
		}
		runAt(t, h, now.Add(wait-time.Second)) // not yet due
		now = now.Add(wait)
	}
	runAt(t, h, now)
	j, _ := h.Store.FindJob(ctx, "j1")
	if j.Attempts != 5 || j.DoneAt == nil {
		t.Fatalf("after the last attempt: %+v, want it given up", j)
	}
	runAt(t, h, now.Add(24*time.Hour))
	if j, _ := h.Store.FindJob(ctx, "j1"); j.Attempts != 5 {
		t.Fatalf("a given-up job ran again: %+v", j)
	}
}
