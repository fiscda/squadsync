package bot_test

import (
	"context"
	"testing"

	"github.com/bwmarrin/discordgo"

	"squadsync-be/internal/app"
	"squadsync-be/internal/bot"
	"squadsync-be/internal/domain"
	"squadsync-be/internal/gateway"
)

// megvoltButton is a press of a "megvolt" button in the result DM.
func megvoltButton(t *testing.T, discordID, esemenyID, utalasID string) *discordgo.Interaction {
	t.Helper()
	id, err := bot.CustomID("megvolt", esemenyID, utalasID)
	if err != nil {
		t.Fatalf("CustomID: %v", err)
	}
	return &discordgo.Interaction{
		Type: discordgo.InteractionMessageComponent,
		User: &discordgo.User{ID: discordID},
		Data: discordgo.MessageComponentInteractionData{CustomID: id, ComponentType: discordgo.ButtonComponent},
	}
}

func TestMegvoltButtonMarksTheUtalas(t *testing.T) {
	w := newWorld(t)
	ctx := context.Background()
	a := w.h.App
	e, err := a.ProposeEsemeny(ctx, "anna", w.csoport.ID, app.EsemenyInput{
		Title:     "Társasjáték",
		Idopontok: []app.IdopontInput{{Start: "2026-12-05T20:00"}},
	})
	if err != nil {
		t.Fatalf("ProposeEsemeny: %v", err)
	}
	d, _ := a.Esemeny(ctx, "anna", e.ID)
	for _, who := range []string{"anna", "bela"} {
		if err := a.SetVisszajelzes(ctx, who, e.ID, d.Idopontok[0].ID, domain.Igen); err != nil {
			t.Fatalf("SetVisszajelzes: %v", err)
		}
	}
	if err := a.ConfirmIdopont(ctx, "anna", e.ID, d.Idopontok[0].ID); err != nil {
		t.Fatalf("ConfirmIdopont: %v", err)
	}
	if err := a.SetKiegeszito(ctx, "anna", e.ID, domain.Koltsegek, true); err != nil {
		t.Fatalf("SetKiegeszito: %v", err)
	}
	if err := w.h.Store.UpdateEsemenyAllapot(ctx, e.ID, domain.Lezajlott, d.Idopontok[0].ID); err != nil {
		t.Fatalf("UpdateEsemenyAllapot: %v", err)
	}
	if _, err := a.AddKiadas(ctx, "anna", e.ID, app.KiadasInput{Description: "Pizza", Amount: 8000}); err != nil {
		t.Fatalf("AddKiadas: %v", err)
	}
	for _, who := range []string{"anna", "bela"} {
		if err := a.ConfirmKiadasokRecorded(ctx, who, e.ID); err != nil {
			t.Fatalf("ConfirmKiadasokRecorded(%s): %v", who, err)
		}
	}

	var dm *gateway.ElszamolasDM
	for _, m := range w.h.Discord.DMsTo("d-bela") {
		if m.Elszamolas != nil {
			dm = m.Elszamolas
		}
	}
	if dm == nil || len(dm.Fizetendo) != 1 || dm.Fizetendo[0].Amount != 4000 {
		t.Fatalf("Bela's result DM = %+v, want him paying 4000", dm)
	}
	utalasID := dm.Fizetendo[0].UtalasID

	expectEphemeral(t, w.router.Route(ctx, megvoltButton(t, "d-bela", e.ID, "nope")).Response, "nem található")
	expectEphemeral(t, w.router.Route(ctx, megvoltButton(t, "d-bela", e.ID, utalasID)).Response, "✅ Megjelölted")
	k, err := a.Koltsegek(ctx, "anna", e.ID)
	if err != nil {
		t.Fatalf("Koltsegek: %v", err)
	}
	if k.Elszamolas.Allapot != app.ElszamolasElszamolva || k.Elszamolas.Utalasok[0].MegvoltBy != "bela" {
		t.Fatalf("elszamolas = %+v, want it elszamolva by Bela's press", k.Elszamolas)
	}
	// Pressing it again is fine.
	expectEphemeral(t, w.router.Route(ctx, megvoltButton(t, "d-bela", e.ID, utalasID)).Response, "✅ Megjelölted")
}
