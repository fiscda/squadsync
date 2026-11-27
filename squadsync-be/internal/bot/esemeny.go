package bot

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/bwmarrin/discordgo"

	"squadsync-be/internal/app"
	"squadsync-be/internal/domain"
	"squadsync-be/internal/gateway"
)

// An Esemeny's message: an embed with its details and each Idopont's
// counts and Helyezes, and a row of igen/talán/nem buttons under each
// answerable Idopont. The application core decides what it says
// (gateway.EsemenyMessage); this file only lays it out, for the Messenger
// and for the reply to a button press.

// visszajelzesRoute is the route of the answer buttons. Their custom id is
// CustomID(visszajelzesRoute, esemenyID, idopontID, valasz): with 32-character
// hex ids it is at most 74 characters, under Discord's 100.
const visszajelzesRoute = "vj"

// Replies to an answer button.
const (
	visszajelzesNotTag        = "Nem vagy tagja ennek a csoportnak, ezért nem válaszolhatsz erre az eseményre."
	visszajelzesNotAnswerable = "Erre az időpontra most nem lehet válaszolni."
)

// Layout of the Esemeny's message.
const (
	webButtonLabel    = "Válaszolj a weboldalon"
	noVisszajelzesYet = "Még senki nem válaszolt."
	helyszinMapsOnly  = "Térkép"
	numberSeparator   = " · "
	embedColor        = 0x5865F2

	// Discord's limits on an embed.
	maxEmbedTitleLength       = 256
	maxEmbedDescriptionLength = 4096
	maxEmbedFieldNameLength   = 256
	maxEmbedFieldValueLength  = 1024
	maxEmbedFooterLength      = 2048
)

var valaszButtons = []struct {
	valasz domain.Valasz
	label  string
	style  discordgo.ButtonStyle
}{
	{domain.Igen, "Igen", discordgo.SuccessButton},
	{domain.Talan, "Talán", discordgo.SecondaryButton},
	{domain.Nem, "Nem", discordgo.DangerButton},
}

// renderEsemeny lays out an Esemeny's message. Components is never nil, so
// an edit that leaves no buttons removes the old ones.
func renderEsemeny(m gateway.EsemenyMessage) ([]*discordgo.MessageEmbed, []discordgo.MessageComponent, error) {
	embed := &discordgo.MessageEmbed{
		Title:       truncate(m.Title, maxEmbedTitleLength),
		URL:         m.URL,
		Description: truncate(m.Description, maxEmbedDescriptionLength),
		Color:       embedColor,
		Author:      &discordgo.MessageEmbedAuthor{Name: truncate(m.CsoportName, maxEmbedFieldNameLength)},
		Fields: []*discordgo.MessageEmbedField{
			{Name: "Szervező", Value: orDash(m.SzervezoName), Inline: true},
			{Name: "Állapot", Value: orDash(m.Allapot), Inline: true},
		},
	}
	if h := helyszin(m); h != "" {
		embed.Fields = append(embed.Fields, &discordgo.MessageEmbedField{Name: "Helyszín", Value: truncate(h, maxEmbedFieldValueLength), Inline: true})
	}
	for n, i := range m.Idopontok {
		embed.Fields = append(embed.Fields, &discordgo.MessageEmbedField{
			Name:  truncate(fmt.Sprintf("%d%s%s", n+1, numberSeparator, i.Label), maxEmbedFieldNameLength),
			Value: allas(i),
		})
	}
	if m.Note != "" {
		embed.Footer = &discordgo.MessageEmbedFooter{Text: truncate(m.Note, maxEmbedFooterLength)}
	}

	components := []discordgo.MessageComponent{}
	for n, i := range m.Idopontok {
		if !i.Answerable {
			continue
		}
		row := discordgo.ActionsRow{}
		for _, b := range valaszButtons {
			id, err := CustomID(visszajelzesRoute, m.EsemenyID, i.ID, string(b.valasz))
			if err != nil {
				return nil, nil, err
			}
			row.Components = append(row.Components, discordgo.Button{
				Label:    fmt.Sprintf("%d%s%s", n+1, numberSeparator, b.label),
				Style:    b.style,
				CustomID: id,
			})
		}
		components = append(components, row)
	}
	if m.WebButton {
		components = append(components, discordgo.ActionsRow{Components: []discordgo.MessageComponent{
			discordgo.Button{Label: webButtonLabel, Style: discordgo.LinkButton, URL: m.URL},
		}})
	}
	if len(components) > gateway.MaxAnswerRows {
		return nil, nil, fmt.Errorf("bot: esemeny message %s has %d button rows, Discord allows %d", m.EsemenyID, len(components), gateway.MaxAnswerRows)
	}
	return []*discordgo.MessageEmbed{embed}, components, nil
}

func helyszin(m gateway.EsemenyMessage) string {
	switch {
	case m.HelyszinURL != "" && m.HelyszinName != "":
		return fmt.Sprintf("[%s](%s)", escapeLinkText(m.HelyszinName), m.HelyszinURL)
	case m.HelyszinURL != "":
		return fmt.Sprintf("[%s](%s)", helyszinMapsOnly, m.HelyszinURL)
	default:
		return m.HelyszinName
	}
}

// allas shows an Idopont's counts and Helyezes.
func allas(i gateway.IdopontLine) string {
	if i.Igen+i.Talan+i.Nem == 0 {
		return noVisszajelzesYet
	}
	s := fmt.Sprintf("✅ %d igen · 🤔 %d talán · ❌ %d nem", i.Igen, i.Talan, i.Nem)
	if i.Helyezes > 0 {
		s += fmt.Sprintf(" · 🏆 %d. hely", i.Helyezes)
	}
	return s
}

func escapeLinkText(s string) string {
	return strings.NewReplacer("[", "(", "]", ")").Replace(s)
}

func orDash(s string) string {
	if s == "" {
		return "–"
	}
	return s
}

// truncate cuts s to at most n runes.
func truncate(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n-1]) + "…"
}

// answerButton records the Visszajelzes of an answer button and updates the
// message it is on.
func (r *Router) answerButton(ctx context.Context, req Request) (Reply, error) {
	if len(req.Args) != 3 {
		return ephemeral(unknownCommand), nil
	}
	esemenyID, idopontID, valasz := req.Args[0], req.Args[1], domain.Valasz(req.Args[2])
	err := r.app.SetVisszajelzes(ctx, req.Felhasznalo.ID, esemenyID, idopontID, valasz)
	switch {
	case errors.Is(err, app.ErrNotFound):
		return ephemeral(visszajelzesNotTag), nil
	case errors.Is(err, app.ErrIdopontNotAnswerable):
		return ephemeral(visszajelzesNotAnswerable), nil
	case errors.Is(err, app.ErrInvalidInput):
		return ephemeral(unknownCommand), nil
	case err != nil:
		return Reply{}, err
	}

	m, err := r.app.EsemenyMessage(ctx, req.Felhasznalo.ID, esemenyID)
	if err != nil {
		return Reply{}, err
	}
	embeds, components, err := renderEsemeny(m)
	if err != nil {
		return Reply{}, err
	}
	data := &discordgo.InteractionResponseData{
		Embeds:          embeds,
		Components:      components,
		AllowedMentions: &discordgo.MessageAllowedMentions{},
	}
	// An update replaces the content too: keep a DM's text above the embed.
	if req.Interaction.Message != nil {
		data.Content = req.Interaction.Message.Content
	}
	return Reply{Response: &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseUpdateMessage,
		Data: data,
	}}, nil
}

// PostEsemenyMessage implements gateway.DiscordMessenger.
func (m Messenger) PostEsemenyMessage(ctx context.Context, channelID string, msg gateway.EsemenyMessage) (gateway.MessageRef, error) {
	return m.sendEsemeny(ctx, channelID, "", msg)
}

// SendEsemenyDM implements gateway.DiscordMessenger.
func (m Messenger) SendEsemenyDM(ctx context.Context, discordID, content string, msg gateway.EsemenyMessage) (gateway.MessageRef, error) {
	ch, err := m.Session.UserChannelCreate(discordID, discordgo.WithContext(ctx))
	if err != nil {
		return gateway.MessageRef{}, fmt.Errorf("open dm channel: %w", err)
	}
	return m.sendEsemeny(ctx, ch.ID, content, msg)
}

func (m Messenger) sendEsemeny(ctx context.Context, channelID, content string, msg gateway.EsemenyMessage) (gateway.MessageRef, error) {
	embeds, components, err := renderEsemeny(msg)
	if err != nil {
		return gateway.MessageRef{}, err
	}
	sent, err := m.Session.ChannelMessageSendComplex(channelID, &discordgo.MessageSend{
		Content:         content,
		Embeds:          embeds,
		Components:      components,
		AllowedMentions: &discordgo.MessageAllowedMentions{},
	}, discordgo.WithContext(ctx))
	if err != nil {
		return gateway.MessageRef{}, fmt.Errorf("send esemeny message: %w", err)
	}
	return gateway.MessageRef{ChannelID: sent.ChannelID, MessageID: sent.ID}, nil
}

// EditEsemenyMessage implements gateway.DiscordMessenger. It leaves the
// message's text content alone.
func (m Messenger) EditEsemenyMessage(ctx context.Context, ref gateway.MessageRef, msg gateway.EsemenyMessage) error {
	embeds, components, err := renderEsemeny(msg)
	if err != nil {
		return err
	}
	edit := discordgo.NewMessageEdit(ref.ChannelID, ref.MessageID)
	edit.Embeds = &embeds
	edit.Components = &components
	edit.AllowedMentions = &discordgo.MessageAllowedMentions{}
	if _, err := m.Session.ChannelMessageEditComplex(edit, discordgo.WithContext(ctx)); err != nil {
		return fmt.Errorf("edit esemeny message: %w", err)
	}
	return nil
}
