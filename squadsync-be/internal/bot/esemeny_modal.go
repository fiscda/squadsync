package bot

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/bwmarrin/discordgo"

	"squadsync-be/internal/app"
	"squadsync-be/internal/domain"
)

// The /esemeny command proposes an Esemeny from Discord. It takes the
// Csoport, then opens a modal for the details; submitting it proposes the
// Esemeny exactly as the web does, so the channel post and the DMs follow
// from the core.
//
// The modal's custom id is CustomID(esemenyModalRoute, csoportID): with a
// 32-character hex id, 41 characters, under Discord's 100. A Csoport id
// that doesn't fit is refused when the modal would open.
const (
	esemenyCommandName = "esemeny"
	esemenyOptionName  = "csoport"
	esemenyModalRoute  = "esemeny"

	// Discord's limits on a modal.
	maxModalTitleLength = 45
	maxChoiceNameLength = 100
)

// The text inputs of the modal; Discord allows at most 5.
const (
	inputCim       = "cim"
	inputHelyszin  = "helyszin"
	inputTerkep    = "terkep"
	inputLeiras    = "leiras"
	inputIdopontok = "idopontok"
)

// Replies of /esemeny.
const (
	esemenyNotFound      = "Nem találom ezt a csoportot. Válassz a listából egy csoportot, ahol tag vagy."
	esemenyNotTag        = "Nem vagy tagja ennek a csoportnak, ezért nem javasolhatsz eseményt."
	esemenyNeedsTitle    = "Adj meg egy címet az eseménynek."
	esemenyBadInput      = "Valami nem jó a megadott adatok között. A térkép linknek http:// vagy https:// címnek kell lennie, a cím legfeljebb %d, a helyszín legfeljebb %d, a leírás legfeljebb %d karakter lehet."
	esemenyNoSuchTime    = "Az egyik időpont nem létezik az óraátállítás miatt (ekkor az óra ugrik egyet előre). Válassz másik időpontot."
	esemenyCreated       = "Létrehozva: **%s**"
	esemenyCreatedLink   = "\nWeboldal: <%s>"
	esemenyCreatedNoLink = "\nWeboldal: %s"
	idopontokPlaceholder = "Soronként egy, legfeljebb 5:\n12.05 20:00\n12.05 20:00-23:00\n12.05-12.07\n12.05 18:00-12.07 14:00"
)

func (r *Router) esemenyCommand() Command {
	return Command{
		Definition: &discordgo.ApplicationCommand{
			Name:        esemenyCommandName,
			Description: "Új esemény javaslata a csoportodnak.",
			Options: []*discordgo.ApplicationCommandOption{{
				Type:         discordgo.ApplicationCommandOptionString,
				Name:         esemenyOptionName,
				Description:  "A csoport, amelynek az eseményt javaslod",
				Required:     true,
				Autocomplete: true,
			}},
		},
		Run:          r.openEsemenyModal,
		Autocomplete: r.esemenyCsoportChoices,
	}
}

// esemenyCsoportChoices offers every Csoport the caller is a Tag of,
// filtered by what they've typed so far.
func (r *Router) esemenyCsoportChoices(ctx context.Context, req Request) (Reply, error) {
	typed := strings.ToLower(strings.TrimSpace(optionValue(req.Interaction.ApplicationCommandData().Options, esemenyOptionName)))
	csoportok, err := r.app.CsoportokOf(ctx, req.Felhasznalo.ID)
	if err != nil {
		return Reply{}, err
	}
	var choices []*discordgo.ApplicationCommandOptionChoice
	for _, c := range csoportok {
		if len(choices) == maxChoices {
			break
		}
		if strings.Contains(strings.ToLower(c.Name), typed) {
			choices = append(choices, &discordgo.ApplicationCommandOptionChoice{Name: truncate(c.Name, maxChoiceNameLength), Value: c.ID})
		}
	}
	return autocomplete(choices), nil
}

// openEsemenyModal answers /esemeny with the modal, after checking that the
// caller is a Tag of the Csoport.
func (r *Router) openEsemenyModal(ctx context.Context, req Request) (Reply, error) {
	csoportID := optionValue(req.Interaction.ApplicationCommandData().Options, esemenyOptionName)
	csoportok, err := r.app.CsoportokOf(ctx, req.Felhasznalo.ID)
	if err != nil {
		return Reply{}, err
	}
	var csoport *domain.CsoportSummary
	for n := range csoportok {
		if csoportok[n].ID == csoportID {
			csoport = &csoportok[n]
		}
	}
	if csoport == nil {
		return ephemeral(esemenyNotFound), nil
	}
	id, err := CustomID(esemenyModalRoute, csoport.ID)
	if err != nil {
		return Reply{}, err
	}
	return Reply{Response: &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseModal,
		Data: &discordgo.InteractionResponseData{
			CustomID:   id,
			Title:      truncate("Új esemény: "+csoport.Name, maxModalTitleLength),
			Components: esemenyModalInputs(),
		},
	}}, nil
}

func esemenyModalInputs() []discordgo.MessageComponent {
	row := func(in discordgo.TextInput) discordgo.MessageComponent {
		return discordgo.ActionsRow{Components: []discordgo.MessageComponent{in}}
	}
	return []discordgo.MessageComponent{
		row(discordgo.TextInput{
			CustomID: inputCim, Label: "Cím", Style: discordgo.TextInputShort,
			Required: true, MaxLength: app.MaxEsemenyTitleLength,
		}),
		row(discordgo.TextInput{
			CustomID: inputHelyszin, Label: "Helyszín (név)", Style: discordgo.TextInputShort,
			Required: false, MaxLength: app.MaxHelyszinNameLength,
		}),
		row(discordgo.TextInput{
			CustomID: inputTerkep, Label: "Térkép link", Style: discordgo.TextInputShort,
			Placeholder: "https://maps.app.goo.gl/…", Required: false, MaxLength: app.MaxHelyszinURLLength,
		}),
		row(discordgo.TextInput{
			CustomID: inputLeiras, Label: "Leírás", Style: discordgo.TextInputParagraph,
			Required: false, MaxLength: app.MaxEsemenyDescriptionLength,
		}),
		row(discordgo.TextInput{
			CustomID: inputIdopontok, Label: "Időpontok", Style: discordgo.TextInputParagraph,
			Placeholder: idopontokPlaceholder, Required: true, MaxLength: 500,
		}),
	}
}

// proposeFromModal reads the submitted modal and proposes the Esemeny.
func (r *Router) proposeFromModal(ctx context.Context, req Request) (Reply, error) {
	if len(req.Args) != 1 {
		return ephemeral(unknownCommand), nil
	}
	csoportID := req.Args[0]
	values := modalValues(req.Interaction.ModalSubmitData())

	title := strings.TrimSpace(values[inputCim])
	if title == "" {
		return ephemeral(esemenyNeedsTitle), nil
	}
	idopontok, err := r.app.ParseIdopontok(values[inputIdopontok])
	var parseErr *app.IdopontParseError
	if errors.As(err, &parseErr) {
		return ephemeral(parseErr.Error()), nil
	}
	if err != nil {
		return Reply{}, err
	}

	e, err := r.app.ProposeEsemeny(ctx, req.Felhasznalo.ID, csoportID, app.EsemenyInput{
		Title:       title,
		Description: values[inputLeiras],
		Helyszin:    domain.Helyszin{Name: values[inputHelyszin], URL: values[inputTerkep]},
		Idopontok:   idopontok,
	})
	switch {
	case errors.Is(err, app.ErrNotFound):
		return ephemeral(esemenyNotTag), nil
	case errors.Is(err, app.ErrNonexistentLocalTime):
		return ephemeral(esemenyNoSuchTime), nil
	case errors.Is(err, app.ErrInvalidInput):
		return ephemeral(fmt.Sprintf(esemenyBadInput, app.MaxEsemenyTitleLength, app.MaxHelyszinNameLength, app.MaxEsemenyDescriptionLength)), nil
	case err != nil:
		return Reply{}, err
	}

	msg := fmt.Sprintf(esemenyCreated, escapeMarkdown(e.Title))
	if u := r.app.EsemenyURL(e.ID); app.DiscordLinkable(u) {
		msg += fmt.Sprintf(esemenyCreatedLink, u)
	} else {
		msg += fmt.Sprintf(esemenyCreatedNoLink, u)
	}
	return ephemeral(msg), nil
}

// modalValues returns what was typed into each text input, by custom id.
func modalValues(d discordgo.ModalSubmitInteractionData) map[string]string {
	values := map[string]string{}
	for _, c := range d.Components {
		var row []discordgo.MessageComponent
		switch r := c.(type) {
		case *discordgo.ActionsRow:
			row = r.Components
		case discordgo.ActionsRow:
			row = r.Components
		}
		for _, c := range row {
			switch in := c.(type) {
			case *discordgo.TextInput:
				values[in.CustomID] = in.Value
			case discordgo.TextInput:
				values[in.CustomID] = in.Value
			}
		}
	}
	return values
}
