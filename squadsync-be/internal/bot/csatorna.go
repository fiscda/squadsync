package bot

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/bwmarrin/discordgo"

	"squadsync-be/internal/app"
)

// The /csatorna command links the channel it runs in to a Csoport.
const (
	csatornaCommandName = "csatorna"
	csatornaOptionName  = "csoport"
	// maxChoices is the most choices Discord accepts in an autocomplete reply.
	maxChoices = 25
)

// Replies of /csatorna.
const (
	csatornaNotInGuild = "Ezt egy szerver csatornájában futtasd: azt a csatornát kapcsolom a csoporthoz."
	csatornaNotAdmin   = "Csak a csoport adminja kapcsolhat csatornát a csoporthoz."
	csatornaNotFound   = "Nem találom ezt a csoportot. Válassz a listából egy csoportot, ahol admin vagy."
	csatornaTaken      = "Ez a csatorna már egy másik csoporthoz tartozik. Előbb annak egy adminja válassza le a csoport weboldalán."
	csatornaLinked     = "Kész! A(z) **%s** csoport bejelentései mostantól ebbe a csatornába érkeznek."
)

func (r *Router) csatornaCommand() Command {
	guildOnly := []discordgo.InteractionContextType{discordgo.InteractionContextGuild}
	return Command{
		Definition: &discordgo.ApplicationCommand{
			Name:        csatornaCommandName,
			Description: "Ezt a csatornát kapcsolja a csoportodhoz (csak adminoknak).",
			Contexts:    &guildOnly,
			Options: []*discordgo.ApplicationCommandOption{{
				Type:         discordgo.ApplicationCommandOptionString,
				Name:         csatornaOptionName,
				Description:  "A csoport, amelyhez a csatornát kapcsolod",
				Required:     true,
				Autocomplete: true,
			}},
		},
		Run:          r.linkCsatorna,
		Autocomplete: r.csatornaCsoportChoices,
	}
}

func (r *Router) linkCsatorna(ctx context.Context, req Request) (Reply, error) {
	i := req.Interaction
	if i.GuildID == "" {
		return ephemeral(csatornaNotInGuild), nil
	}
	csoportID := optionValue(i.ApplicationCommandData().Options, csatornaOptionName)

	linked, err := r.app.LinkCsatorna(ctx, req.Felhasznalo.ID, app.LinkCsatornaInput{
		CsoportID:   csoportID,
		GuildID:     i.GuildID,
		ChannelID:   i.ChannelID,
		ChannelName: r.channelName(i.ChannelID),
	})
	switch {
	case errors.Is(err, app.ErrNotAdmin):
		return ephemeral(csatornaNotAdmin), nil
	case errors.Is(err, app.ErrNotFound):
		return ephemeral(csatornaNotFound), nil
	case errors.Is(err, app.ErrCsatornaTaken):
		return ephemeral(csatornaTaken), nil
	case err != nil:
		return Reply{}, err
	}
	return public(fmt.Sprintf(csatornaLinked, linked.Csoport.Name)), nil
}

// csatornaCsoportChoices offers the Csoports where the caller is Admin,
// filtered by what they've typed so far.
func (r *Router) csatornaCsoportChoices(ctx context.Context, req Request) (Reply, error) {
	typed := strings.ToLower(strings.TrimSpace(optionValue(req.Interaction.ApplicationCommandData().Options, csatornaOptionName)))
	csoportok, err := r.app.AdminCsoportokOf(ctx, req.Felhasznalo.ID)
	if err != nil {
		return Reply{}, err
	}
	var choices []*discordgo.ApplicationCommandOptionChoice
	for _, c := range csoportok {
		if len(choices) == maxChoices {
			break
		}
		if strings.Contains(strings.ToLower(c.Name), typed) {
			choices = append(choices, &discordgo.ApplicationCommandOptionChoice{Name: c.Name, Value: c.ID})
		}
	}
	return autocomplete(choices), nil
}

// optionValue returns the string value of the named option, or "".
func optionValue(options []*discordgo.ApplicationCommandInteractionDataOption, name string) string {
	for _, o := range options {
		if o.Name == name && o.Type == discordgo.ApplicationCommandOptionString {
			return o.StringValue()
		}
	}
	return ""
}
