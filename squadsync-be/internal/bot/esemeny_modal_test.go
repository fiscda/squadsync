package bot_test

import (
	"context"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/bwmarrin/discordgo"

	"squadsync-be/internal/app"
	"squadsync-be/internal/apptest"
	"squadsync-be/internal/bot"
	"squadsync-be/internal/clock"
	"squadsync-be/internal/domain"
	"squadsync-be/internal/gateway/gatewayfake"
)

// esemenyCommand is a /esemeny interaction from a server member.
func esemenyCommand(discordID, csoportID string, typ discordgo.InteractionType) *discordgo.Interaction {
	return &discordgo.Interaction{
		Type:      typ,
		GuildID:   "guild-1",
		ChannelID: "chan-1",
		Member:    &discordgo.Member{User: &discordgo.User{ID: discordID}},
		Data: discordgo.ApplicationCommandInteractionData{
			Name: "esemeny",
			Options: []*discordgo.ApplicationCommandInteractionDataOption{{
				Name:    "csoport",
				Type:    discordgo.ApplicationCommandOptionString,
				Value:   csoportID,
				Focused: typ == discordgo.InteractionApplicationCommandAutocomplete,
			}},
		},
	}
}

// modalFields are what a user types into the /esemeny modal.
type modalFields struct {
	cim, helyszin, terkep, leiras, idopontok string
}

// esemenySubmit is the submit of the modal that /esemeny opened for csoportID.
func esemenySubmit(t *testing.T, discordID, csoportID string, f modalFields) *discordgo.Interaction {
	t.Helper()
	id, err := bot.CustomID("esemeny", csoportID)
	if err != nil {
		t.Fatalf("CustomID: %v", err)
	}
	input := func(customID, value string) discordgo.MessageComponent {
		// As discordgo unmarshals a real submit: pointers.
		return &discordgo.ActionsRow{Components: []discordgo.MessageComponent{
			&discordgo.TextInput{CustomID: customID, Value: value},
		}}
	}
	return &discordgo.Interaction{
		Type:      discordgo.InteractionModalSubmit,
		GuildID:   "guild-1",
		ChannelID: "chan-1",
		Member:    &discordgo.Member{User: &discordgo.User{ID: discordID}},
		Data: discordgo.ModalSubmitInteractionData{
			CustomID: id,
			Components: []discordgo.MessageComponent{
				input("cim", f.cim),
				input("helyszin", f.helyszin),
				input("terkep", f.terkep),
				input("leiras", f.leiras),
				input("idopontok", f.idopontok),
			},
		},
	}
}

// esemenyekOf lists the Esemenyek of the world's Csoport, as Anna sees them.
func (w world) esemenyekOf(t *testing.T) []domain.EsemenySummary {
	t.Helper()
	list, err := w.h.App.EsemenyekOfCsoport(context.Background(), "anna", w.csoport.ID)
	if err != nil {
		t.Fatalf("EsemenyekOfCsoport: %v", err)
	}
	return list
}

func TestEsemenyAutocompleteOffersEveryCsoportTheCallerIsATagOf(t *testing.T) {
	w := newWorld(t)
	ctx := context.Background()
	own, err := w.h.App.CreateCsoport(ctx, "bela", "Béla túrái")
	if err != nil {
		t.Fatalf("CreateCsoport: %v", err)
	}
	// Anna is not a Tag of this one.
	if _, err := w.h.App.CreateCsoport(ctx, "bela", "Titkos"); err != nil {
		t.Fatalf("CreateCsoport: %v", err)
	}

	// Béla is a plain Tag of Péntek esték and Admin of the two others.
	resp := w.router.Route(ctx, esemenyCommand("d-bela", "", discordgo.InteractionApplicationCommandAutocomplete)).Response
	if resp.Type != discordgo.InteractionApplicationCommandAutocompleteResult || len(resp.Data.Choices) != 3 {
		t.Fatalf("choices = %+v, want all three of Béla's Csoports", resp.Data)
	}

	// Anna is a Tag of only Péntek esték; filtering ignores case.
	resp = w.router.Route(ctx, esemenyCommand("d-anna", "", discordgo.InteractionApplicationCommandAutocomplete)).Response
	choices := resp.Data.Choices
	if len(choices) != 1 || choices[0].Name != "Péntek esték" || choices[0].Value != w.csoport.ID {
		t.Fatalf("Anna's choices = %+v, want only Péntek esték", choices)
	}

	resp = w.router.Route(ctx, esemenyCommand("d-bela", "TÚR", discordgo.InteractionApplicationCommandAutocomplete)).Response
	choices = resp.Data.Choices
	if len(choices) != 1 || choices[0].Value != own.ID {
		t.Fatalf("filtered choices = %+v, want only Béla túrái", choices)
	}

	// Anna is not a Tag of Béla's Csoports: neither shows up for her.
	resp = w.router.Route(ctx, esemenyCommand("d-anna", "túr", discordgo.InteractionApplicationCommandAutocomplete)).Response
	if len(resp.Data.Choices) != 0 {
		t.Fatalf("Anna's choices for \"túr\" = %+v, want none", resp.Data.Choices)
	}
}

func TestEsemenyOpensAModalWithTheFiveInputs(t *testing.T) {
	w := newWorld(t)

	// Béla is a plain Tag, not an Admin, and may still propose.
	resp := w.router.Route(context.Background(), esemenyCommand("d-bela", w.csoport.ID, discordgo.InteractionApplicationCommand)).Response
	if resp.Type != discordgo.InteractionResponseModal || resp.Data == nil {
		t.Fatalf("reply = %+v, want a modal", resp)
	}
	if want := "esemeny:" + w.csoport.ID; resp.Data.CustomID != want {
		t.Fatalf("custom id = %q, want %q", resp.Data.CustomID, want)
	}
	if n := utf8.RuneCountInString(resp.Data.Title); n == 0 || n > 45 || !strings.Contains(resp.Data.Title, "Péntek esték") {
		t.Fatalf("title = %q, want 1-45 characters naming the Csoport", resp.Data.Title)
	}

	type input struct {
		style    discordgo.TextInputStyle
		required bool
		max      int
	}
	got := map[string]input{}
	var order []string
	for _, c := range resp.Data.Components {
		row := c.(discordgo.ActionsRow)
		if len(row.Components) != 1 {
			t.Fatalf("row = %+v, want one input", row)
		}
		in := row.Components[0].(discordgo.TextInput)
		got[in.CustomID] = input{in.Style, in.Required, in.MaxLength}
		order = append(order, in.CustomID)
		if n := utf8.RuneCountInString(in.Placeholder); n > 100 {
			t.Errorf("%s placeholder has %d characters, Discord allows 100", in.CustomID, n)
		}
		if n := utf8.RuneCountInString(in.Label); n == 0 || n > 45 {
			t.Errorf("%s label %q has %d characters, Discord allows 1-45", in.CustomID, in.Label, n)
		}
	}
	if strings.Join(order, ",") != "cim,helyszin,terkep,leiras,idopontok" {
		t.Fatalf("inputs = %v, want cím, helyszín, térkép, leírás, időpontok", order)
	}
	want := map[string]input{
		"cim":       {discordgo.TextInputShort, true, 100},
		"helyszin":  {discordgo.TextInputShort, false, app.MaxHelyszinNameLength},
		"terkep":    {discordgo.TextInputShort, false, app.MaxHelyszinURLLength},
		"leiras":    {discordgo.TextInputParagraph, false, app.MaxEsemenyDescriptionLength},
		"idopontok": {discordgo.TextInputParagraph, true, 500},
	}
	for id, w := range want {
		if got[id] != w {
			t.Errorf("input %s = %+v, want %+v", id, got[id], w)
		}
	}
}

func TestEsemenyIsRefusedForACsoportTheCallerIsNotATagOf(t *testing.T) {
	w := newWorld(t)
	ctx := context.Background()
	other, err := w.h.App.CreateCsoport(ctx, "bela", "Béla túrái")
	if err != nil {
		t.Fatalf("CreateCsoport: %v", err)
	}

	// No modal opens for a Csoport Anna is not in, nor for a typed name.
	for _, id := range []string{other.ID, "Péntek esték", ""} {
		resp := w.router.Route(ctx, esemenyCommand("d-anna", id, discordgo.InteractionApplicationCommand)).Response
		expectEphemeral(t, resp, "Nem találom ezt a csoportot")
	}

	// A forged submit is refused too, and creates nothing.
	resp := w.router.Route(ctx, esemenySubmit(t, "d-anna", other.ID, modalFields{cim: "Túra", idopontok: "12.05 20:00"})).Response
	expectEphemeral(t, resp, "Nem vagy tagja")
	if list, err := w.h.App.EsemenyekOfCsoport(ctx, "bela", other.ID); err != nil || len(list) != 0 {
		t.Fatalf("Esemenyek = %+v, %v; want none", list, err)
	}
	if posts := w.h.Discord.Posts(); len(posts) != 0 {
		t.Fatalf("posts = %+v, want none", posts)
	}
}

func TestEsemenySubmitProposesLikeTheWebAndPostsInTheCsatorna(t *testing.T) {
	w := newWorld(t)
	ctx := context.Background()
	if _, err := w.h.App.LinkCsatorna(ctx, "anna", app.LinkCsatornaInput{CsoportID: w.csoport.ID, GuildID: "guild-1", ChannelID: "chan-1"}); err != nil {
		t.Fatalf("LinkCsatorna: %v", err)
	}

	// Béla, a plain Tag, proposes. The fake clock stands on 2026-03-02.
	routed := w.router.Route(ctx, esemenySubmit(t, "d-bela", w.csoport.ID, modalFields{
		cim:      "  Társasjáték  ",
		helyszin: "Béla lakása",
		terkep:   "https://maps.app.goo.gl/abc",
		leiras:   "Hozzatok chipset",
		idopontok: "12.05 20:00-23:00\n" +
			"\n" +
			"12.12.–12.13.\n" +
			"12.30 18:00-01.02 10:00",
	}))
	if routed.Complete != nil {
		t.Fatalf("reply is deferred, want an immediate one")
	}
	expectEphemeral(t, routed.Response, "Létrehozva: **Társasjáték**")
	if c := routed.Response.Data.Content; !strings.Contains(c, apptest.WebURL) {
		t.Fatalf("reply = %q, want a link to the web page under %s", c, apptest.WebURL)
	}

	list := w.esemenyekOf(t)
	if len(list) != 1 {
		t.Fatalf("Esemenyek = %+v, want exactly one", list)
	}
	d, err := w.h.App.Esemeny(ctx, "anna", list[0].ID)
	if err != nil {
		t.Fatalf("Esemeny: %v", err)
	}
	if d.Title != "Társasjáték" || d.Description != "Hozzatok chipset" || d.Helyszin != (domain.Helyszin{Name: "Béla lakása", URL: "https://maps.app.goo.gl/abc"}) ||
		d.SzervezoID != "bela" || d.Allapot != domain.Egyeztetes {
		t.Fatalf("Esemeny = %+v, want Béla's Társasjáték in Egyeztetés", d.Esemeny)
	}
	if !strings.Contains(routed.Response.Data.Content, d.ID) {
		t.Fatalf("reply = %q, want the link to Esemeny %s", routed.Response.Data.Content, d.ID)
	}
	if len(d.Idopontok) != 3 {
		t.Fatalf("Idopontok = %+v, want 3", d.Idopontok)
	}
	// Dec 5 20:00 CET is 19:00 UTC; the whole days run to Budapest midnight.
	if s, e := d.Idopontok[0], d.Idopontok[0].End; s.Start.UTC().Format("2006-01-02T15:04") != "2026-12-05T19:00" || e == nil || e.UTC().Format("2006-01-02T15:04") != "2026-12-05T22:00" || s.EgeszNapos {
		t.Fatalf("first Idopont = %+v, want 2026-12-05 20:00-23:00 Budapest", s)
	}
	if s := d.Idopontok[1]; !s.EgeszNapos || s.Start.UTC().Format("2006-01-02T15:04") != "2026-12-11T23:00" || s.End.UTC().Format("2006-01-02T15:04") != "2026-12-13T23:00" {
		t.Fatalf("second Idopont = %+v, want whole days 12.12-12.13", s)
	}
	if s := d.Idopontok[2]; s.End == nil || s.End.UTC().Format("2006-01-02T15:04") != "2027-01-02T09:00" {
		t.Fatalf("third Idopont = %+v, want an end on 2027-01-02 across New Year", s)
	}

	// The channel post is the one from the core's flow.
	posts := w.h.Discord.Posts()
	if len(posts) != 1 || posts[0].Ref.ChannelID != "chan-1" || posts[0].Message.Title != "Társasjáték" || len(posts[0].Message.Idopontok) != 3 {
		t.Fatalf("posts = %+v, want one in chan-1 with 3 Idopontok", posts)
	}
}

func TestEsemenySubmitLinksTheWebPageOnlyWhenDiscordCanLinkIt(t *testing.T) {
	submit := func(webURL string) string {
		t.Helper()
		ctx := context.Background()
		store, _ := apptest.NewStore(t)
		a := app.New(app.Deps{Store: store, Clock: clock.NewFake(apptest.Start), Discord: &gatewayfake.Discord{}, WebURL: webURL})
		if _, err := a.FirstLogin(ctx, app.FirstLoginInput{FelhasznaloID: "anna", DiscordID: "d-anna", Name: "Anna"}); err != nil {
			t.Fatalf("FirstLogin: %v", err)
		}
		c, err := a.CreateCsoport(ctx, "anna", "Péntek esték")
		if err != nil {
			t.Fatalf("CreateCsoport: %v", err)
		}
		resp := bot.NewRouter(bot.Deps{App: a, LoginURL: loginURL}).
			Route(ctx, esemenySubmit(t, "d-anna", c.ID, modalFields{cim: "Mozi", idopontok: "12.05 20:00"})).Response
		expectEphemeral(t, resp, "Létrehozva: **Mozi**")
		return resp.Data.Content
	}

	// A public URL is a link, in <> so Discord shows no preview of it.
	if got := submit("https://squadsync.example"); !strings.Contains(got, "Weboldal: <https://squadsync.example/esemenyek/") {
		t.Fatalf("reply = %q, want a link to the web page", got)
	}
	// Discord can't link localhost: print it as plain text.
	if got := submit("http://localhost:3000"); !strings.Contains(got, "Weboldal: http://localhost:3000/esemenyek/") || strings.Contains(got, "<http") {
		t.Fatalf("reply = %q, want the localhost URL as plain text", got)
	}
}

func TestEsemenySubmitWithBadIdopontLineCreatesNothing(t *testing.T) {
	w := newWorld(t)
	ctx := context.Background()
	if _, err := w.h.App.LinkCsatorna(ctx, "anna", app.LinkCsatornaInput{CsoportID: w.csoport.ID, GuildID: "guild-1", ChannelID: "chan-1"}); err != nil {
		t.Fatalf("LinkCsatorna: %v", err)
	}

	tests := []struct {
		name, idopontok, want string
	}{
		{"invalid date on the third line", "12.05 20:00\n12.06 20:00\n12.40 20:00", "3. sor („12.40 20:00”): érvénytelen dátum"},
		{"bad format", "holnap", "1. sor („holnap”): érvénytelen formátum"},
		{"end before start", "12.05 20:00-19:00", "a vége nem lehet a kezdete előtt"},
		{"too many", "12.01 20:00\n12.02 20:00\n12.03 20:00\n12.04 20:00\n12.05 20:00\n12.06 20:00", "legfeljebb 5 időpontot"},
		{"none", "  \n ", "adj meg legalább egy időpontot"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			routed := w.router.Route(ctx, esemenySubmit(t, "d-anna", w.csoport.ID, modalFields{cim: "Kirándulás", idopontok: tc.idopontok}))
			expectEphemeral(t, routed.Response, tc.want)
			if routed.Complete != nil {
				t.Fatalf("an error reply must not be deferred")
			}
			if list := w.esemenyekOf(t); len(list) != 0 {
				t.Fatalf("Esemenyek = %+v, want none", list)
			}
			if posts := w.h.Discord.Posts(); len(posts) != 0 {
				t.Fatalf("posts = %+v, want none", posts)
			}
		})
	}
}

func TestEsemenySubmitExplainsValidationErrorsInHungarian(t *testing.T) {
	w := newWorld(t)
	ctx := context.Background()
	w.h.Clock.Set(time.Date(2027, time.March, 1, 12, 0, 0, 0, app.Budapest))

	tests := []struct {
		name   string
		fields modalFields
		want   string
	}{
		{"blank title", modalFields{cim: "   ", idopontok: "12.05 20:00"}, "Adj meg egy címet"},
		{"bad maps link", modalFields{cim: "Mozi", terkep: "javascript:alert(1)", idopontok: "12.05 20:00"}, "http:// vagy https://"},
		{"a time skipped by the clocks changing", modalFields{cim: "Mozi", idopontok: "03.28 02:30"}, "óraátállítás"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			resp := w.router.Route(ctx, esemenySubmit(t, "d-anna", w.csoport.ID, tc.fields)).Response
			expectEphemeral(t, resp, tc.want)
			if list := w.esemenyekOf(t); len(list) != 0 {
				t.Fatalf("Esemenyek = %+v, want none", list)
			}
		})
	}
}

func TestEsemenySubmitWithAMalformedCustomIDIsRefused(t *testing.T) {
	w := newWorld(t)
	i := esemenySubmit(t, "d-anna", w.csoport.ID, modalFields{cim: "Mozi", idopontok: "12.05 20:00"})
	d := i.ModalSubmitData()
	d.CustomID = "esemeny"
	i.Data = d
	expectEphemeral(t, w.router.Route(context.Background(), i).Response, "Ezt nem ismerem")
	if list := w.esemenyekOf(t); len(list) != 0 {
		t.Fatalf("Esemenyek = %+v, want none", list)
	}
}
