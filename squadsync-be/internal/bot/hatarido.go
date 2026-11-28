package bot

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/bwmarrin/discordgo"

	"squadsync-be/internal/app"
	"squadsync-be/internal/gateway"
)

// The DMs of timed jobs: an Emlekezteto before a confirmed Esemeny, and the
// Szervezo's DM when the voting Hatarido passed, with a confirm button per
// Idopont. The DM isn't an Esemeny message, so a confirm button may answer
// on its own; the Esemeny's messages are re-rendered by the core.

// veglegesitesRoute is the route of the confirm buttons of the Hatarido DM.
// Their custom id is CustomID(veglegesitesRoute, esemenyID, idopontID): with
// 32-character hex ids it is 70 characters, under Discord's 100.
const veglegesitesRoute = "vegl"

// Text of the timed DMs.
const (
	emlekeztetoFormat    = "⏰ Emlékeztető: %s – %s (%s)"
	hataridoFormat       = "⏳ Lejárt a szavazási határidő: **%s** (%s)\nVálaszd ki a végleges időpontot. A szavazás továbbra is nyitva marad."
	hataridoLineFormat   = "\n%d%s%s – %s"
	hataridoWebFormat    = "\nTúl sok időpont van a gombokhoz, véglegesíteni a weboldalon tudsz: %s"
	hataridoWebNoLink    = "\nTúl sok időpont van a gombokhoz, véglegesíteni a weboldalon tudsz."
	confirmButtonFormat  = "Véglegesítés: %d%s%s"
	maxButtonLabelLength = 80
)

// Replies to a confirm button.
const (
	veglegesitesDone       = "✅ Véglegesítetted az időpontot. Mindenki értesítést kap róla."
	veglegesitesNotFound   = "Ez az esemény vagy időpont nem található, vagy már nem vagy tagja a csoportnak."
	veglegesitesNotSzerv   = "Ezt csak az esemény szervezője teheti meg."
	veglegesitesNotAllowed = "Ez most nem lehetséges: az esemény már nincs egyeztetés alatt."
)

// emlekeztetoText words an Emlekezteto.
func emlekeztetoText(e gateway.Emlekezteto) string {
	s := fmt.Sprintf(emlekeztetoFormat, escapeMarkdown(e.Title), e.Idopont, escapeMarkdown(e.CsoportName))
	if e.Link != "" {
		s += fmt.Sprintf(linkFormat, e.Link)
	}
	return s
}

// renderHatarido lays out the Hatarido DM: every Idopont ranked, then a
// confirm button per Idopont, one row each, or the web link.
func renderHatarido(h gateway.HataridoDM) (string, []discordgo.MessageComponent, error) {
	var b strings.Builder
	fmt.Fprintf(&b, hataridoFormat, escapeMarkdown(h.Title), escapeMarkdown(h.CsoportName))
	for n, i := range h.Idopontok {
		place := "nincs helyezés"
		if i.Helyezes > 0 {
			place = fmt.Sprintf("%d. hely", i.Helyezes)
		}
		fmt.Fprintf(&b, hataridoLineFormat, n+1, numberSeparator, i.Label,
			fmt.Sprintf("🏆 %s · ✅ %d igen · 🤔 %d talán · ❌ %d nem", place, i.Igen, i.Talan, i.Nem))
	}
	components := []discordgo.MessageComponent{}
	if !h.ConfirmButtons || len(h.Idopontok) > gateway.MaxAnswerRows {
		if h.Link != "" {
			fmt.Fprintf(&b, hataridoWebFormat, h.Link)
		} else {
			b.WriteString(hataridoWebNoLink)
		}
		return b.String(), components, nil
	}
	if h.Link != "" {
		fmt.Fprintf(&b, linkFormat, h.Link)
	}
	for n, i := range h.Idopontok {
		id, err := CustomID(veglegesitesRoute, h.EsemenyID, i.ID)
		if err != nil {
			return "", nil, err
		}
		components = append(components, discordgo.ActionsRow{Components: []discordgo.MessageComponent{
			discordgo.Button{
				Label:    truncate(fmt.Sprintf(confirmButtonFormat, n+1, numberSeparator, i.Label), maxButtonLabelLength),
				Style:    discordgo.SuccessButton,
				CustomID: id,
			},
		}})
	}
	return b.String(), components, nil
}

// confirmButton confirms an Idopont from the Hatarido DM, as whoever
// pressed it: only the Szervezo can. It answers ephemerally.
func (r *Router) confirmButton(ctx context.Context, req Request) (Reply, error) {
	if len(req.Args) != 2 {
		return ephemeral(unknownCommand), nil
	}
	esemenyID, idopontID := req.Args[0], req.Args[1]
	err := r.app.ConfirmIdopont(ctx, req.Felhasznalo.ID, esemenyID, idopontID)
	switch {
	case errors.Is(err, app.ErrNotFound):
		return ephemeral(veglegesitesNotFound), nil
	case errors.Is(err, app.ErrNotSzervezo):
		return ephemeral(veglegesitesNotSzerv), nil
	case errors.Is(err, app.ErrInvalidTransition):
		return ephemeral(veglegesitesNotAllowed), nil
	case err != nil:
		return Reply{}, err
	}
	return ephemeral(veglegesitesDone), nil
}

// SendEmlekeztetoDM implements gateway.DiscordMessenger.
func (m Messenger) SendEmlekeztetoDM(ctx context.Context, discordID string, e gateway.Emlekezteto) error {
	return m.SendDM(ctx, discordID, emlekeztetoText(e))
}

// SendHataridoDM implements gateway.DiscordMessenger.
func (m Messenger) SendHataridoDM(ctx context.Context, discordID string, h gateway.HataridoDM) error {
	content, components, err := renderHatarido(h)
	if err != nil {
		return err
	}
	ch, err := m.Session.UserChannelCreate(discordID, discordgo.WithContext(ctx))
	if err != nil {
		return fmt.Errorf("open dm channel: %w", err)
	}
	if _, err := m.Session.ChannelMessageSendComplex(ch.ID, &discordgo.MessageSend{
		Content:         content,
		Components:      components,
		AllowedMentions: &discordgo.MessageAllowedMentions{},
		Flags:           discordgo.MessageFlagsSuppressEmbeds,
	}, discordgo.WithContext(ctx)); err != nil {
		return fmt.Errorf("send hatarido dm: %w", err)
	}
	return nil
}
