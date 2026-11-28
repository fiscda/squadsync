package bot_test

import (
	"context"
	"testing"
	"time"

	"github.com/bwmarrin/discordgo"

	"squadsync-be/internal/app"
	"squadsync-be/internal/bot"
	"squadsync-be/internal/domain"
)

// confirmButton is a press of a confirm button in the Hatarido DM.
func confirmButton(t *testing.T, discordID, esemenyID, idopontID string) *discordgo.Interaction {
	t.Helper()
	id, err := bot.CustomID("vegl", esemenyID, idopontID)
	if err != nil {
		t.Fatalf("CustomID: %v", err)
	}
	return &discordgo.Interaction{
		Type: discordgo.InteractionMessageComponent,
		User: &discordgo.User{ID: discordID},
		Data: discordgo.MessageComponentInteractionData{CustomID: id, ComponentType: discordgo.ButtonComponent},
	}
}

func TestHataridoDMConfirmButtonConfirmsForTheSzervezoOnly(t *testing.T) {
	w := newWorld(t)
	ctx := context.Background()
	e, err := w.h.App.ProposeEsemeny(ctx, "anna", w.csoport.ID, app.EsemenyInput{
		Title:     "Társasjáték",
		Idopontok: []app.IdopontInput{{Start: "2026-12-05T20:00"}, {Start: "2026-12-06T18:00"}},
		Hatarido:  "2026-03-05T20:00",
	})
	if err != nil {
		t.Fatalf("ProposeEsemeny: %v", err)
	}
	w.h.Clock.Set(time.Date(2026, time.March, 5, 19, 0, 0, 0, time.UTC))
	if err := w.h.App.RunDueJobs(ctx); err != nil {
		t.Fatalf("RunDueJobs: %v", err)
	}
	var idopontID string
	for _, dm := range w.h.Discord.DMsTo("d-anna") {
		if dm.Hatarido != nil {
			idopontID = dm.Hatarido.Idopontok[1].ID
		}
	}
	if idopontID == "" {
		t.Fatal("Anna got no hatarido DM")
	}

	expectEphemeral(t, w.router.Route(ctx, confirmButton(t, "d-bela", e.ID, idopontID)).Response, "Ezt csak az esemény szervezője teheti meg.")
	d, _ := w.h.App.Esemeny(ctx, "anna", e.ID)
	if d.Allapot != domain.Egyeztetes {
		t.Fatalf("allapot after Bela's press = %s, want Egyeztetes", d.Allapot)
	}

	expectEphemeral(t, w.router.Route(ctx, confirmButton(t, "d-anna", e.ID, idopontID)).Response, "✅ Véglegesítetted az időpontot.")
	d, _ = w.h.App.Esemeny(ctx, "anna", e.ID)
	if d.Allapot != domain.Veglegesitve || d.ConfirmedIdopont != idopontID {
		t.Fatalf("esemeny = %+v, want Veglegesitve on the pressed Idopont", d.Esemeny)
	}

	// Pressing another one afterwards is refused.
	expectEphemeral(t, w.router.Route(ctx, confirmButton(t, "d-anna", e.ID, d.Idopontok[0].ID)).Response, "már nincs egyeztetés alatt")
	expectEphemeral(t, w.router.Route(ctx, confirmButton(t, "d-anna", e.ID, "nope")).Response, "nem található")
}
