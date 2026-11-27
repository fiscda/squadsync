package app_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"squadsync-be/internal/app"
	"squadsync-be/internal/apptest"
	"squadsync-be/internal/domain"
	"squadsync-be/internal/gateway"
)

// linkCsatorna has Anna (Admin) link the channel chan-1 of guild-1.
func linkCsatorna(t *testing.T, h *apptest.Harness, csoportID string) {
	t.Helper()
	if _, err := h.App.LinkCsatorna(context.Background(), "anna", app.LinkCsatornaInput{
		CsoportID: csoportID, GuildID: "guild-1", ChannelID: "chan-1",
	}); err != nil {
		t.Fatalf("LinkCsatorna: %v", err)
	}
}

// linkSecondAccount gives the Felhasznalo another Discord account.
func linkSecondAccount(t *testing.T, h *apptest.Harness, felhasznaloID, discordID string) {
	t.Helper()
	if err := h.Store.InsertDiscordAccount(context.Background(), domain.DiscordAccount{
		DiscordID: discordID, FelhasznaloID: felhasznaloID, CreatedAt: apptest.Start,
	}); err != nil {
		t.Fatalf("InsertDiscordAccount: %v", err)
	}
}

func esemenyURL(id string) string { return apptest.WebURL + "/esemenyek/" + id }

func TestProposingAnEsemenyPostsOneChannelMessageAndAsksTheOtherTagsByDM(t *testing.T) {
	h := apptest.New(t)
	c := csoportOf(t, h, "bela", "cecil")
	linkCsatorna(t, h, c.ID)
	linkSecondAccount(t, h, "cecil", "discord-cecil-phone")

	e := propose(t, h, "bela", c.ID, 2)

	posts := h.Discord.Posts()
	if len(posts) != 1 {
		t.Fatalf("posts = %+v, want exactly one", posts)
	}
	p := posts[0]
	if p.Ref.ChannelID != "chan-1" || p.Message.EsemenyID != e.ID || p.Message.Title != "Társasjáték" ||
		p.Message.URL != esemenyURL(e.ID) || p.Message.CsoportName != "Péntek esték" || p.Message.SzervezoName != "bela" {
		t.Fatalf("post = %+v", p)
	}
	if len(p.Message.Idopontok) != 2 || !p.Message.Idopontok[0].Answerable || !p.Message.Idopontok[1].Answerable || p.Message.WebButton {
		t.Fatalf("post idopontok = %+v, want two answerable ones", p.Message.Idopontok)
	}

	// Everyone but the Szervezo, on every linked account.
	for _, discordID := range []string{"discord-anna", "discord-cecil", "discord-cecil-phone"} {
		dms := h.Discord.DMsTo(discordID)
		if len(dms) != 1 {
			t.Fatalf("DMs to %s = %+v, want one", discordID, dms)
		}
		dm := dms[0]
		if dm.Esemeny != nil {
			t.Fatalf("DM to %s carries the Esemeny message; with a Csatorna it should only link to it", discordID)
		}
		want := gateway.VoteRequest{
			CsoportName:       "Péntek esték",
			Title:             "Társasjáték",
			ChannelMessageURL: "https://discord.com/channels/guild-1/chan-1/" + p.Ref.MessageID,
			WebURL:            esemenyURL(e.ID),
		}
		if dm.VoteRequest == nil || *dm.VoteRequest != want {
			t.Fatalf("DM to %s = %+v, want the vote request %+v", discordID, dm.VoteRequest, want)
		}
	}
	if dms := h.Discord.DMsTo("discord-bela"); len(dms) != 0 {
		t.Fatalf("the Szervezo got DMs: %+v", dms)
	}
	if got := len(h.Discord.DMs()); got != 3 {
		t.Fatalf("%d DMs sent, want 3", got)
	}
}

func TestWithoutCsatornaEveryTagGetsTheAnnouncementByDMWithAnswerButtons(t *testing.T) {
	h := apptest.New(t)
	c := csoportOf(t, h, "bela")

	e := propose(t, h, "anna", c.ID, 3)

	if posts := h.Discord.Posts(); len(posts) != 0 {
		t.Fatalf("posts = %+v, want none without a Csatorna", posts)
	}
	// The Szervezo too, so they have the message to follow in Discord.
	for _, discordID := range []string{"discord-anna", "discord-bela"} {
		dms := h.Discord.DMsTo(discordID)
		if len(dms) != 1 || dms[0].Esemeny == nil {
			t.Fatalf("DMs to %s = %+v, want one carrying the Esemeny message", discordID, dms)
		}
		dm := dms[0]
		if dm.VoteRequest == nil || *dm.VoteRequest != (gateway.VoteRequest{CsoportName: "Péntek esték", Title: "Társasjáték"}) {
			t.Fatalf("DM vote request = %+v", dm.VoteRequest)
		}
		if dm.Esemeny.EsemenyID != e.ID || dm.Esemeny.URL != esemenyURL(e.ID) || len(dm.Esemeny.Idopontok) != 3 {
			t.Fatalf("DM message = %+v", dm.Esemeny)
		}
		for _, i := range dm.Esemeny.Idopontok {
			if !i.Answerable {
				t.Fatalf("DM idopont %+v has no answer buttons", i)
			}
		}
	}
}

func TestVisszajelzesEditsTheChannelMessageInPlace(t *testing.T) {
	h := apptest.New(t)
	c := csoportOf(t, h, "bela")
	linkCsatorna(t, h, c.ID)
	e := propose(t, h, "anna", c.ID, 2)
	post := h.Discord.Posts()[0]
	dmsBefore := len(h.Discord.DMs())

	// Answering from the web.
	d := detail(t, h, "bela", e.ID)
	answer(t, h, "bela", d, 1, domain.Igen)
	answer(t, h, "anna", d, 0, domain.Nem)

	edits := h.Discord.Edits()
	if len(edits) != 2 {
		t.Fatalf("edits = %+v, want one per Visszajelzes", edits)
	}
	last := edits[1]
	if last.Ref != post.Ref {
		t.Fatalf("edited %+v, want the channel post %+v", last.Ref, post.Ref)
	}
	got := last.Message.Idopontok
	if got[0].Nem != 1 || got[0].Helyezes != 2 || got[1].Igen != 1 || got[1].Helyezes != 1 {
		t.Fatalf("edited idopontok = %+v, want 1 nem (2.) and 1 igen (1.)", got)
	}
	if len(h.Discord.Posts()) != 1 || len(h.Discord.DMs()) != dmsBefore {
		t.Fatal("a Visszajelzes sent a new message; it should only edit")
	}
}

func TestVisszajelzesEditsEveryAnnouncementDM(t *testing.T) {
	h := apptest.New(t)
	c := csoportOf(t, h, "bela", "cecil")
	linkSecondAccount(t, h, "cecil", "discord-cecil-phone")
	e := propose(t, h, "anna", c.ID, 1)
	var refs []gateway.MessageRef
	for _, dm := range h.Discord.DMs() {
		refs = append(refs, dm.Ref)
	}

	answer(t, h, "cecil", detail(t, h, "cecil", e.ID), 0, domain.Talan)

	edits := h.Discord.Edits()
	// Anna (Szervezo), Bela, and Cecil on both accounts.
	if len(edits) != len(refs) || len(refs) != 4 {
		t.Fatalf("edits = %+v, want one for each of the %d DMs", edits, len(refs))
	}
	edited := map[gateway.MessageRef]bool{}
	for _, ed := range edits {
		edited[ed.Ref] = true
		if ed.Message.Idopontok[0].Talan != 1 {
			t.Fatalf("edit = %+v, want 1 talán", ed.Message)
		}
	}
	for _, r := range refs {
		if !edited[r] {
			t.Fatalf("DM %+v wasn't edited", r)
		}
	}
}

func TestDiscordBeingDownFailsNeitherProposingNorAnswering(t *testing.T) {
	h := apptest.New(t)
	c := csoportOf(t, h, "bela")
	linkCsatorna(t, h, c.ID)
	h.Discord.Fail(errors.New("discord is down"))

	e := propose(t, h, "anna", c.ID, 1)
	answer(t, h, "bela", detail(t, h, "bela", e.ID), 0, domain.Igen)

	if d := detail(t, h, "anna", e.ID); d.Idopontok[0].Igen != 1 {
		t.Fatalf("idopont = %+v, want Bela's igen recorded", d.Idopontok[0])
	}
	if len(h.Discord.Posts())+len(h.Discord.DMs())+len(h.Discord.Edits()) != 0 {
		t.Fatal("the failing fake recorded a message")
	}
}

func TestEsemenyMessageShowsBudapestTimesAndFallsBackToTheWebForMoreThanFiveIdopontok(t *testing.T) {
	h := apptest.New(t)
	ctx := context.Background()
	c := csoportOf(t, h, "bela")
	e, err := h.App.ProposeEsemeny(ctx, "anna", c.ID, app.EsemenyInput{
		Title:    "Év vége",
		Helyszin: domain.Helyszin{Name: "Kisüzem", URL: "https://maps.app.goo.gl/abc"},
		Idopontok: []app.IdopontInput{
			{Start: "2026-12-05T20:00", End: "2026-12-05T23:30"},
			{Start: "2026-07-10T18:00"},
			{Start: "2026-08-14", End: "2026-08-16", EgeszNapos: true},
			{Start: "2026-12-24", EgeszNapos: true},
			{Start: "2026-12-31T22:00", End: "2027-01-01T02:00"},
			{Start: "2027-01-02T10:00"},
		},
	})
	if err != nil {
		t.Fatalf("ProposeEsemeny: %v", err)
	}
	d := detail(t, h, "bela", e.ID)
	answer(t, h, "bela", d, 0, domain.Igen) // júl. 10.
	answer(t, h, "anna", d, 0, domain.Talan)
	answer(t, h, "bela", d, 2, domain.Igen) // dec. 5.

	edits := h.Discord.Edits()
	if len(edits) == 0 {
		t.Fatal("no message was edited")
	}
	m := edits[len(edits)-1].Message
	if m.HelyszinName != "Kisüzem" || m.HelyszinURL != "https://maps.app.goo.gl/abc" || m.Allapot != "Egyeztetés" || m.SzervezoName != "Anna" {
		t.Fatalf("message = %+v", m)
	}
	wantLabels := []string{
		"júl. 10. (péntek) 18:00",
		"aug. 14. (péntek) – aug. 16. (vasárnap)",
		"dec. 5. (szombat) 20:00–23:30",
		"dec. 24. (csütörtök), egész nap",
		"dec. 31. (csütörtök) 22:00 – 2027. jan. 1. (péntek) 02:00",
		"2027. jan. 2. (szombat) 10:00",
	}
	if len(m.Idopontok) != len(wantLabels) {
		t.Fatalf("idopontok = %+v", m.Idopontok)
	}
	for n, want := range wantLabels {
		if m.Idopontok[n].Label != want {
			t.Fatalf("idopont %d label = %q, want %q", n, m.Idopontok[n].Label, want)
		}
		if m.Idopontok[n].Answerable {
			t.Fatalf("idopont %d has answer buttons; 6 Idopontok don't fit", n)
		}
	}
	if first := m.Idopontok[0]; first.Igen != 1 || first.Talan != 1 || first.Helyezes != 1 {
		t.Fatalf("júl. 10. = %+v, want 1 igen, 1 talán, 1.", first)
	}
	if third := m.Idopontok[2]; third.Igen != 1 || third.Talan != 0 || third.Helyezes != 2 {
		t.Fatalf("dec. 5. = %+v, want 1 igen, 2.", third)
	}
	if m.Idopontok[1].Helyezes != 0 {
		t.Fatalf("an unanswered Idopont has Helyezes %d", m.Idopontok[1].Helyezes)
	}
	if !m.WebButton || !strings.Contains(m.Note, "weboldalon") {
		t.Fatalf("WebButton %v, Note %q; want a link to the web instead of buttons", m.WebButton, m.Note)
	}

	// Answering from the web keeps working for every Idopont.
	answer(t, h, "bela", d, 5, domain.Nem)
}

func TestLocalhostWebURLIsShownAsTextNotAsALink(t *testing.T) {
	h := apptest.New(t)
	local := app.New(app.Deps{
		Store: h.Store, Clock: h.Clock, Discord: h.Discord, Calendar: h.Calendar, Spotify: h.Spotify,
		WebURL: "http://localhost:3000",
	})
	c := csoportOf(t, h, "bela")
	linkCsatorna(t, h, c.ID)

	e, err := local.ProposeEsemeny(context.Background(), "anna", c.ID, app.EsemenyInput{Title: "Helyben", Idopontok: idopontok(6)})
	if err != nil {
		t.Fatalf("ProposeEsemeny: %v", err)
	}

	m := h.Discord.Posts()[0].Message
	page := "http://localhost:3000/esemenyek/" + e.ID
	if m.URL != "" || m.WebButton {
		t.Fatalf("URL %q, WebButton %v; Discord rejects localhost links", m.URL, m.WebButton)
	}
	if !strings.Contains(m.Note, "weboldalon") || !strings.Contains(m.Note, page) {
		t.Fatalf("Note = %q, want the page as text", m.Note)
	}
	if dm := h.Discord.DMsTo("discord-bela")[0]; dm.VoteRequest.WebURL != page {
		t.Fatalf("vote request = %+v, want the page as text", dm.VoteRequest)
	}
}

func TestAnnouncementsWaitInTheBackgroundWithAsyncDiscord(t *testing.T) {
	h := apptest.New(t)
	c := csoportOf(t, h, "bela")
	async := app.New(app.Deps{
		Store: h.Store, Clock: h.Clock, Discord: h.Discord, Calendar: h.Calendar, Spotify: h.Spotify,
		WebURL: apptest.WebURL, AsyncDiscord: true,
	})
	if _, err := async.ProposeEsemeny(context.Background(), "anna", c.ID, app.EsemenyInput{
		Title:     "Háttér",
		Idopontok: []app.IdopontInput{{Start: "2026-12-05T20:00"}},
	}); err != nil {
		t.Fatalf("ProposeEsemeny: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	async.WaitDiscord(ctx)
	if ctx.Err() != nil {
		t.Fatal("WaitDiscord ran into its deadline")
	}
	if dms := h.Discord.DMsTo("discord-bela"); len(dms) != 1 {
		t.Fatalf("DMs to Bela = %+v, want the announcement once the background work is done", dms)
	}
}
