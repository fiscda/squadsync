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
// (gateway.EsemenyMessage); this file lays it out for the Messenger and
// escapes the user-given text, all in one place.
//
// # One writer per message
//
// Once sent, an Esemeny's message is written only by the core's refresh
// job, which runs after every change, one job at a time. A button that
// changes the Esemeny therefore answers with deferredUpdate (acknowledge,
// change nothing) and never renders the message itself; otherwise its reply
// could overwrite a newer edit. Future buttons on the Esemeny message must
// follow the same rule.

// visszajelzesRoute is the route of the answer buttons. Their custom id is
// CustomID(visszajelzesRoute, esemenyID, idopontID, valasz): with 32-character
// hex ids it is at most 74 characters, under Discord's 100.
const visszajelzesRoute = "vj"

// Replies to an answer button.
const (
	visszajelzesNotTag        = "Nem vagy tagja ennek a csoportnak, ezért nem válaszolhatsz erre az eseményre."
	visszajelzesNotAnswerable = "Erre az időpontra most nem lehet válaszolni."
)

// Text of a vote-request DM.
const (
	voteRequestFormat = "Új esemény a(z) **%s** csoportban: **%s** – válaszolj!"
	channelLinkFormat = "\nÜzenet a csatornában: %s"
	webLinkFormat     = "\nWeboldal: %s"
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
	maxEmbedAuthorLength      = 256
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

// markdownEscaper backslash-escapes everything Discord's markdown reacts to,
// so user-given names show as typed.
var markdownEscaper = strings.NewReplacer(
	`\`, `\\`, `*`, `\*`, `_`, `\_`, "~", `\~`, "`", "\\`", `|`, `\|`,
	`>`, `\>`, `#`, `\#`, `-`, `\-`, `[`, `\[`, `]`, `\]`, `(`, `\(`, `)`, `\)`,
)

// escapeMarkdown makes user-given text show literally in Discord.
func escapeMarkdown(s string) string { return markdownEscaper.Replace(s) }

// field is plain text cut to at most n characters once escaped.
func field(s string, n int) string {
	runes := []rune(s)
	for len(runes) > 0 {
		e := escapeMarkdown(string(runes))
		if len([]rune(e)) <= n {
			return e
		}
		runes = runes[:len(runes)-1]
	}
	return ""
}

// voteRequestText words a vote request.
func voteRequestText(r gateway.VoteRequest) string {
	s := fmt.Sprintf(voteRequestFormat, escapeMarkdown(r.CsoportName), escapeMarkdown(r.Title))
	if r.ChannelMessageURL != "" {
		s += fmt.Sprintf(channelLinkFormat, r.ChannelMessageURL)
	}
	if r.WebURL != "" {
		s += fmt.Sprintf(webLinkFormat, r.WebURL)
	}
	return s
}

// renderEsemeny lays out an Esemeny's message. Components is never nil, so
// an edit that leaves no buttons removes the old ones.
func renderEsemeny(m gateway.EsemenyMessage) ([]*discordgo.MessageEmbed, []discordgo.MessageComponent, error) {
	embed := &discordgo.MessageEmbed{
		Title:       field(m.Title, maxEmbedTitleLength),
		URL:         m.URL,
		Description: truncate(m.Description, maxEmbedDescriptionLength),
		Color:       embedColor,
		// An author name shows as plain text: no escaping.
		Author: &discordgo.MessageEmbedAuthor{Name: truncate(m.CsoportName, maxEmbedAuthorLength)},
		Fields: []*discordgo.MessageEmbedField{
			{Name: "Szervező", Value: orDash(field(m.SzervezoName, maxEmbedFieldValueLength)), Inline: true},
			{Name: "Állapot", Value: orDash(m.Allapot), Inline: true},
		},
	}
	if h := helyszin(m); h != "" {
		embed.Fields = append(embed.Fields, &discordgo.MessageEmbedField{Name: "Helyszín", Value: h, Inline: true})
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
	if m.WebButton && m.URL != "" {
		components = append(components, discordgo.ActionsRow{Components: []discordgo.MessageComponent{
			discordgo.Button{Label: webButtonLabel, Style: discordgo.LinkButton, URL: m.URL},
		}})
	}
	if len(components) > gateway.MaxAnswerRows {
		return nil, nil, fmt.Errorf("bot: esemeny message %s has %d button rows, Discord allows %d", m.EsemenyID, len(components), gateway.MaxAnswerRows)
	}
	return []*discordgo.MessageEmbed{embed}, components, nil
}

// helyszin shows the Helyszin, its name linked to its Maps URL. A link that
// wouldn't fit in a field is dropped rather than cut in half.
func helyszin(m gateway.EsemenyMessage) string {
	name := escapeMarkdown(m.HelyszinName)
	if m.HelyszinURL == "" {
		return field(m.HelyszinName, maxEmbedFieldValueLength)
	}
	text := name
	if text == "" {
		text = helyszinMapsOnly
	}
	// Parentheses would end the link early.
	link := fmt.Sprintf("[%s](%s)", text, strings.NewReplacer("(", "%28", ")", "%29").Replace(m.HelyszinURL))
	if len([]rune(link)) <= maxEmbedFieldValueLength {
		return link
	}
	return field(m.HelyszinName, maxEmbedFieldValueLength)
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

// answerButton records the Visszajelzes of an answer button. It only
// acknowledges the press: the core's refresh job updates the message (see
// "One writer per message" above).
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
	return deferredUpdate(), nil
}

// deferredUpdate acknowledges a button press without changing its message,
// and without anything to follow up.
func deferredUpdate() Reply {
	return Reply{Response: &discordgo.InteractionResponse{Type: discordgo.InteractionResponseDeferredMessageUpdate}}
}

// SendVoteRequestDM implements gateway.DiscordMessenger.
func (m Messenger) SendVoteRequestDM(ctx context.Context, discordID string, r gateway.VoteRequest) error {
	return m.SendDM(ctx, discordID, voteRequestText(r))
}

// PostEsemenyMessage implements gateway.DiscordMessenger.
func (m Messenger) PostEsemenyMessage(ctx context.Context, channelID string, msg gateway.EsemenyMessage) (gateway.MessageRef, error) {
	return m.sendEsemeny(ctx, channelID, "", msg)
}

// SendEsemenyDM implements gateway.DiscordMessenger.
func (m Messenger) SendEsemenyDM(ctx context.Context, discordID string, r gateway.VoteRequest, msg gateway.EsemenyMessage) (gateway.MessageRef, error) {
	ch, err := m.Session.UserChannelCreate(discordID, discordgo.WithContext(ctx))
	if err != nil {
		return gateway.MessageRef{}, fmt.Errorf("open dm channel: %w", err)
	}
	return m.sendEsemeny(ctx, ch.ID, voteRequestText(r), msg)
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
