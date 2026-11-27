package bot_test

import (
	"context"
	"testing"

	"github.com/bwmarrin/discordgo"

	"squadsync-be/internal/app"
	"squadsync-be/internal/bot"
	"squadsync-be/internal/domain"
)

// proposeEsemeny has Anna propose an Esemeny with two Idopontok. There is no
// Csatorna, Anna and Bela get it by DM with the answer buttons.
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

func TestAnswerButtonRecordsTheVisszajelzesAndTheMessageIsEditedOnce(t *testing.T) {
	w := newWorld(t)
	ctx := context.Background()
	if _, err := w.h.App.LinkCsatorna(ctx, "anna", app.LinkCsatornaInput{CsoportID: w.csoport.ID, GuildID: "guild-1", ChannelID: "chan-1"}); err != nil {
		t.Fatalf("LinkCsatorna: %v", err)
	}
	d := w.proposeEsemeny(t)
	post := w.h.Discord.Posts()[0]

	routed := w.router.Route(ctx, answerButton(t, "d-bela", d.ID, d.Idopontok[1].ID, domain.Talan))

	// The press is only acknowledged; the core's refresh is the one writer.
	if resp := routed.Response; resp.Type != discordgo.InteractionResponseDeferredMessageUpdate || resp.Data != nil || routed.Complete != nil {
		t.Fatalf("reply = %+v, want a bare deferred update", resp)
	}
	edits := w.h.Discord.Edits()
	if len(edits) != 1 || edits[0].Ref != post.Ref {
		t.Fatalf("edits = %+v, want exactly one, of the channel post", edits)
	}
	if got := edits[0].Message.Idopontok[1]; got.Talan != 1 || got.Helyezes != 1 {
		t.Fatalf("edited idopont = %+v, want 1 talán in 1st place", got)
	}

	got, err := w.h.App.Esemeny(ctx, "bela", d.ID)
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
