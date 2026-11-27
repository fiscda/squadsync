package app_test

import (
	"context"
	"errors"
	"sync"
	"testing"

	"squadsync-be/internal/app"
	"squadsync-be/internal/apptest"
	"squadsync-be/internal/domain"
	"squadsync-be/internal/gateway"
	"squadsync-be/internal/gateway/gatewayfake"
)

func confirm(t *testing.T, h *apptest.Harness, felhasznaloID string, d app.EsemenyDetail, idopont int) {
	t.Helper()
	if err := h.App.ConfirmIdopont(context.Background(), felhasznaloID, d.ID, d.Idopontok[idopont].ID); err != nil {
		t.Fatalf("ConfirmIdopont(%s, %d): %v", felhasznaloID, idopont, err)
	}
}

func reopen(t *testing.T, h *apptest.Harness, felhasznaloID, esemenyID string) {
	t.Helper()
	if err := h.App.ReopenEgyeztetes(context.Background(), felhasznaloID, esemenyID); err != nil {
		t.Fatalf("ReopenEgyeztetes(%s): %v", felhasznaloID, err)
	}
}

func cancelEsemeny(t *testing.T, h *apptest.Harness, felhasznaloID, esemenyID string) {
	t.Helper()
	if err := h.App.CancelEsemeny(context.Background(), felhasznaloID, esemenyID); err != nil {
		t.Fatalf("CancelEsemeny(%s): %v", felhasznaloID, err)
	}
}

func resztvevoIDs(d app.EsemenyDetail) []string {
	ids := []string{}
	for _, r := range d.Resztvevok {
		ids = append(ids, r.FelhasznaloID)
	}
	return ids
}

func expectResztvevok(t *testing.T, d app.EsemenyDetail, want ...string) {
	t.Helper()
	got := resztvevoIDs(d)
	if len(got) != len(want) {
		t.Fatalf("resztvevok = %v, want %v", got, want)
	}
	for i := range got {
		if got[i] != want[i] {
			t.Fatalf("resztvevok = %v, want %v", got, want)
		}
	}
}

func answerables(d app.EsemenyDetail) []bool {
	out := make([]bool, len(d.Idopontok))
	for i, a := range d.Idopontok {
		out[i] = a.Answerable
	}
	return out
}

func expectAnswerable(t *testing.T, h *apptest.Harness, esemenyID string, want ...bool) {
	t.Helper()
	d := detail(t, h, "bela", esemenyID)
	got := answerables(d)
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("answerable = %v, want %v", got, want)
		}
		err := h.App.SetVisszajelzes(context.Background(), "bela", esemenyID, d.Idopontok[i].ID, domain.Talan)
		if want[i] && err != nil {
			t.Fatalf("answering idopont %d: %v, want it accepted", i, err)
		}
		if !want[i] && !errors.Is(err, app.ErrIdopontNotAnswerable) {
			t.Fatalf("answering idopont %d: err = %v, want ErrIdopontNotAnswerable", i, err)
		}
	}
}

func TestSzervezoConfirmsAnyIdopontNotOnlyTheFirstPlaced(t *testing.T) {
	h := apptest.New(t)
	c := csoportOf(t, h, "bela")
	e := propose(t, h, "anna", c.ID, 3)
	d := detail(t, h, "anna", e.ID)
	answer(t, h, "bela", d, 0, domain.Igen)
	answer(t, h, "anna", d, 0, domain.Igen)
	answer(t, h, "bela", d, 2, domain.Talan)

	confirm(t, h, "anna", d, 2) // 2nd place

	got := detail(t, h, "bela", e.ID)
	if got.Allapot != domain.Veglegesitve || got.ConfirmedIdopont != d.Idopontok[2].ID {
		t.Fatalf("esemeny = %+v, want Veglegesitve with the third Idopont", got.Esemeny)
	}
	if got.Idopontok[0].Confirmed || got.Idopontok[1].Confirmed || !got.Idopontok[2].Confirmed {
		t.Fatalf("confirmed flags = %v %v %v", got.Idopontok[0].Confirmed, got.Idopontok[1].Confirmed, got.Idopontok[2].Confirmed)
	}
}

func TestOnlyTheSzervezoConfirmsReopensOrCancels(t *testing.T) {
	h := apptest.New(t)
	ctx := context.Background()
	c := csoportOf(t, h, "bela")
	addFelhasznalo(t, h, "dani", "Dani") // not a Tag
	e := propose(t, h, "anna", c.ID, 2)
	d := detail(t, h, "anna", e.ID)
	idopontID := d.Idopontok[0].ID

	for who, want := range map[string]error{"bela": app.ErrNotSzervezo, "dani": app.ErrNotFound} {
		if err := h.App.ConfirmIdopont(ctx, who, e.ID, idopontID); !errors.Is(err, want) {
			t.Errorf("ConfirmIdopont by %s: err = %v, want %v", who, err, want)
		}
		if err := h.App.CancelEsemeny(ctx, who, e.ID); !errors.Is(err, want) {
			t.Errorf("CancelEsemeny by %s: err = %v, want %v", who, err, want)
		}
	}
	confirm(t, h, "anna", d, 0)
	for who, want := range map[string]error{"bela": app.ErrNotSzervezo, "dani": app.ErrNotFound} {
		if err := h.App.ReopenEgyeztetes(ctx, who, e.ID); !errors.Is(err, want) {
			t.Errorf("ReopenEgyeztetes by %s: err = %v, want %v", who, err, want)
		}
		if err := h.App.CancelEsemeny(ctx, who, e.ID); !errors.Is(err, want) {
			t.Errorf("CancelEsemeny by %s: err = %v, want %v", who, err, want)
		}
	}
	if got := detail(t, h, "anna", e.ID); got.Allapot != domain.Veglegesitve {
		t.Fatalf("allapot = %s, want the Szervezo's confirmation to stand", got.Allapot)
	}
}

func TestEveryTransitionValidAndInvalid(t *testing.T) {
	ctx := context.Background()
	type step func(h *apptest.Harness, d app.EsemenyDetail) error
	confirmFirst := func(h *apptest.Harness, d app.EsemenyDetail) error {
		return h.App.ConfirmIdopont(ctx, "anna", d.ID, d.Idopontok[0].ID)
	}
	reopenIt := func(h *apptest.Harness, d app.EsemenyDetail) error {
		return h.App.ReopenEgyeztetes(ctx, "anna", d.ID)
	}
	cancelIt := func(h *apptest.Harness, d app.EsemenyDetail) error {
		return h.App.CancelEsemeny(ctx, "anna", d.ID)
	}

	for name, tc := range map[string]struct {
		before []step
		do     step
		want   error
		after  domain.Allapot
	}{
		"confirm in Egyeztetes":       {nil, confirmFirst, nil, domain.Veglegesitve},
		"confirm in Veglegesitve":     {[]step{confirmFirst}, confirmFirst, app.ErrInvalidTransition, domain.Veglegesitve},
		"confirm in Elmaradt":         {[]step{cancelIt}, confirmFirst, app.ErrInvalidTransition, domain.Elmaradt},
		"reopen in Egyeztetes":        {nil, reopenIt, app.ErrInvalidTransition, domain.Egyeztetes},
		"reopen in Veglegesitve":      {[]step{confirmFirst}, reopenIt, nil, domain.Egyeztetes},
		"reopen in Elmaradt":          {[]step{confirmFirst, cancelIt}, reopenIt, app.ErrInvalidTransition, domain.Elmaradt},
		"cancel in Egyeztetes":        {nil, cancelIt, nil, domain.Elmaradt},
		"cancel in Veglegesitve":      {[]step{confirmFirst}, cancelIt, nil, domain.Elmaradt},
		"cancel in Elmaradt":          {[]step{cancelIt}, cancelIt, app.ErrInvalidTransition, domain.Elmaradt},
		"confirm again after reopen":  {[]step{confirmFirst, reopenIt}, confirmFirst, nil, domain.Veglegesitve},
		"cancel after reopen":         {[]step{confirmFirst, reopenIt}, cancelIt, nil, domain.Elmaradt},
		"reopen twice":                {[]step{confirmFirst, reopenIt}, reopenIt, app.ErrInvalidTransition, domain.Egyeztetes},
		"confirm after re-confirming": {[]step{confirmFirst, reopenIt, confirmFirst}, confirmFirst, app.ErrInvalidTransition, domain.Veglegesitve},
	} {
		t.Run(name, func(t *testing.T) {
			h := apptest.New(t)
			c := csoportOf(t, h, "bela")
			e := propose(t, h, "anna", c.ID, 2)
			d := detail(t, h, "anna", e.ID)
			for _, s := range tc.before {
				if err := s(h, d); err != nil {
					t.Fatalf("setup: %v", err)
				}
			}
			if err := tc.do(h, d); !errors.Is(err, tc.want) {
				t.Fatalf("err = %v, want %v", err, tc.want)
			}
			if got := detail(t, h, "anna", e.ID).Allapot; got != tc.after {
				t.Fatalf("allapot = %s, want %s", got, tc.after)
			}
		})
	}
}

func TestOfTwoConcurrentConfirmationsExactlyOneWins(t *testing.T) {
	h := apptest.New(t)
	c := csoportOf(t, h, "bela")
	e := propose(t, h, "anna", c.ID, 2)
	d := detail(t, h, "anna", e.ID)

	errs := make([]error, 2)
	var wg sync.WaitGroup
	start := make(chan struct{})
	for n := range errs {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			errs[n] = h.App.ConfirmIdopont(context.Background(), "anna", e.ID, d.Idopontok[n].ID)
		}()
	}
	close(start)
	wg.Wait()

	won := -1
	for n, err := range errs {
		switch {
		case err == nil:
			if won != -1 {
				t.Fatalf("both confirmations succeeded")
			}
			won = n
		case !errors.Is(err, app.ErrInvalidTransition):
			t.Fatalf("confirmation %d: err = %v, want nil or ErrInvalidTransition", n, err)
		}
	}
	if won == -1 {
		t.Fatalf("neither confirmation succeeded: %v", errs)
	}
	if got := detail(t, h, "anna", e.ID); got.ConfirmedIdopont != d.Idopontok[won].ID {
		t.Fatalf("confirmed = %q, want the winner's %q", got.ConfirmedIdopont, d.Idopontok[won].ID)
	}
	if anns := len(h.Discord.Announcements()); anns != 0 {
		// No Csatorna: the winner's announcement went by DM, once.
		t.Fatalf("channel announcements = %d, want none", anns)
	}
	var announced int
	for _, dm := range h.Discord.DMsTo("discord-bela") {
		if dm.Announcement != nil {
			announced++
		}
	}
	if announced != 1 {
		t.Fatalf("Bela got %d announcements, want exactly one", announced)
	}
}

func TestConfirmingAnIdopontOfAnotherEsemenyIsNotFound(t *testing.T) {
	h := apptest.New(t)
	c := csoportOf(t, h)
	e := propose(t, h, "anna", c.ID, 1)
	other := detail(t, h, "anna", propose(t, h, "anna", c.ID, 1).ID)

	err := h.App.ConfirmIdopont(context.Background(), "anna", e.ID, other.Idopontok[0].ID)
	if !errors.Is(err, app.ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}
	if err := h.App.ConfirmIdopont(context.Background(), "anna", e.ID, "no-such-idopont"); !errors.Is(err, app.ErrNotFound) {
		t.Fatalf("unknown idopont: err = %v, want ErrNotFound", err)
	}
}

func TestAnswerableIdopontokInEachAllapot(t *testing.T) {
	h := apptest.New(t)
	c := csoportOf(t, h, "bela")
	e := propose(t, h, "anna", c.ID, 3)
	d := detail(t, h, "anna", e.ID)

	expectAnswerable(t, h, e.ID, true, true, true) // Egyeztetes

	confirm(t, h, "anna", d, 1)
	expectAnswerable(t, h, e.ID, false, true, false) // Veglegesitve

	// Lezajlott is set by a timed job (#9); set it directly here.
	if err := h.Store.UpdateEsemenyAllapot(context.Background(), e.ID, domain.Lezajlott, d.Idopontok[1].ID); err != nil {
		t.Fatalf("UpdateEsemenyAllapot: %v", err)
	}
	expectAnswerable(t, h, e.ID, false, false, false)

	h2 := apptest.New(t)
	c2 := csoportOf(t, h2, "bela")
	e2 := propose(t, h2, "anna", c2.ID, 2)
	cancelEsemeny(t, h2, "anna", e2.ID)
	expectAnswerable(t, h2, e2.ID, false, false) // Elmaradt
}

func TestATagWhoNeverAnsweredCanStillJoinTheConfirmedIdopont(t *testing.T) {
	h := apptest.New(t)
	c := csoportOf(t, h, "bela", "cecil")
	e := propose(t, h, "anna", c.ID, 2)
	d := detail(t, h, "anna", e.ID)
	answer(t, h, "bela", d, 0, domain.Igen)
	confirm(t, h, "anna", d, 0)

	answer(t, h, "cecil", d, 0, domain.Igen)

	expectResztvevok(t, detail(t, h, "anna", e.ID), "bela", "cecil")
}

func TestResztvevokAreTheTagsWithIgenOnTheConfirmedIdopont(t *testing.T) {
	h := apptest.New(t)
	c := csoportOf(t, h, "bela", "cecil", "dani", "edit")
	e := propose(t, h, "anna", c.ID, 2)
	d := detail(t, h, "anna", e.ID)
	answer(t, h, "bela", d, 1, domain.Igen)
	answer(t, h, "cecil", d, 1, domain.Talan)
	answer(t, h, "dani", d, 1, domain.Nem)
	answer(t, h, "edit", d, 0, domain.Igen) // igen, but on the other Idopont
	answer(t, h, "anna", d, 1, domain.Igen)

	expectResztvevok(t, detail(t, h, "anna", e.ID)) // none before Veglegesitve

	confirm(t, h, "anna", d, 1)
	got := detail(t, h, "cecil", e.ID)
	expectResztvevok(t, got, "anna", "bela") // ordered by name
	if r := got.Resztvevok[1]; r.Name != "bela" || !r.IsTag {
		t.Fatalf("resztvevo = %+v", r)
	}

	// Lezajlott keeps them (the Elszamolas runs among them).
	if err := h.Store.UpdateEsemenyAllapot(context.Background(), e.ID, domain.Lezajlott, d.Idopontok[1].ID); err != nil {
		t.Fatalf("UpdateEsemenyAllapot: %v", err)
	}
	expectResztvevok(t, detail(t, h, "anna", e.ID), "anna", "bela")
}

func TestResztvevokAreGoneOnceReopenedOrElmaradt(t *testing.T) {
	h := apptest.New(t)
	c := csoportOf(t, h, "bela")
	e := propose(t, h, "anna", c.ID, 1)
	d := detail(t, h, "anna", e.ID)
	answer(t, h, "bela", d, 0, domain.Igen)
	confirm(t, h, "anna", d, 0)
	expectResztvevok(t, detail(t, h, "anna", e.ID), "bela")

	reopen(t, h, "anna", e.ID)
	expectResztvevok(t, detail(t, h, "anna", e.ID))

	confirm(t, h, "anna", d, 0)
	expectResztvevok(t, detail(t, h, "anna", e.ID), "bela")

	cancelEsemeny(t, h, "anna", e.ID)
	got := detail(t, h, "anna", e.ID)
	expectResztvevok(t, got)
	if got.Idopontok[0].Confirmed || got.ConfirmedIdopont != "" {
		t.Fatal("an Elmaradt Esemeny still shows a confirmed Idopont")
	}
}

func TestReopeningKeepsEveryVisszajelzes(t *testing.T) {
	h := apptest.New(t)
	c := csoportOf(t, h, "bela")
	e := propose(t, h, "anna", c.ID, 2)
	d := detail(t, h, "anna", e.ID)
	answer(t, h, "bela", d, 0, domain.Igen)
	answer(t, h, "bela", d, 1, domain.Nem)
	answer(t, h, "anna", d, 1, domain.Talan)
	confirm(t, h, "anna", d, 1)

	reopen(t, h, "anna", e.ID)

	got := detail(t, h, "bela", e.ID)
	if got.Allapot != domain.Egyeztetes || got.ConfirmedIdopont != "" {
		t.Fatalf("esemeny = %+v, want Egyeztetes without a confirmed Idopont", got.Esemeny)
	}
	if got.Idopontok[0].Sajat != domain.Igen || got.Idopontok[1].Sajat != domain.Nem || got.Idopontok[1].Talan != 1 {
		t.Fatalf("idopontok = %+v, want every Visszajelzes kept", got.Idopontok)
	}
	expectHelyezesek(t, got, 1, 2)
	expectAnswerable(t, h, e.ID, true, true)
}

func TestElmaradtIsFinal(t *testing.T) {
	h := apptest.New(t)
	ctx := context.Background()
	c := csoportOf(t, h, "bela")
	e := propose(t, h, "anna", c.ID, 2)
	d := detail(t, h, "anna", e.ID)
	confirm(t, h, "anna", d, 0)
	cancelEsemeny(t, h, "anna", e.ID)

	for name, err := range map[string]error{
		"confirm": h.App.ConfirmIdopont(ctx, "anna", e.ID, d.Idopontok[1].ID),
		"reopen":  h.App.ReopenEgyeztetes(ctx, "anna", e.ID),
		"cancel":  h.App.CancelEsemeny(ctx, "anna", e.ID),
	} {
		if !errors.Is(err, app.ErrInvalidTransition) {
			t.Errorf("%s: err = %v, want ErrInvalidTransition", name, err)
		}
	}
	got := detail(t, h, "anna", e.ID)
	if got.Allapot != domain.Elmaradt {
		t.Fatalf("allapot = %s, want Elmaradt", got.Allapot)
	}
	expectAnswerable(t, h, e.ID, false, false)
}

func dropOutDMs(h *apptest.Harness) []gatewayfake.DM {
	var out []gatewayfake.DM
	for _, dm := range h.Discord.DMs() {
		if dm.DropOut != nil {
			out = append(out, dm)
		}
	}
	return out
}

func TestDroppingFromIgenOnTheConfirmedIdopontSendsTheSzervezoADM(t *testing.T) {
	for _, to := range []domain.Valasz{domain.Talan, domain.Nem} {
		t.Run(string(to), func(t *testing.T) {
			h := apptest.New(t)
			c := csoportOf(t, h, "bela")
			linkSecondAccount(t, h, "anna", "discord-anna-phone")
			e := propose(t, h, "anna", c.ID, 2)
			d := detail(t, h, "anna", e.ID)
			answer(t, h, "bela", d, 1, domain.Igen)
			confirm(t, h, "anna", d, 1)

			answer(t, h, "bela", d, 1, to)

			want := gateway.DropOut{Name: "bela", CsoportName: "Péntek esték", Title: "Társasjáték", Link: esemenyURL(e.ID)}
			for _, discordID := range []string{"discord-anna", "discord-anna-phone"} {
				var got []gateway.DropOut
				for _, dm := range h.Discord.DMsTo(discordID) {
					if dm.DropOut != nil {
						got = append(got, *dm.DropOut)
					}
				}
				if len(got) != 1 || got[0] != want {
					t.Fatalf("drop-out DMs to %s = %+v, want %+v", discordID, got, want)
				}
			}
			if n := len(dropOutDMs(h)); n != 2 {
				t.Fatalf("%d drop-out DMs, want only the Szervezo's two accounts", n)
			}
			expectResztvevok(t, detail(t, h, "anna", e.ID))
		})
	}
}

func TestNoDropOutDMForOtherChanges(t *testing.T) {
	for name, tc := range map[string]func(t *testing.T, h *apptest.Harness, d app.EsemenyDetail){
		"igen to nem during Egyeztetes": func(t *testing.T, h *apptest.Harness, d app.EsemenyDetail) {
			answer(t, h, "bela", d, 0, domain.Igen)
			answer(t, h, "bela", d, 0, domain.Nem)
		},
		"igen again on the confirmed Idopont": func(t *testing.T, h *apptest.Harness, d app.EsemenyDetail) {
			answer(t, h, "bela", d, 0, domain.Igen)
			confirm(t, h, "anna", d, 0)
			answer(t, h, "bela", d, 0, domain.Igen)
		},
		"talan to nem on the confirmed Idopont": func(t *testing.T, h *apptest.Harness, d app.EsemenyDetail) {
			answer(t, h, "bela", d, 0, domain.Talan)
			confirm(t, h, "anna", d, 0)
			answer(t, h, "bela", d, 0, domain.Nem)
		},
		"a first answer of talan": func(t *testing.T, h *apptest.Harness, d app.EsemenyDetail) {
			confirm(t, h, "anna", d, 0)
			answer(t, h, "bela", d, 0, domain.Talan)
		},
		"a first answer of nem": func(t *testing.T, h *apptest.Harness, d app.EsemenyDetail) {
			confirm(t, h, "anna", d, 0)
			answer(t, h, "bela", d, 0, domain.Nem)
		},
		"nem to igen": func(t *testing.T, h *apptest.Harness, d app.EsemenyDetail) {
			answer(t, h, "bela", d, 0, domain.Nem)
			confirm(t, h, "anna", d, 0)
			answer(t, h, "bela", d, 0, domain.Igen)
		},
		"the Szervezo drops out themselves": func(t *testing.T, h *apptest.Harness, d app.EsemenyDetail) {
			answer(t, h, "anna", d, 0, domain.Igen)
			confirm(t, h, "anna", d, 0)
			answer(t, h, "anna", d, 0, domain.Nem)
		},
		"igen given before a reopen, changed in Egyeztetes": func(t *testing.T, h *apptest.Harness, d app.EsemenyDetail) {
			answer(t, h, "bela", d, 0, domain.Igen)
			confirm(t, h, "anna", d, 0)
			reopen(t, h, "anna", d.ID)
			answer(t, h, "bela", d, 0, domain.Nem)
		},
	} {
		t.Run(name, func(t *testing.T) {
			h := apptest.New(t)
			c := csoportOf(t, h, "bela")
			e := propose(t, h, "anna", c.ID, 2)
			tc(t, h, detail(t, h, "anna", e.ID))
			if dms := dropOutDMs(h); len(dms) != 0 {
				t.Fatalf("drop-out DMs = %+v, want none", dms)
			}
		})
	}
}

func TestLifecycleIsAnnouncedInTheCsatornaAndTheMessageIsReRendered(t *testing.T) {
	h := apptest.New(t)
	c := csoportOf(t, h, "bela")
	linkCsatorna(t, h, c.ID)
	e := propose(t, h, "anna", c.ID, 2)
	post := h.Discord.Posts()[0]
	d := detail(t, h, "anna", e.ID)
	answer(t, h, "bela", d, 1, domain.Igen)
	dmsBefore := len(h.Discord.DMs())
	editsBefore := len(h.Discord.Edits())
	postLink := "https://discord.com/channels/guild-1/chan-1/" + post.Ref.MessageID

	confirm(t, h, "anna", d, 1)

	anns := h.Discord.Announcements()
	want := gateway.Announcement{
		Headline: "✅ Véglegesítve", CsoportName: "Péntek esték", Title: "Társasjáték",
		Detail: "dec. 2. (szerda) 20:00–23:00", Link: postLink,
	}
	if len(anns) != 1 || anns[0].Ref.ChannelID != "chan-1" || anns[0].Announcement != want {
		t.Fatalf("announcements = %+v, want %+v in chan-1", anns, want)
	}
	edits := h.Discord.Edits()[editsBefore:]
	if len(edits) != 1 || edits[0].Ref != post.Ref {
		t.Fatalf("edits = %+v, want the channel post re-rendered once", edits)
	}
	m := edits[0].Message
	if m.Allapot != "Véglegesítve" || m.Idopontok[0].Answerable || !m.Idopontok[1].Answerable ||
		m.Idopontok[0].Confirmed || !m.Idopontok[1].Confirmed || len(m.Resztvevok) != 1 || m.Resztvevok[0] != "bela" {
		t.Fatalf("re-rendered message = %+v", m)
	}

	reopen(t, h, "anna", e.ID)
	cancelEsemeny(t, h, "anna", e.ID)

	anns = h.Discord.Announcements()
	if len(anns) != 3 || anns[1].Announcement.Headline != "↩️ Újra egyeztetés" || anns[2].Announcement.Headline != "❌ Elmaradt" ||
		anns[1].Announcement.Link != postLink || anns[2].Announcement.Detail != "" {
		t.Fatalf("announcements = %+v", anns)
	}
	edits = h.Discord.Edits()[editsBefore:]
	if len(edits) != 3 {
		t.Fatalf("edits = %+v, want one per change", edits)
	}
	if m := edits[1].Message; m.Allapot != "Egyeztetés" || !m.Idopontok[0].Answerable || !m.Idopontok[1].Answerable || m.Resztvevok != nil {
		t.Fatalf("after reopening = %+v", m)
	}
	if m := edits[2].Message; m.Allapot != "Elmaradt" || m.Idopontok[0].Answerable || m.Idopontok[1].Answerable {
		t.Fatalf("after cancelling = %+v, want no answer buttons", m)
	}
	if len(h.Discord.DMs()) != dmsBefore {
		t.Fatalf("DMs = %+v; with a Csatorna the changes are announced only there", h.Discord.DMs()[dmsBefore:])
	}
}

func TestWithoutCsatornaTheLifecycleIsAnnouncedByDMToEveryOtherTag(t *testing.T) {
	h := apptest.New(t)
	c := csoportOf(t, h, "bela", "cecil")
	linkSecondAccount(t, h, "cecil", "discord-cecil-phone")
	e := propose(t, h, "anna", c.ID, 2)
	announcementDMs := map[gateway.MessageRef]bool{}
	for _, dm := range h.Discord.DMs() {
		announcementDMs[dm.Ref] = true
	}
	d := detail(t, h, "anna", e.ID)

	confirm(t, h, "anna", d, 0)

	if anns := h.Discord.Announcements(); len(anns) != 0 {
		t.Fatalf("channel announcements = %+v, want none without a Csatorna", anns)
	}
	want := gateway.Announcement{
		Headline: "✅ Véglegesítve", CsoportName: "Péntek esték", Title: "Társasjáték",
		Detail: "dec. 1. (kedd) 20:00–23:00", Link: esemenyURL(e.ID),
	}
	for _, discordID := range []string{"discord-bela", "discord-cecil", "discord-cecil-phone"} {
		var got []gateway.Announcement
		for _, dm := range h.Discord.DMsTo(discordID) {
			if dm.Announcement != nil {
				got = append(got, *dm.Announcement)
			}
		}
		if len(got) != 1 || got[0] != want {
			t.Fatalf("announcements to %s = %+v, want %+v", discordID, got, want)
		}
	}
	for _, dm := range h.Discord.DMsTo("discord-anna") {
		if dm.Announcement != nil {
			t.Fatalf("the Szervezo was told about their own change: %+v", dm)
		}
	}
	// Every DM copy of the Esemeny's message is re-rendered.
	edits := h.Discord.Edits()
	if len(edits) != len(announcementDMs) || len(edits) != 4 {
		t.Fatalf("edits = %+v, want each of the %d DM copies", edits, len(announcementDMs))
	}
	for _, ed := range edits {
		if !announcementDMs[ed.Ref] || ed.Message.Allapot != "Véglegesítve" || ed.Message.Idopontok[1].Answerable {
			t.Fatalf("edit = %+v", ed)
		}
	}

	cancelEsemeny(t, h, "anna", e.ID)
	var headlines []string
	for _, dm := range h.Discord.DMsTo("discord-bela") {
		if dm.Announcement != nil {
			headlines = append(headlines, dm.Announcement.Headline)
		}
	}
	if len(headlines) != 2 || headlines[1] != "❌ Elmaradt" {
		t.Fatalf("Bela's announcements = %v", headlines)
	}
}

func TestAnnouncementLinksToTheWebWhenTheCsatornaHasNoPostOfTheEsemeny(t *testing.T) {
	h := apptest.New(t)
	c := csoportOf(t, h, "bela")
	e := propose(t, h, "anna", c.ID, 1) // proposed before the Csatorna was linked
	linkCsatorna(t, h, c.ID)

	confirm(t, h, "anna", detail(t, h, "anna", e.ID), 0)

	anns := h.Discord.Announcements()
	if len(anns) != 1 || anns[0].Announcement.Link != esemenyURL(e.ID) {
		t.Fatalf("announcements = %+v, want one linking to the web page", anns)
	}
}

func TestDiscordBeingDownFailsNoLifecycleChange(t *testing.T) {
	h := apptest.New(t)
	c := csoportOf(t, h, "bela")
	linkCsatorna(t, h, c.ID)
	e := propose(t, h, "anna", c.ID, 1)
	d := detail(t, h, "anna", e.ID)
	answer(t, h, "bela", d, 0, domain.Igen)
	h.Discord.Fail(errors.New("discord is down"))

	confirm(t, h, "anna", d, 0)
	answer(t, h, "bela", d, 0, domain.Nem) // a drop-out
	reopen(t, h, "anna", e.ID)
	cancelEsemeny(t, h, "anna", e.ID)

	if got := detail(t, h, "anna", e.ID); got.Allapot != domain.Elmaradt {
		t.Fatalf("allapot = %s, want Elmaradt", got.Allapot)
	}
}
