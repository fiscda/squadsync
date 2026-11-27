package bot_test

import (
	"context"
	"strings"
	"testing"

	"github.com/bwmarrin/discordgo"

	"squadsync-be/internal/app"
	"squadsync-be/internal/bot"
	"squadsync-be/internal/domain"
)

// proposeEsemeny has Anna propose an Esemeny with two Idopontok. There is no
// Csatorna, so Bela gets it by DM with the answer buttons.
func (w world) proposeEsemeny(t *testing.T) app.EsemenyDetail {
	t.Helper()
	ctx := context.Background()
	e, err := w.h.App.ProposeEsemeny(ctx, "anna", w.csoport.ID, app.EsemenyInput{
		Title: "Társasjáték",
		Idopontok: []app.IdopontInput{
			{Start: "2026-12-05T20:00", End: "2026-12-05T23:00"},
			{Start: "2026-12-06T18:00"},
		},
	})
	if err != nil {
		t.Fatalf("ProposeEsemeny: %v", err)
	}
	d, err := w.h.App.Esemeny(ctx, "anna", e.ID)
	if err != nil {
		t.Fatalf("Esemeny: %v", err)
	}
	return d
}

// answerButton is a press of an answer button in a DM.
func answerButton(t *testing.T, discordID, esemenyID, idopontID string, v domain.Valasz) *discordgo.Interaction {
	t.Helper()
	id, err := bot.CustomID("vj", esemenyID, idopontID, string(v))
	if err != nil {
		t.Fatalf("CustomID: %v", err)
	}
	return &discordgo.Interaction{
		Type:    discordgo.InteractionMessageComponent,
		User:    &discordgo.User{ID: discordID},
		Message: &discordgo.Message{ID: "msg-1", ChannelID: "dm-" + discordID, Content: "Új esemény – válaszolj!"},
		Data:    discordgo.MessageComponentInteractionData{CustomID: id, ComponentType: discordgo.ButtonComponent},
	}
}

func TestAnswerButtonRecordsTheVisszajelzesAndUpdatesTheMessage(t *testing.T) {
	w := newWorld(t)
	d := w.proposeEsemeny(t)

	routed := w.router.Route(context.Background(), answerButton(t, "d-bela", d.ID, d.Idopontok[1].ID, domain.Talan))

	resp := routed.Response
	if resp.Type != discordgo.InteractionResponseUpdateMessage || resp.Data == nil {
		t.Fatalf("reply = %+v, want an update of the message", resp)
	}
	if resp.Data.Content != "Új esemény – válaszolj!" {
		t.Fatalf("content = %q, want the DM's text kept", resp.Data.Content)
	}
	if len(resp.Data.Embeds) != 1 {
		t.Fatalf("embeds = %+v", resp.Data.Embeds)
	}
	var idopontFields []string
	for _, f := range resp.Data.Embeds[0].Fields {
		if strings.HasPrefix(f.Name, "1 · ") || strings.HasPrefix(f.Name, "2 · ") {
			idopontFields = append(idopontFields, f.Name+" = "+f.Value)
		}
	}
	if len(idopontFields) != 2 || !strings.Contains(idopontFields[1], "1 talán") || !strings.Contains(idopontFields[1], "1. hely") ||
		!strings.Contains(idopontFields[0], "Még senki") {
		t.Fatalf("idopont fields = %q, want the second with 1 talán in 1st place", idopontFields)
	}
	if len(resp.Data.Components) != 2 {
		t.Fatalf("components = %+v, want a button row per Idopont", resp.Data.Components)
	}

	got, err := w.h.App.Esemeny(context.Background(), "bela", d.ID)
	if err != nil {
		t.Fatalf("Esemeny: %v", err)
	}
	if got.Idopontok[1].Sajat != domain.Talan || got.Idopontok[0].Sajat != "" {
		t.Fatalf("Bela's answers = %q, %q; want only talán on the second", got.Idopontok[0].Sajat, got.Idopontok[1].Sajat)
	}
}

func TestAnswerButtonRefusesANonTag(t *testing.T) {
	w := newWorld(t)
	d := w.proposeEsemeny(t)
	if _, err := w.h.App.FirstLogin(context.Background(), app.FirstLoginInput{FelhasznaloID: "cili", DiscordID: "d-cili", Name: "Cili"}); err != nil {
		t.Fatalf("FirstLogin: %v", err)
	}

	resp := w.router.Route(context.Background(), answerButton(t, "d-cili", d.ID, d.Idopontok[0].ID, domain.Igen)).Response
	expectEphemeral(t, resp, "Nem vagy tagja ennek a csoportnak")

	got, err := w.h.App.Esemeny(context.Background(), "anna", d.ID)
	if err != nil {
		t.Fatalf("Esemeny: %v", err)
	}
	if got.Idopontok[0].Igen != 0 {
		t.Fatalf("a non-Tag's press was recorded: %+v", got.Idopontok[0])
	}
}

func TestAnswerButtonSendsAnUnknownUserToLogIn(t *testing.T) {
	w := newWorld(t)
	d := w.proposeEsemeny(t)

	resp := w.router.Route(context.Background(), answerButton(t, "d-stranger", d.ID, d.Idopontok[0].ID, domain.Igen)).Response
	expectEphemeral(t, resp, "Ehhez be kell lépned: "+loginURL)
	got, err := w.h.App.Esemeny(context.Background(), "anna", d.ID)
	if err != nil {
		t.Fatalf("Esemeny: %v", err)
	}
	if got.Idopontok[0].Igen != 0 {
		t.Fatal("an unknown user's press was recorded")
	}
}

func TestAnswerButtonWithAMalformedCustomIDIsRefused(t *testing.T) {
	w := newWorld(t)
	d := w.proposeEsemeny(t)

	resp := w.router.Route(context.Background(), answerButton(t, "d-bela", d.ID, d.Idopontok[0].ID, "lehet")).Response
	expectEphemeral(t, resp, "Ezt nem ismerem")
}

func TestMessageWithMoreThanFiveIdopontokLinksToTheWebInsteadOfButtons(t *testing.T) {
	w := newWorld(t)
	ctx := context.Background()
	var idopontok []app.IdopontInput
	for _, day := range []string{"01", "02", "03", "04", "05", "06"} {
		idopontok = append(idopontok, app.IdopontInput{Start: "2026-12-" + day + "T20:00"})
	}
	e, err := w.h.App.ProposeEsemeny(ctx, "anna", w.csoport.ID, app.EsemenyInput{Title: "Sok időpont", Idopontok: idopontok})
	if err != nil {
		t.Fatalf("ProposeEsemeny: %v", err)
	}
	d, err := w.h.App.Esemeny(ctx, "anna", e.ID)
	if err != nil {
		t.Fatalf("Esemeny: %v", err)
	}

	// A button of an older version of the message still works.
	resp := w.router.Route(ctx, answerButton(t, "d-bela", d.ID, d.Idopontok[0].ID, domain.Igen)).Response
	if resp.Type != discordgo.InteractionResponseUpdateMessage || len(resp.Data.Components) != 1 {
		t.Fatalf("reply = %+v, want an update with one row", resp.Data)
	}
	row, ok := resp.Data.Components[0].(discordgo.ActionsRow)
	if !ok || len(row.Components) != 1 {
		t.Fatalf("row = %+v, want a single link button", resp.Data.Components[0])
	}
	link, ok := row.Components[0].(discordgo.Button)
	if !ok || link.Style != discordgo.LinkButton || link.URL != "https://squadsync.example/esemenyek/"+e.ID {
		t.Fatalf("button = %+v, want a link to the Esemeny's page", row.Components[0])
	}
	if footer := resp.Data.Embeds[0].Footer; footer == nil || !strings.Contains(footer.Text, "weboldalon") {
		t.Fatalf("footer = %+v, want the note about answering on the web", footer)
	}
}
