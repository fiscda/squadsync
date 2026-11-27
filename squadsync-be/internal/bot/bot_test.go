package bot_test

import (
	"context"
	"strings"
	"testing"

	"github.com/bwmarrin/discordgo"

	"squadsync-be/internal/app"
	"squadsync-be/internal/apptest"
	"squadsync-be/internal/bot"
	"squadsync-be/internal/domain"
)

const loginURL = "https://squadsync.example"

// world is a Router over a fresh application core with Anna (Admin of
// "Péntek esték") and Bela (a plain Tag of it). Nobody connects to Discord.
type world struct {
	h       *apptest.Harness
	router  *bot.Router
	csoport domain.Csoport
}

func newWorld(t *testing.T) world {
	t.Helper()
	ctx := context.Background()
	h := apptest.New(t)
	for _, f := range []app.FirstLoginInput{
		{FelhasznaloID: "anna", DiscordID: "d-anna", Name: "Anna"},
		{FelhasznaloID: "bela", DiscordID: "d-bela", Name: "Béla"},
	} {
		if _, err := h.App.FirstLogin(ctx, f); err != nil {
			t.Fatalf("FirstLogin: %v", err)
		}
	}
	c, err := h.App.CreateCsoport(ctx, "anna", "Péntek esték")
	if err != nil {
		t.Fatalf("CreateCsoport: %v", err)
	}
	m, err := h.App.CreateCsoportMeghivo(ctx, "anna", c.ID)
	if err != nil {
		t.Fatalf("CreateCsoportMeghivo: %v", err)
	}
	if _, err := h.App.JoinCsoport(ctx, "bela", m.Token); err != nil {
		t.Fatalf("JoinCsoport: %v", err)
	}
	r := bot.NewRouter(bot.Deps{
		App:      h.App,
		LoginURL: loginURL,
		ChannelName: func(id string) string {
			if id == "chan-1" {
				return "altalanos"
			}
			return ""
		},
	})
	return world{h: h, router: r, csoport: c}
}

// csatorna builds a /csatorna interaction from a server member.
func csatorna(discordID, csoportID string, typ discordgo.InteractionType) *discordgo.Interaction {
	return &discordgo.Interaction{
		Type:      typ,
		GuildID:   "guild-1",
		ChannelID: "chan-1",
		Member:    &discordgo.Member{User: &discordgo.User{ID: discordID}},
		Data: discordgo.ApplicationCommandInteractionData{
			Name: "csatorna",
			Options: []*discordgo.ApplicationCommandInteractionDataOption{{
				Name:    "csoport",
				Type:    discordgo.ApplicationCommandOptionString,
				Value:   csoportID,
				Focused: typ == discordgo.InteractionApplicationCommandAutocomplete,
			}},
		},
	}
}

func (w world) csatorna(t *testing.T) *domain.Csatorna {
	t.Helper()
	d, err := w.h.App.Csoport(context.Background(), "anna", w.csoport.ID)
	if err != nil {
		t.Fatalf("Csoport: %v", err)
	}
	return d.Csatorna
}

func expectEphemeral(t *testing.T, resp *discordgo.InteractionResponse, contains string) {
	t.Helper()
	if resp.Type != discordgo.InteractionResponseChannelMessageWithSource || resp.Data == nil ||
		resp.Data.Flags&discordgo.MessageFlagsEphemeral == 0 || !strings.Contains(resp.Data.Content, contains) {
		t.Fatalf("reply = %+v, want an ephemeral message containing %q", resp.Data, contains)
	}
}

func TestUnknownDiscordAccountGetsOnlyALoginLink(t *testing.T) {
	w := newWorld(t)
	ctx := context.Background()

	resp := w.router.Route(ctx, csatorna("d-stranger", w.csoport.ID, discordgo.InteractionApplicationCommand))
	expectEphemeral(t, resp, "Ehhez be kell lépned: "+loginURL)
	if got := w.csatorna(t); got != nil {
		t.Fatalf("unknown account linked a channel: %+v", got)
	}

	// From a DM the user is in User, not Member, and buttons get the same.
	button := &discordgo.Interaction{
		Type: discordgo.InteractionMessageComponent,
		User: &discordgo.User{ID: "d-stranger"},
		Data: discordgo.MessageComponentInteractionData{CustomID: "valami:1"},
	}
	expectEphemeral(t, w.router.Route(ctx, button), "Ehhez be kell lépned: "+loginURL)

	auto := w.router.Route(ctx, csatorna("d-stranger", "", discordgo.InteractionApplicationCommandAutocomplete))
	if auto.Type != discordgo.InteractionApplicationCommandAutocompleteResult || len(auto.Data.Choices) != 0 {
		t.Fatalf("autocomplete for a stranger = %+v, want no choices", auto.Data)
	}
}

func TestAdminLinksTheChannelWithCsatorna(t *testing.T) {
	w := newWorld(t)

	resp := w.router.Route(context.Background(), csatorna("d-anna", w.csoport.ID, discordgo.InteractionApplicationCommand))
	if resp.Data == nil || resp.Data.Flags&discordgo.MessageFlagsEphemeral != 0 || !strings.Contains(resp.Data.Content, "Péntek esték") {
		t.Fatalf("reply = %+v, want a public confirmation naming the Csoport", resp.Data)
	}
	got := w.csatorna(t)
	if got == nil || got.GuildID != "guild-1" || got.ChannelID != "chan-1" || got.ChannelName != "altalanos" || got.LinkedBy != "anna" {
		t.Fatalf("Csatorna = %+v, want guild-1/chan-1 altalanos linked by Anna", got)
	}
}

func TestNonAdminIsRefusedByCsatorna(t *testing.T) {
	w := newWorld(t)

	resp := w.router.Route(context.Background(), csatorna("d-bela", w.csoport.ID, discordgo.InteractionApplicationCommand))
	expectEphemeral(t, resp, "Csak a csoport adminja")
	if got := w.csatorna(t); got != nil {
		t.Fatalf("a Tag linked a channel: %+v", got)
	}

	// Typing a name instead of picking from the list isn't a Csoport id.
	resp = w.router.Route(context.Background(), csatorna("d-anna", "Péntek esték", discordgo.InteractionApplicationCommand))
	expectEphemeral(t, resp, "Nem találom ezt a csoportot")
}

func TestCsatornaAutocompleteOffersOnlyAdminCsoports(t *testing.T) {
	w := newWorld(t)
	ctx := context.Background()
	own, err := w.h.App.CreateCsoport(ctx, "bela", "Béla túrái")
	if err != nil {
		t.Fatalf("CreateCsoport: %v", err)
	}
	if _, err := w.h.App.CreateCsoport(ctx, "bela", "Társas"); err != nil {
		t.Fatalf("CreateCsoport: %v", err)
	}

	resp := w.router.Route(ctx, csatorna("d-bela", "TÚR", discordgo.InteractionApplicationCommandAutocomplete))
	if resp.Type != discordgo.InteractionApplicationCommandAutocompleteResult {
		t.Fatalf("reply type = %v, want an autocomplete result", resp.Type)
	}
	choices := resp.Data.Choices
	if len(choices) != 1 || choices[0].Name != "Béla túrái" || choices[0].Value != own.ID {
		t.Fatalf("choices = %+v, want only Béla túrái (filtered, and never Péntek esték where Béla isn't Admin)", choices)
	}
}

func TestEveryCommandHasADefinition(t *testing.T) {
	w := newWorld(t)
	cmds := w.router.Commands()
	if len(cmds) != 1 || cmds[0].Name != "csatorna" || len(cmds[0].Options) != 1 || !cmds[0].Options[0].Autocomplete {
		t.Fatalf("commands = %+v, want /csatorna with an autocomplete option", cmds)
	}
}

func TestComponentsAreRoutedByCustomIDWithTheirArgs(t *testing.T) {
	w := newWorld(t)
	var got bot.Request
	w.router.Component("proba", func(_ context.Context, req bot.Request) (*discordgo.InteractionResponse, error) {
		got = req
		return bot.Ephemeral("ok"), nil
	})

	button := &discordgo.Interaction{
		Type:   discordgo.InteractionMessageComponent,
		Member: &discordgo.Member{User: &discordgo.User{ID: "d-bela"}},
		Data:   discordgo.MessageComponentInteractionData{CustomID: bot.CustomID("proba", "idopont-1", "igen")},
	}
	expectEphemeral(t, w.router.Route(context.Background(), button), "ok")
	if got.Felhasznalo.ID != "bela" || len(got.Args) != 2 || got.Args[0] != "idopont-1" || got.Args[1] != "igen" {
		t.Fatalf("request = %+v, want Béla with args [idopont-1 igen]", got)
	}

	button.Data = discordgo.MessageComponentInteractionData{CustomID: "ismeretlen:1"}
	expectEphemeral(t, w.router.Route(context.Background(), button), "Ezt nem ismerem")
}
