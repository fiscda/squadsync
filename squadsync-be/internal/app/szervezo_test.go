package app_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"squadsync-be/internal/app"
	"squadsync-be/internal/apptest"
	"squadsync-be/internal/domain"
	"squadsync-be/internal/gateway"
	"squadsync-be/internal/gateway/gatewayfake"
)

// szervezoWorld has Anna (Admin) propose an Esemeny of the Csoport with
// Bela, Cecil and Dani as Tags. Bela and Cecil answered it; Dani didn't.
func szervezoWorld(t *testing.T) (*apptest.Harness, domain.Csoport, app.EsemenyDetail) {
	t.Helper()
	h := apptest.New(t)
	c := csoportOf(t, h, "bela", "cecil", "dani")
	e := propose(t, h, "anna", c.ID, 2)
	d := detail(t, h, "anna", e.ID)
	answer(t, h, "bela", d, 0, domain.Igen)
	answer(t, h, "cecil", d, 1, domain.Talan)
	return h, c, d
}

func handOver(t *testing.T, h *apptest.Harness, from, esemenyID, to string) {
	t.Helper()
	if err := h.App.HandOverSzervezo(context.Background(), from, esemenyID, to); err != nil {
		t.Fatalf("HandOverSzervezo(%s -> %s): %v", from, to, err)
	}
}

func szervezoOf(t *testing.T, h *apptest.Harness, esemenyID string) string {
	t.Helper()
	return detail(t, h, "anna", esemenyID).SzervezoID
}

func szervezoDMs(h *apptest.Harness) []gatewayfake.DM {
	var out []gatewayfake.DM
	for _, dm := range h.Discord.DMs() {
		if dm.Szervezo != nil {
			out = append(out, dm)
		}
	}
	return out
}

func TestTheSzervezoHandsTheRoleToATagWhoAnsweredAndTheyAreToldAndTheMessageRerendered(t *testing.T) {
	h, _, d := szervezoWorld(t)
	edits := len(h.Discord.Edits())

	handOver(t, h, "anna", d.ID, "bela")

	got := detail(t, h, "bela", d.ID)
	if got.SzervezoID != "bela" || got.SzervezoName != "bela" {
		t.Fatalf("szervezo = %s (%s), want bela", got.SzervezoID, got.SzervezoName)
	}
	dms := szervezoDMs(h)
	want := gateway.SzervezoDM{CsoportName: "Péntek esték", Title: "Társasjáték", Link: esemenyURL(d.ID)}
	if len(dms) != 1 || dms[0].DiscordID != "discord-bela" || *dms[0].Szervezo != want {
		t.Fatalf("szervezo DMs = %+v, want one to Bela: %+v", dms, want)
	}
	edited := h.Discord.Edits()[edits:]
	if len(edited) == 0 {
		t.Fatal("the Esemeny's messages weren't re-rendered")
	}
	for _, e := range edited {
		if e.Message.SzervezoName != "bela" {
			t.Fatalf("re-rendered message names the szervezo %q, want bela", e.Message.SzervezoName)
		}
	}
	// The role can go back and forth: Anna answered nothing, so she's out,
	// but Bela can hand it to Cecil.
	handOver(t, h, "bela", d.ID, "cecil")
	if got := szervezoOf(t, h, d.ID); got != "cecil" {
		t.Fatalf("szervezo = %s, want cecil", got)
	}
}

func TestTheRoleOnlyGoesToATagWithAVisszajelzesOnThatEsemeny(t *testing.T) {
	h, c, d := szervezoWorld(t)
	ctx := context.Background()
	addFelhasznalo(t, h, "vendeg", "Vendég")
	addFelhasznalo(t, h, "kivul", "Kívül")
	// A Felhasznalo who answered without being a Tag, as a Vendeg will.
	if err := h.Store.UpsertVisszajelzes(ctx, domain.Visszajelzes{
		IdopontID: d.Idopontok[0].ID, FelhasznaloID: "vendeg", Valasz: domain.Igen, UpdatedAt: apptest.Start,
	}); err != nil {
		t.Fatalf("UpsertVisszajelzes: %v", err)
	}
	// A former Tag whose Visszajelzes stays.
	if err := h.App.LeaveCsoport(ctx, "cecil", c.ID); err != nil {
		t.Fatalf("Cecil leaving: %v", err)
	}
	// A Tag who answered another Esemeny only.
	other := propose(t, h, "anna", c.ID, 1)
	answer(t, h, "dani", detail(t, h, "anna", other.ID), 0, domain.Igen)
	dms := len(h.Discord.DMs())

	for who, name := range map[string]string{
		"dani":   "a Tag without a Visszajelzes on it",
		"vendeg": "a Vendeg",
		"kivul":  "a non-Tag",
		"cecil":  "a former Tag",
		"anna":   "the Szervezo themself",
		"nincs":  "an unknown Felhasznalo",
		"":       "nobody",
	} {
		if err := h.App.HandOverSzervezo(ctx, "anna", d.ID, who); !errors.Is(err, app.ErrNotEligibleSzervezo) {
			t.Errorf("handing over to %s: err = %v, want ErrNotEligibleSzervezo", name, err)
		}
	}
	if got := szervezoOf(t, h, d.ID); got != "anna" {
		t.Fatalf("szervezo = %s after refusals, want anna", got)
	}
	if len(h.Discord.DMs()) != dms {
		t.Fatalf("DMs sent for refused handovers: %+v", h.Discord.DMs()[dms:])
	}
}

func TestOnlyTheSzervezoHandsOverTheRole(t *testing.T) {
	h, _, d := szervezoWorld(t)
	ctx := context.Background()
	addFelhasznalo(t, h, "kivul", "Kívül")

	if err := h.App.HandOverSzervezo(ctx, "bela", d.ID, "cecil"); !errors.Is(err, app.ErrNotSzervezo) {
		t.Fatalf("by another Tag: err = %v, want ErrNotSzervezo", err)
	}
	// An Admin has no authority over an Esemeny they don't organise.
	if err := h.App.SetAdmin(ctx, "anna", d.CsoportID, "dani", true); err != nil {
		t.Fatalf("SetAdmin: %v", err)
	}
	if err := h.App.HandOverSzervezo(ctx, "dani", d.ID, "bela"); !errors.Is(err, app.ErrNotSzervezo) {
		t.Fatalf("by an Admin: err = %v, want ErrNotSzervezo", err)
	}
	if err := h.App.HandOverSzervezo(ctx, "kivul", d.ID, "bela"); !errors.Is(err, app.ErrNotFound) {
		t.Fatalf("by a non-Tag: err = %v, want ErrNotFound", err)
	}
	if err := h.App.HandOverSzervezo(ctx, "anna", "nincs", "bela"); !errors.Is(err, app.ErrNotFound) {
		t.Fatalf("of an unknown Esemeny: err = %v, want ErrNotFound", err)
	}
}

func TestTheRoleCanBeHandedOverInEgyeztetesAndVeglegesitveButNotAfter(t *testing.T) {
	h, _, d := szervezoWorld(t)
	ctx := context.Background()
	confirm(t, h, "anna", d, 0)
	handOver(t, h, "anna", d.ID, "bela") // Veglegesitve

	reopen(t, h, "bela", d.ID)
	handOver(t, h, "bela", d.ID, "cecil") // Egyeztetes again

	for _, allapot := range []domain.Allapot{domain.Elmaradt, domain.Lezajlott} {
		t.Run(string(allapot), func(t *testing.T) {
			h := apptest.New(t)
			c := csoportOf(t, h, "bela")
			insertEsemeny(t, h, c.ID, "anna", allapot)
			id := "esemeny-" + string(allapot) + "-anna"
			if err := h.Store.UpsertVisszajelzes(ctx, domain.Visszajelzes{
				IdopontID: id + "-idopont", FelhasznaloID: "bela", Valasz: domain.Igen, UpdatedAt: apptest.Start,
			}); err != nil {
				t.Fatalf("UpsertVisszajelzes: %v", err)
			}

			if err := h.App.HandOverSzervezo(ctx, "anna", id, "bela"); !errors.Is(err, app.ErrInvalidTransition) {
				t.Fatalf("err = %v, want ErrInvalidTransition", err)
			}
			if got := szervezoOf(t, h, id); got != "anna" {
				t.Fatalf("szervezo = %s, want anna", got)
			}
		})
	}
}

func TestTheNewSzervezoConfirmsAndTheOldOneNoLongerCan(t *testing.T) {
	h, _, d := szervezoWorld(t)
	ctx := context.Background()
	handOver(t, h, "anna", d.ID, "bela")

	if err := h.App.ConfirmIdopont(ctx, "anna", d.ID, d.Idopontok[0].ID); !errors.Is(err, app.ErrNotSzervezo) {
		t.Fatalf("old Szervezo confirming: err = %v, want ErrNotSzervezo", err)
	}
	if err := h.App.HandOverSzervezo(ctx, "anna", d.ID, "cecil"); !errors.Is(err, app.ErrNotSzervezo) {
		t.Fatalf("old Szervezo handing over: err = %v, want ErrNotSzervezo", err)
	}
	confirm(t, h, "bela", d, 0)
	if got := detail(t, h, "anna", d.ID).Allapot; got != domain.Veglegesitve {
		t.Fatalf("allapot = %s, want Veglegesitve", got)
	}
}

func TestDropOutAndHataridoDMsGoToTheNewSzervezo(t *testing.T) {
	t.Run("drop-out", func(t *testing.T) {
		h, _, d := szervezoWorld(t)
		answer(t, h, "cecil", d, 0, domain.Igen)
		confirm(t, h, "anna", d, 0)
		handOver(t, h, "anna", d.ID, "bela")

		answer(t, h, "cecil", d, 0, domain.Nem)

		dms := dropOutDMs(h)
		if len(dms) != 1 || dms[0].DiscordID != "discord-bela" {
			t.Fatalf("drop-out DMs = %+v, want one to Bela", dms)
		}
	})
	t.Run("hatarido", func(t *testing.T) {
		h, d, hatarido := hataridoWorld(t)
		handOver(t, h, "anna", d.ID, "bela")

		runAt(t, h, hatarido)

		dms := hataridoDMs(h)
		if len(dms) != 1 || dms[0].DiscordID != "discord-bela" {
			t.Fatalf("hatarido DMs = %+v, want one to Bela", dms)
		}
	})
	t.Run("the new Szervezo dropping out needs no DM", func(t *testing.T) {
		h, _, d := szervezoWorld(t)
		confirm(t, h, "anna", d, 0)
		handOver(t, h, "anna", d.ID, "bela")

		answer(t, h, "bela", d, 0, domain.Nem)

		if dms := dropOutDMs(h); len(dms) != 0 {
			t.Fatalf("drop-out DMs = %+v, want none", dms)
		}
	})
}

func TestAMutedNewSzervezoGetsNoHandoverDM(t *testing.T) {
	h, c, d := szervezoWorld(t)
	if err := h.App.SetMuted(context.Background(), "bela", c.ID, true); err != nil {
		t.Fatalf("SetMuted: %v", err)
	}

	handOver(t, h, "anna", d.ID, "bela")

	if dms := szervezoDMs(h); len(dms) != 0 {
		t.Fatalf("szervezo DMs = %+v, want none for a muted Tag", dms)
	}
}

func TestASzervezoCantLeaveUntilTheyHandOverOrCancel(t *testing.T) {
	h, c, _ := szervezoWorld(t)
	ctx := context.Background()
	// Bela organises two active Esemenyek and an Elmaradt one.
	h.Clock.Advance(time.Minute) // Esemenyek are listed by their creation time
	first := propose(t, h, "bela", c.ID, 1)
	h.Clock.Advance(time.Minute)
	second := propose(t, h, "bela", c.ID, 1)
	h.Clock.Advance(time.Minute)
	third := propose(t, h, "bela", c.ID, 1)
	cancelEsemeny(t, h, "bela", third.ID)
	answer(t, h, "anna", detail(t, h, "anna", first.ID), 0, domain.Igen)
	answer(t, h, "cecil", detail(t, h, "anna", second.ID), 0, domain.Igen)
	confirm(t, h, "bela", detail(t, h, "bela", second.ID), 0)

	err := h.App.LeaveCsoport(ctx, "bela", c.ID)
	var blocked *app.SzervezoOfActiveEsemenyError
	if !errors.Is(err, app.ErrSzervezoOfActiveEsemeny) || !errors.As(err, &blocked) {
		t.Fatalf("leaving: err = %v, want ErrSzervezoOfActiveEsemeny", err)
	}
	if len(blocked.Esemenyek) != 2 || blocked.Esemenyek[0].ID != first.ID || blocked.Esemenyek[1].ID != second.ID ||
		blocked.Esemenyek[1].Allapot != domain.Veglegesitve {
		t.Fatalf("blocking esemenyek = %+v, want the two active ones, oldest first", blocked.Esemenyek)
	}

	handOver(t, h, "bela", first.ID, "anna")
	if err := h.App.LeaveCsoport(ctx, "bela", c.ID); !errors.Is(err, app.ErrSzervezoOfActiveEsemeny) {
		t.Fatalf("leaving with one left: err = %v, want ErrSzervezoOfActiveEsemeny", err)
	}
	// Cancelling is the other way out.
	cancelEsemeny(t, h, "bela", second.ID)
	if err := h.App.LeaveCsoport(ctx, "bela", c.ID); err != nil {
		t.Fatalf("leaving after handing over and cancelling: %v", err)
	}
	if isTag(t, h, "bela", c.ID) {
		t.Fatal("Bela is still a Tag")
	}
	// The Esemeny they handed over keeps running under its new Szervezo.
	if got := szervezoOf(t, h, first.ID); got != "anna" {
		t.Fatalf("szervezo = %s, want anna", got)
	}
}

func TestLeavingAfterAHandoverIsAllowed(t *testing.T) {
	h, c, _ := szervezoWorld(t)
	e := propose(t, h, "bela", c.ID, 1)
	answer(t, h, "cecil", detail(t, h, "bela", e.ID), 0, domain.Igen)

	handOver(t, h, "bela", e.ID, "cecil")

	if err := h.App.LeaveCsoport(context.Background(), "bela", c.ID); err != nil {
		t.Fatalf("leaving: %v", err)
	}
}

func TestSzervezoEsemenyekListsTheActiveOnesWithTheirSuccessors(t *testing.T) {
	h, c, d := szervezoWorld(t)
	ctx := context.Background()
	h.Clock.Advance(time.Minute) // Esemenyek are listed by their creation time
	over := propose(t, h, "anna", c.ID, 1)
	cancelEsemeny(t, h, "anna", over.ID)
	h.Clock.Advance(time.Minute)
	alone := propose(t, h, "anna", c.ID, 1)
	addFelhasznalo(t, h, "kivul", "Kívül")

	list, err := h.App.SzervezoEsemenyek(ctx, "anna", c.ID, "anna")
	if err != nil {
		t.Fatalf("SzervezoEsemenyek: %v", err)
	}
	if len(list) != 2 || list[0].ID != d.ID || list[1].ID != alone.ID {
		t.Fatalf("list = %+v, want the two active Esemenyek, oldest first", list)
	}
	names := func(tags []domain.TagWithName) []string {
		out := []string{}
		for _, t := range tags {
			out = append(out, t.FelhasznaloID)
		}
		return out
	}
	if got := names(list[0].Utodok); len(got) != 2 || got[0] != "bela" || got[1] != "cecil" {
		t.Fatalf("successors = %v, want bela and cecil", got)
	}
	if len(list[1].Utodok) != 0 {
		t.Fatalf("successors of an unanswered Esemeny = %v, want none", names(list[1].Utodok))
	}

	// Bela may see their own, an Admin anyone's; no other Tag, no outsider.
	if list, err := h.App.SzervezoEsemenyek(ctx, "bela", c.ID, "bela"); err != nil || len(list) != 0 {
		t.Fatalf("own list = %+v, %v, want an empty one", list, err)
	}
	if _, err := h.App.SzervezoEsemenyek(ctx, "bela", c.ID, "anna"); !errors.Is(err, app.ErrNotAdmin) {
		t.Fatalf("another Tag's: err = %v, want ErrNotAdmin", err)
	}
	if _, err := h.App.SzervezoEsemenyek(ctx, "kivul", c.ID, "kivul"); !errors.Is(err, app.ErrNotFound) {
		t.Fatalf("outsider: err = %v, want ErrNotFound", err)
	}
	if _, err := h.App.SzervezoEsemenyek(ctx, "anna", c.ID, "kivul"); !errors.Is(err, app.ErrNotFound) {
		t.Fatalf("a non-Tag target: err = %v, want ErrNotFound", err)
	}
}

// removalWorld has Bela organise two Esemenyek of the Csoport (the second one
// Veglegesitve), answered by Anna and Cecil; Dani answered neither.
func removalWorld(t *testing.T) (h *apptest.Harness, csoport domain.Csoport, first, second domain.Esemeny) {
	t.Helper()
	h = apptest.New(t)
	csoport = csoportOf(t, h, "bela", "cecil", "dani")
	first = propose(t, h, "bela", csoport.ID, 1)
	h.Clock.Advance(time.Minute) // Esemenyek are handled by their creation time
	second = propose(t, h, "bela", csoport.ID, 1)
	for _, e := range []domain.Esemeny{first, second} {
		d := detail(t, h, "bela", e.ID)
		answer(t, h, "anna", d, 0, domain.Igen)
		answer(t, h, "cecil", d, 0, domain.Igen)
	}
	confirm(t, h, "bela", detail(t, h, "bela", second.ID), 0)
	return h, csoport, first, second
}

// expectUntouched checks that a refused removal changed nothing.
func expectUntouched(t *testing.T, h *apptest.Harness, csoport domain.Csoport, first, second domain.Esemeny, dms int) {
	t.Helper()
	if !isTag(t, h, "bela", csoport.ID) {
		t.Fatal("Bela lost their Tag despite the refusal")
	}
	for _, e := range []domain.Esemeny{first, second} {
		if got := szervezoOf(t, h, e.ID); got != "bela" {
			t.Fatalf("szervezo of %s = %s after the refusal, want bela", e.ID, got)
		}
	}
	if got := len(h.Discord.DMs()); got != dms {
		t.Fatalf("DMs after the refusal = %+v, want none sent", h.Discord.DMs()[dms:])
	}
}

func TestAnAdminRemovingASzervezoPicksANewOneForEachEsemeny(t *testing.T) {
	h, c, first, second := removalWorld(t)
	ctx := context.Background()
	edits := len(h.Discord.Edits())

	err := h.App.RemoveTag(ctx, "anna", c.ID, "bela", map[string]string{first.ID: "cecil", second.ID: "anna"})
	if err != nil {
		t.Fatalf("RemoveTag: %v", err)
	}

	if isTag(t, h, "bela", c.ID) {
		t.Fatal("Bela is still a Tag")
	}
	if got := szervezoOf(t, h, first.ID); got != "cecil" {
		t.Fatalf("first szervezo = %s, want cecil", got)
	}
	if got := szervezoOf(t, h, second.ID); got != "anna" {
		t.Fatalf("second szervezo = %s, want anna", got)
	}
	dms := szervezoDMs(h)
	if len(dms) != 2 || dms[0].DiscordID != "discord-cecil" || dms[1].DiscordID != "discord-anna" {
		t.Fatalf("szervezo DMs = %+v, want one each to Cecil and Anna", dms)
	}
	if dms[0].Szervezo.Link != esemenyURL(first.ID) || dms[1].Szervezo.Link != esemenyURL(second.ID) {
		t.Fatalf("szervezo DMs = %+v, want links to their Esemenyek", dms)
	}
	if len(h.Discord.Edits()) == edits {
		t.Fatal("the Esemenyek's messages weren't re-rendered")
	}
	// The new Szervezo can act at once.
	confirm(t, h, "cecil", detail(t, h, "cecil", first.ID), 0)
}

func TestRemovingASzervezoStillReleasesTheirClaims(t *testing.T) {
	h := apptest.New(t)
	ctx := context.Background()
	c := csoportOf(t, h, "bela", "cecil")
	e := propose(t, h, "bela", c.ID, 1)
	switchKiegeszito(t, h, "bela", e.ID, domain.Felszereles, true)
	sator := addTetel(t, h, "bela", e.ID, "sátor", 3)
	d := detail(t, h, "bela", e.ID)
	answer(t, h, "bela", d, 0, domain.Igen)
	answer(t, h, "cecil", d, 0, domain.Igen)
	confirm(t, h, "bela", d, 0)
	vallal(t, h, "bela", e.ID, sator.ID, 2)
	vallal(t, h, "cecil", e.ID, sator.ID, 1)

	if err := h.App.RemoveTag(ctx, "anna", c.ID, "bela", map[string]string{e.ID: "cecil"}); err != nil {
		t.Fatalf("RemoveTag: %v", err)
	}

	expectVallalasok(t, felszereles(t, h, "anna", e.ID), "sátor:cecil=1")
	if got := szervezoOf(t, h, e.ID); got != "cecil" {
		t.Fatalf("szervezo = %s, want cecil", got)
	}
	if dms := dropOutDMs(h); len(dms) != 0 {
		t.Fatalf("drop-out DMs = %+v, want none", dms)
	}
}

func TestRemovingASzervezoWithoutAPickForEachEsemenyChangesNothing(t *testing.T) {
	h, c, first, second := removalWorld(t)
	ctx := context.Background()
	dms := len(h.Discord.DMs())

	for name, picks := range map[string]map[string]string{
		"no picks":     nil,
		"empty picks":  {},
		"one missing":  {first.ID: "cecil"},
		"a blank pick": {first.ID: "cecil", second.ID: ""},
	} {
		err := h.App.RemoveTag(ctx, "anna", c.ID, "bela", picks)
		var blocked *app.SzervezoOfActiveEsemenyError
		if !errors.Is(err, app.ErrSzervezoOfActiveEsemeny) || !errors.As(err, &blocked) {
			t.Fatalf("%s: err = %v, want ErrSzervezoOfActiveEsemeny", name, err)
		}
		expectUntouched(t, h, c, first, second, dms)
	}

	err := h.App.RemoveTag(ctx, "anna", c.ID, "bela", map[string]string{first.ID: "cecil"})
	var blocked *app.SzervezoOfActiveEsemenyError
	if !errors.As(err, &blocked) || len(blocked.Esemenyek) != 1 || blocked.Esemenyek[0].ID != second.ID {
		t.Fatalf("err = %v, want it to name the second Esemeny", err)
	}
}

func TestRemovingASzervezoWithAnInvalidPickChangesNothing(t *testing.T) {
	h, c, first, second := removalWorld(t)
	ctx := context.Background()
	addFelhasznalo(t, h, "kivul", "Kívül")
	dms := len(h.Discord.DMs())

	for name, bad := range map[string]string{
		"a Tag without a Visszajelzes": "dani",
		"the removed Tag":              "bela",
		"a non-Tag":                    "kivul",
		"an unknown Felhasznalo":       "nincs",
	} {
		// The first pick is fine, so only a transaction keeps it from sticking.
		err := h.App.RemoveTag(ctx, "anna", c.ID, "bela", map[string]string{first.ID: "cecil", second.ID: bad})
		if !errors.Is(err, app.ErrNotEligibleSzervezo) {
			t.Fatalf("%s: err = %v, want ErrNotEligibleSzervezo", name, err)
		}
		expectUntouched(t, h, c, first, second, dms)
	}
}

func TestRemovingASzervezoWhoseEsemenyHasNobodyToTakeItIsRefusedByName(t *testing.T) {
	h, c, first, second := removalWorld(t)
	ctx := context.Background()
	lonely := propose(t, h, "bela", c.ID, 1) // nobody answered it
	dms := len(h.Discord.DMs())

	err := h.App.RemoveTag(ctx, "anna", c.ID, "bela", map[string]string{first.ID: "cecil", second.ID: "anna", lonely.ID: "cecil"})

	var none *app.NoEligibleSzervezoError
	if !errors.Is(err, app.ErrNoEligibleSzervezo) || !errors.As(err, &none) || none.Esemeny.ID != lonely.ID || none.Esemeny.Title != "Társasjáték" {
		t.Fatalf("err = %v, want ErrNoEligibleSzervezo naming the lonely Esemeny", err)
	}
	expectUntouched(t, h, c, first, second, dms)
	if got := szervezoOf(t, h, lonely.ID); got != "bela" {
		t.Fatalf("szervezo of the lonely one = %s, want bela", got)
	}

	// The Szervezo cancelling it frees the way, and the Admin can't do that.
	if err := h.App.CancelEsemeny(ctx, "anna", lonely.ID); !errors.Is(err, app.ErrNotSzervezo) {
		t.Fatalf("Admin cancelling: err = %v, want ErrNotSzervezo", err)
	}
	cancelEsemeny(t, h, "bela", lonely.ID)
	if err := h.App.RemoveTag(ctx, "anna", c.ID, "bela", map[string]string{first.ID: "cecil", second.ID: "anna"}); err != nil {
		t.Fatalf("RemoveTag after cancelling: %v", err)
	}
}

func TestRemovingASzervezoIsRefusedAsAWholeWhenTheRemovalItselfIsRefused(t *testing.T) {
	h := apptest.New(t)
	ctx := context.Background()
	// Anna is the only Admin and the Szervezo of an Esemeny Bela answered.
	c := csoportOf(t, h, "bela")
	e := propose(t, h, "anna", c.ID, 1)
	answer(t, h, "bela", detail(t, h, "anna", e.ID), 0, domain.Igen)

	err := h.App.RemoveTag(ctx, "anna", c.ID, "anna", map[string]string{e.ID: "bela"})

	if !errors.Is(err, app.ErrLastAdmin) {
		t.Fatalf("err = %v, want ErrLastAdmin", err)
	}
	if got := szervezoOf(t, h, e.ID); got != "anna" {
		t.Fatalf("szervezo = %s, want the handover rolled back to anna", got)
	}
	if dms := szervezoDMs(h); len(dms) != 0 {
		t.Fatalf("szervezo DMs = %+v, want none", dms)
	}
}

func TestPicksForOtherEsemenyekAreIgnoredAndOnlyAdminsRemove(t *testing.T) {
	h, c, first, second := removalWorld(t)
	ctx := context.Background()
	other, err := h.App.CreateCsoport(ctx, "anna", "Másik")
	if err != nil {
		t.Fatalf("CreateCsoport: %v", err)
	}
	foreign := propose(t, h, "anna", other.ID, 1)
	picks := map[string]string{first.ID: "cecil", second.ID: "anna", foreign.ID: "cecil", "nincs": "cecil"}

	if err := h.App.RemoveTag(ctx, "cecil", c.ID, "bela", picks); !errors.Is(err, app.ErrNotAdmin) {
		t.Fatalf("by a plain Tag: err = %v, want ErrNotAdmin", err)
	}
	if got := szervezoOf(t, h, first.ID); got != "bela" {
		t.Fatalf("szervezo = %s after a refused removal, want bela", got)
	}
	if err := h.App.RemoveTag(ctx, "anna", c.ID, "bela", picks); err != nil {
		t.Fatalf("RemoveTag: %v", err)
	}
	if got := szervezoOf(t, h, foreign.ID); got != "anna" {
		t.Fatalf("szervezo of the other Csoport's Esemeny = %s, want it untouched", got)
	}
}

func TestTheHataridoJobReadsTheSzervezoWhenItRunsNotWhenItWasScheduled(t *testing.T) {
	// A handover after proposing, long before the Hatarido, still counts.
	h, d, hatarido := hataridoWorld(t)
	h.Clock.Advance(time.Hour)
	handOver(t, h, "anna", d.ID, "cili")
	handOver(t, h, "cili", d.ID, "bela")

	runAt(t, h, hatarido)

	if dms := hataridoDMs(h); len(dms) != 1 || dms[0].DiscordID != "discord-bela" {
		t.Fatalf("hatarido DMs = %+v, want one to Bela", dms)
	}
}
