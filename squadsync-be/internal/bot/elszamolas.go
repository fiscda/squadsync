package bot

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/bwmarrin/discordgo"

	"squadsync-be/internal/app"
	"squadsync-be/internal/gateway"
)

// The Elszamolas DMs: the reminder to confirm the Kiadasok, and each
// Felhasznalo's result once it is calculated, with a "megvolt" button on
// every Utalas they pay.

// megvoltRoute is the route of the "megvolt" buttons of the result DM.
// Their custom id is CustomID(megvoltRoute, esemenyID, utalasID): with
// 32-character hex ids it is 73 characters, under Discord's 100.
const megvoltRoute = "megvolt"

// Text of the Elszamolas DMs.
const (
	elszamolasEmlekeztetoFormat = "🧾 Ne felejtsd el: **%s** (%s) elszámolásához rögzítsd a kiadásaidat, majd nyomd meg a „Rögzítettem a kiadásaimat” gombot a weboldalon."
	elszamolasFormat            = "🧾 Elkészült az elszámolás: **%s** (%s)"
	elszamolasFizetendo         = "\nEzt kell utalnod:"
	elszamolasKapando           = "\nEzt kapod:"
	elszamolasLineFormat        = "\n• %s: %s"
	elszamolasRendben           = "\nRendben vagy: nem kell utalnod, és neked sem utal senki."
	elszamolasButtonsHint       = "\nHa megvolt egy utalás, nyomd meg a gombját."
	elszamolasWebHint           = "\nAz utalásokat a weboldalon jelölheted megvoltnak."
	megvoltButtonFormat         = "Megvolt: %s, %s"
	maxDMLength                 = 2000
)

// Replies to a "megvolt" button.
const (
	megvoltDone        = "✅ Megjelölted: ez az utalás megvolt."
	megvoltNotFound    = "Ez az utalás már nem található: lehet, hogy a szervező újranyitotta az elszámolást, vagy már nem vagy tagja a csoportnak."
	megvoltNotAllowed  = "Ezt csak az utalás két fele vagy a szervező jelölheti megvoltnak."
	megvoltKiegeszitoK = "A Költségek közben ki lett kapcsolva."
)

// formatFt words whole forints with Hungarian grouping: "12 000 Ft".
func formatFt(amount int) string {
	sign := ""
	if amount < 0 {
		sign, amount = "-", -amount
	}
	digits := strconv.Itoa(amount)
	var b strings.Builder
	for n, d := range digits {
		if n > 0 && (len(digits)-n)%3 == 0 {
			b.WriteByte(' ')
		}
		b.WriteRune(d)
	}
	return sign + b.String() + " Ft"
}

// elszamolasEmlekeztetoText words the reminder to confirm the Kiadasok.
func elszamolasEmlekeztetoText(e gateway.ElszamolasEmlekezteto) string {
	s := fmt.Sprintf(elszamolasEmlekeztetoFormat, escapeMarkdown(e.Title), escapeMarkdown(e.CsoportName))
	if e.Link != "" {
		s += fmt.Sprintf(linkFormat, e.Link)
	}
	return s
}

// renderElszamolas lays out a Felhasznalo's result DM: whom they pay and
// who pays them, then a "megvolt" button per Utalas they pay, five a row,
// or the web hint when there are more than fit.
func renderElszamolas(d gateway.ElszamolasDM) (string, []discordgo.MessageComponent, error) {
	var b strings.Builder
	fmt.Fprintf(&b, elszamolasFormat, escapeMarkdown(d.Title), escapeMarkdown(d.CsoportName))
	if len(d.Fizetendo) == 0 && len(d.Kapando) == 0 {
		b.WriteString(elszamolasRendben)
	}
	if len(d.Fizetendo) > 0 {
		b.WriteString(elszamolasFizetendo)
		for _, u := range d.Fizetendo {
			fmt.Fprintf(&b, elszamolasLineFormat, escapeMarkdown(u.Name), formatFt(u.Amount))
		}
	}
	if len(d.Kapando) > 0 {
		b.WriteString(elszamolasKapando)
		for _, u := range d.Kapando {
			fmt.Fprintf(&b, elszamolasLineFormat, escapeMarkdown(u.Name), formatFt(u.Amount))
		}
	}
	buttons := len(d.Fizetendo) > 0 && len(d.Fizetendo) <= gateway.MaxMegvoltButtons
	switch {
	case buttons:
		b.WriteString(elszamolasButtonsHint)
	case len(d.Fizetendo) > 0:
		b.WriteString(elszamolasWebHint)
	}
	if d.Link != "" {
		fmt.Fprintf(&b, linkFormat, d.Link)
	}
	content := truncate(b.String(), maxDMLength)

	components := []discordgo.MessageComponent{}
	if !buttons {
		return content, components, nil
	}
	var row []discordgo.MessageComponent
	for _, u := range d.Fizetendo {
		id, err := CustomID(megvoltRoute, d.EsemenyID, u.UtalasID)
		if err != nil {
			return "", nil, err
		}
		row = append(row, discordgo.Button{
			Label:    truncate(fmt.Sprintf(megvoltButtonFormat, u.Name, formatFt(u.Amount)), maxButtonLabelLength),
			Style:    discordgo.SuccessButton,
			CustomID: id,
		})
		if len(row) == 5 {
			components = append(components, discordgo.ActionsRow{Components: row})
			row = nil
		}
	}
	if len(row) > 0 {
		components = append(components, discordgo.ActionsRow{Components: row})
	}
	return content, components, nil
}

// megvoltButton marks an Utalas megvolt from the result DM, as whoever
// pressed it: either party or the Szervezo. It answers ephemerally; the
// core re-renders the Esemeny's messages.
func (r *Router) megvoltButton(ctx context.Context, req Request) (Reply, error) {
	if len(req.Args) != 2 {
		return ephemeral(unknownCommand), nil
	}
	esemenyID, utalasID := req.Args[0], req.Args[1]
	err := r.app.SetUtalasMegvolt(ctx, req.Felhasznalo.ID, esemenyID, utalasID, true)
	switch {
	case errors.Is(err, app.ErrNotFound):
		return ephemeral(megvoltNotFound), nil
	case errors.Is(err, app.ErrNotPermitted):
		return ephemeral(megvoltNotAllowed), nil
	case errors.Is(err, app.ErrKiegeszitoOff):
		return ephemeral(megvoltKiegeszitoK), nil
	case err != nil:
		return Reply{}, err
	}
	return ephemeral(megvoltDone), nil
}

// SendElszamolasEmlekeztetoDM implements gateway.DiscordMessenger.
func (m Messenger) SendElszamolasEmlekeztetoDM(ctx context.Context, discordID string, e gateway.ElszamolasEmlekezteto) error {
	return m.SendDM(ctx, discordID, elszamolasEmlekeztetoText(e))
}

// SendElszamolasDM implements gateway.DiscordMessenger.
func (m Messenger) SendElszamolasDM(ctx context.Context, discordID string, d gateway.ElszamolasDM) error {
	content, components, err := renderElszamolas(d)
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
		return fmt.Errorf("send elszamolas dm: %w", err)
	}
	return nil
}
