// Package bot is the Discord entry point: a thin adapter that turns
// interactions (slash commands, autocomplete, buttons, modals) into calls on
// the application core and formats the replies. Rules belong in package app,
// not here.
//
// The Router is the testable part. It takes a constructed
// *discordgo.Interaction and returns the *discordgo.InteractionResponse to
// send, without touching the network. Run connects it to the gateway.
//
// # Adding a handler
//
// Handlers live in their own file, one per feature, and are registered in
// registerAll (handlers.go):
//
//   - A slash command: r.Command(Command{Definition: ..., Run: ..., Autocomplete: ...}).
//     Definition is what gets registered with Discord; its Name is the route.
//   - A button or select menu: r.Component("name", handler). Give the
//     component the custom id "name:arg1:arg2"; the handler gets
//     Request.Args = ["arg1", "arg2"]. Build ids with CustomID.
//   - A modal: r.Modal("name", handler), with the same custom-id scheme.
//
// Every interaction is first resolved to a Felhasznalo (ADR-0002). Handlers
// only ever see known Felhasznalok: an unknown Discord account gets an
// ephemeral login link from the Router, and no handler runs.
package bot

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sort"
	"strings"

	"github.com/bwmarrin/discordgo"

	"squadsync-be/internal/app"
	"squadsync-be/internal/domain"
)

// Replies the Router itself gives.
const (
	loginPrompt    = "Ehhez be kell lépned:"
	unknownCommand = "Ezt nem ismerem. Lehet, hogy a bot frissült; próbáld újra."
	internalError  = "Valami hiba történt. Próbáld újra később."
)

// customIDSeparator separates a component's or modal's route name from its
// arguments in its custom id.
const customIDSeparator = ":"

// Request is one interaction from a known Felhasznalo.
type Request struct {
	Interaction *discordgo.Interaction
	// Felhasznalo is who acted, resolved from their Discord account.
	Felhasznalo domain.Felhasznalo
	// Args are the custom-id arguments of a component or modal, after its
	// route name. Empty for commands.
	Args []string
}

// Handler answers one interaction. A returned error is logged and the user
// gets a generic ephemeral error, so handlers should turn expected refusals
// into replies themselves.
type Handler func(ctx context.Context, req Request) (*discordgo.InteractionResponse, error)

// Command is one slash command.
type Command struct {
	// Definition is registered with Discord. Its Name is the route.
	Definition *discordgo.ApplicationCommand
	// Run answers the command itself.
	Run Handler
	// Autocomplete answers autocomplete requests for its options. Optional.
	Autocomplete Handler
}

// Deps is what the Router needs.
type Deps struct {
	App *app.App
	// LoginURL is where an unknown Discord account is sent to log in.
	LoginURL string
	Log      *slog.Logger
	// ChannelName returns a channel's name, or "" if it isn't known. It must
	// not call Discord's API (it runs before the reply). Optional.
	ChannelName func(channelID string) string
}

// Router routes interactions to handlers.
type Router struct {
	app         *app.App
	loginURL    string
	log         *slog.Logger
	channelName func(channelID string) string

	commands   map[string]Command
	components map[string]Handler
	modals     map[string]Handler
}

// NewRouter builds a Router with every handler of the bot registered.
func NewRouter(d Deps) *Router {
	r := &Router{
		app:         d.App,
		loginURL:    d.LoginURL,
		log:         d.Log,
		channelName: d.ChannelName,
		commands:    map[string]Command{},
		components:  map[string]Handler{},
		modals:      map[string]Handler{},
	}
	if r.log == nil {
		r.log = slog.New(slog.DiscardHandler)
	}
	if r.channelName == nil {
		r.channelName = func(string) string { return "" }
	}
	r.registerAll()
	return r
}

// Command registers a slash command.
func (r *Router) Command(c Command) {
	r.mustBeNew(c.Definition.Name, r.commands[c.Definition.Name].Run != nil)
	r.commands[c.Definition.Name] = c
}

// Component registers the handler of buttons and select menus whose custom
// id is name, or starts with name followed by ":".
func (r *Router) Component(name string, h Handler) {
	r.mustBeNew(name, r.components[name] != nil)
	r.components[name] = h
}

// Modal registers the handler of modals whose custom id is name, or starts
// with name followed by ":".
func (r *Router) Modal(name string, h Handler) {
	r.mustBeNew(name, r.modals[name] != nil)
	r.modals[name] = h
}

func (r *Router) mustBeNew(name string, taken bool) {
	if name == "" || strings.Contains(name, customIDSeparator) || taken {
		panic(fmt.Sprintf("bot: invalid or duplicate route %q", name))
	}
}

// Commands returns the definitions of every slash command, to register with
// Discord, sorted by name.
func (r *Router) Commands() []*discordgo.ApplicationCommand {
	defs := make([]*discordgo.ApplicationCommand, 0, len(r.commands))
	for _, c := range r.commands {
		defs = append(defs, c.Definition)
	}
	sort.Slice(defs, func(i, j int) bool { return defs[i].Name < defs[j].Name })
	return defs
}

// CustomID builds the custom id of a component or modal routed to name, with
// the given arguments. Discord allows at most 100 characters.
func CustomID(name string, args ...string) string {
	return strings.Join(append([]string{name}, args...), customIDSeparator)
}

// Route answers one interaction. It never returns nil.
func (r *Router) Route(ctx context.Context, i *discordgo.Interaction) *discordgo.InteractionResponse {
	discordID := userID(i)
	f, err := r.app.FelhasznaloByDiscordID(ctx, discordID)
	if errors.Is(err, app.ErrNotFound) {
		// Nothing is recorded for an unknown account (ADR-0002).
		if i.Type == discordgo.InteractionApplicationCommandAutocomplete {
			return AutocompleteChoices(nil)
		}
		return Ephemeral(loginPrompt + " " + r.loginURL)
	}
	if err != nil {
		r.log.Error("resolve discord account", "discord_id", discordID, "err", err)
		return r.errorReply(i)
	}

	h, args := r.handler(i)
	if h == nil {
		r.log.Warn("no handler for interaction", "type", i.Type.String(), "route", routeName(i))
		return r.errorReplyWith(i, unknownCommand)
	}
	resp, err := h(ctx, Request{Interaction: i, Felhasznalo: f, Args: args})
	if err != nil {
		r.log.Error("interaction failed", "type", i.Type.String(), "route", routeName(i), "felhasznalo", f.ID, "err", err)
		return r.errorReply(i)
	}
	return resp
}

// handler finds the handler for an interaction, with its custom-id
// arguments.
func (r *Router) handler(i *discordgo.Interaction) (Handler, []string) {
	switch i.Type {
	case discordgo.InteractionApplicationCommand:
		return r.commands[i.ApplicationCommandData().Name].Run, nil
	case discordgo.InteractionApplicationCommandAutocomplete:
		return r.commands[i.ApplicationCommandData().Name].Autocomplete, nil
	case discordgo.InteractionMessageComponent:
		name, args := splitCustomID(i.MessageComponentData().CustomID)
		return r.components[name], args
	case discordgo.InteractionModalSubmit:
		name, args := splitCustomID(i.ModalSubmitData().CustomID)
		return r.modals[name], args
	}
	return nil, nil
}

func (r *Router) errorReply(i *discordgo.Interaction) *discordgo.InteractionResponse {
	return r.errorReplyWith(i, internalError)
}

// errorReplyWith answers with an ephemeral message, or with no choices for
// autocomplete, which can't show a message.
func (r *Router) errorReplyWith(i *discordgo.Interaction, msg string) *discordgo.InteractionResponse {
	if i.Type == discordgo.InteractionApplicationCommandAutocomplete {
		return AutocompleteChoices(nil)
	}
	return Ephemeral(msg)
}

func splitCustomID(id string) (string, []string) {
	parts := strings.Split(id, customIDSeparator)
	return parts[0], parts[1:]
}

// routeName names the interaction's route, for logs.
func routeName(i *discordgo.Interaction) string {
	switch i.Type {
	case discordgo.InteractionApplicationCommand, discordgo.InteractionApplicationCommandAutocomplete:
		return i.ApplicationCommandData().Name
	case discordgo.InteractionMessageComponent:
		return i.MessageComponentData().CustomID
	case discordgo.InteractionModalSubmit:
		return i.ModalSubmitData().CustomID
	}
	return ""
}

// userID is the Discord id of whoever triggered the interaction: Member is
// set in a server, User in a DM.
func userID(i *discordgo.Interaction) string {
	if i.Member != nil && i.Member.User != nil {
		return i.Member.User.ID
	}
	if i.User != nil {
		return i.User.ID
	}
	return ""
}

// Ephemeral is a reply only the user who acted can see.
func Ephemeral(content string) *discordgo.InteractionResponse {
	return &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Content: content,
			Flags:   discordgo.MessageFlagsEphemeral,
		},
	}
}

// Public is a reply everyone in the channel can see.
func Public(content string) *discordgo.InteractionResponse {
	return &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Content: content,
			// Never ping anyone by accident from user-provided text.
			AllowedMentions: &discordgo.MessageAllowedMentions{},
		},
	}
}

// AutocompleteChoices answers an autocomplete request. Discord shows at most
// 25 choices.
func AutocompleteChoices(choices []*discordgo.ApplicationCommandOptionChoice) *discordgo.InteractionResponse {
	if choices == nil {
		choices = []*discordgo.ApplicationCommandOptionChoice{}
	}
	return &discordgo.InteractionResponse{
		Type: discordgo.InteractionApplicationCommandAutocompleteResult,
		Data: &discordgo.InteractionResponseData{Choices: choices},
	}
}
