// Package bot is the Discord entry point: a thin adapter that turns
// interactions (slash commands, autocomplete, buttons, modals) into calls on
// the application core and formats the replies. Rules belong in package app,
// not here.
//
// The Router is the testable part. It takes a constructed
// *discordgo.Interaction and returns the Routed reply to send, without
// touching the network. Start connects it to the gateway.
//
// # Adding a handler
//
// Handlers live in their own file, one per feature, and are registered in
// registerAll (handlers.go):
//
//   - A slash command: r.Command(Command{Definition: ..., Run: ..., Autocomplete: ...}).
//     Definition is what gets registered with Discord; its Name is the route.
//   - A button or select menu: r.Component("name", handler). Build its
//     custom id with CustomID("name", "arg1", "arg2"), which gives
//     "name:arg1:arg2"; the handler gets Request.Args = ["arg1", "arg2"].
//   - A modal: r.Modal("name", handler), with the same custom-id scheme.
//
// Every interaction is first resolved to a Felhasznalo (ADR-0002). Handlers
// only ever see known Felhasznalok: an unknown Discord account gets an
// ephemeral login link from the Router, and no handler runs.
//
// # Discord's 3-second limit
//
// Discord drops an interaction that isn't answered within 3 seconds, so a
// handler's context expires shortly before that. A handler whose work may
// take longer (calling Spotify or Google, sending many DMs) must defer: it
// returns deferMessage or deferUpdate with the slow work as a Followup.
// Discord then shows "thinking…" (or nothing, for a component update), and
// the Followup's result replaces the original reply once it finishes; it has
// up to 15 minutes.
//
// A handler that returns an error or panics is logged, and the user gets a
// generic ephemeral error. A panic never stops the process.
package bot

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"runtime/debug"
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

const (
	// customIDSeparator separates a component's or modal's route name from
	// its arguments in its custom id.
	customIDSeparator = ":"
	// maxCustomIDLength is the longest custom id Discord accepts.
	maxCustomIDLength = 100
)

// Request is one interaction from a known Felhasznalo.
type Request struct {
	Interaction *discordgo.Interaction
	// Felhasznalo is who acted, resolved from their Discord account.
	Felhasznalo domain.Felhasznalo
	// Args are the custom-id arguments of a component or modal, after its
	// route name. Empty for commands.
	Args []string
}

// Followup is the slow part of a deferred reply. Its result replaces the
// original (deferred) reply.
type Followup func(ctx context.Context) (*discordgo.WebhookEdit, error)

// Reply is what a handler answers with.
type Reply struct {
	// Response is sent at once.
	Response *discordgo.InteractionResponse
	// Then, if set, runs after Response was sent, which must then be a
	// deferred response. Build both with deferMessage or deferUpdate.
	Then Followup
}

// Handler answers one interaction within Discord's 3-second limit (see the
// package comment). A returned error is logged and the user gets a generic
// ephemeral error, so handlers should turn expected refusals into replies
// themselves.
type Handler func(ctx context.Context, req Request) (Reply, error)

// Command is one slash command.
type Command struct {
	// Definition is registered with Discord. Its Name is the route.
	Definition *discordgo.ApplicationCommand
	// Run answers the command itself.
	Run Handler
	// Autocomplete answers autocomplete requests for its options. Optional.
	Autocomplete Handler
}

// Routed is the Router's answer to one interaction.
type Routed struct {
	// Response is sent at once. It is never nil.
	Response *discordgo.InteractionResponse
	// Complete, if set, finishes a deferred reply: send its result with
	// InteractionResponseEdit. It never fails and never panics.
	Complete func(ctx context.Context) *discordgo.WebhookEdit
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

// Command registers a slash command. It panics on a duplicate name.
func (r *Router) Command(c Command) {
	r.mustBeNew(c.Definition.Name, r.commands[c.Definition.Name].Run != nil)
	r.commands[c.Definition.Name] = c
}

// Component registers the handler of buttons and select menus whose custom
// id is name, or name followed by ":" and arguments. It panics on a
// duplicate or invalid name.
func (r *Router) Component(name string, h Handler) {
	r.mustBeNew(name, r.components[name] != nil)
	r.components[name] = h
}

// Modal registers the handler of modals whose custom id is name, or name
// followed by ":" and arguments. It panics on a duplicate or invalid name.
func (r *Router) Modal(name string, h Handler) {
	r.mustBeNew(name, r.modals[name] != nil)
	r.modals[name] = h
}

func (r *Router) mustBeNew(name string, taken bool) {
	if name == "" || strings.Contains(name, customIDSeparator) || len(name) > maxCustomIDLength || taken {
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
// the given arguments. It fails if an argument contains ":" or the id would
// be longer than Discord's 100 characters.
func CustomID(name string, args ...string) (string, error) {
	for _, a := range args {
		if strings.Contains(a, customIDSeparator) {
			return "", fmt.Errorf("bot: custom id argument %q contains %q", a, customIDSeparator)
		}
	}
	id := strings.Join(append([]string{name}, args...), customIDSeparator)
	if len(id) > maxCustomIDLength {
		return "", fmt.Errorf("bot: custom id %q is longer than %d characters", id, maxCustomIDLength)
	}
	return id, nil
}

// Route answers one interaction. ctx bounds the handler's work; it should
// expire before Discord's 3-second limit.
func (r *Router) Route(ctx context.Context, i *discordgo.Interaction) (routed Routed) {
	defer func() {
		if p := recover(); p != nil {
			r.log.Error("interaction panicked", "type", i.Type.String(), "panic", p, "stack", string(debug.Stack()))
			routed = Routed{Response: errorResponse(i, internalError)}
		}
	}()

	discordID := userID(i)
	f, err := r.app.FelhasznaloByDiscordID(ctx, discordID)
	if errors.Is(err, app.ErrNotFound) {
		// Nothing is recorded for an unknown account (ADR-0002).
		return Routed{Response: errorResponse(i, loginPrompt+" "+r.loginURL)}
	}
	if err != nil {
		r.log.Error("resolve discord account", "discord_id", discordID, "err", err)
		return Routed{Response: errorResponse(i, internalError)}
	}

	h, args := r.handler(i)
	if h == nil {
		r.log.Warn("no handler for interaction", "type", i.Type.String(), "route", routeName(i))
		return Routed{Response: errorResponse(i, unknownCommand)}
	}
	reply, err := h(ctx, Request{Interaction: i, Felhasznalo: f, Args: args})
	if err == nil && reply.Response == nil {
		err = errors.New("handler returned no response")
	}
	if err != nil {
		r.log.Error("interaction failed", "type", i.Type.String(), "route", routeName(i), "felhasznalo", f.ID, "err", err)
		return Routed{Response: errorResponse(i, internalError)}
	}

	routed = Routed{Response: reply.Response}
	if reply.Then != nil {
		routed.Complete = r.safeFollowup(i, f, reply.Then)
	}
	return routed
}

// safeFollowup wraps a Followup so that an error or a panic becomes the
// generic error message instead.
func (r *Router) safeFollowup(i *discordgo.Interaction, f domain.Felhasznalo, then Followup) func(context.Context) *discordgo.WebhookEdit {
	failed := func() *discordgo.WebhookEdit {
		msg := internalError
		return &discordgo.WebhookEdit{Content: &msg}
	}
	return func(ctx context.Context) (edit *discordgo.WebhookEdit) {
		defer func() {
			if p := recover(); p != nil {
				r.log.Error("deferred interaction panicked", "route", routeName(i), "felhasznalo", f.ID, "panic", p, "stack", string(debug.Stack()))
				edit = failed()
			}
		}()
		edit, err := then(ctx)
		if err == nil && edit == nil {
			err = errors.New("followup returned no edit")
		}
		if err != nil {
			r.log.Error("deferred interaction failed", "route", routeName(i), "felhasznalo", f.ID, "err", err)
			return failed()
		}
		return edit
	}
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

// errorResponse answers with an ephemeral message, or with no choices for
// autocomplete, which can't show a message.
func errorResponse(i *discordgo.Interaction, msg string) *discordgo.InteractionResponse {
	if i.Type == discordgo.InteractionApplicationCommandAutocomplete {
		return autocomplete(nil).Response
	}
	return ephemeral(msg).Response
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

// ephemeral is a reply only the user who acted can see.
func ephemeral(content string) Reply {
	return Reply{Response: &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Content: content,
			Flags:   discordgo.MessageFlagsEphemeral,
		},
	}}
}

// public is a reply everyone in the channel can see.
func public(content string) Reply {
	return Reply{Response: &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Content: content,
			// Never ping anyone by accident from user-provided text.
			AllowedMentions: &discordgo.MessageAllowedMentions{},
		},
	}}
}

// autocomplete answers an autocomplete request. Discord shows at most 25
// choices.
func autocomplete(choices []*discordgo.ApplicationCommandOptionChoice) Reply {
	if choices == nil {
		choices = []*discordgo.ApplicationCommandOptionChoice{}
	}
	return Reply{Response: &discordgo.InteractionResponse{
		Type: discordgo.InteractionApplicationCommandAutocompleteResult,
		Data: &discordgo.InteractionResponseData{Choices: choices},
	}}
}

// deferMessage acknowledges a command or modal at once ("thinking…") and
// runs then afterwards; its result becomes the reply. With isEphemeral, only
// the user who acted sees it.
func deferMessage(isEphemeral bool, then Followup) Reply {
	data := &discordgo.InteractionResponseData{}
	if isEphemeral {
		data.Flags = discordgo.MessageFlagsEphemeral
	}
	return Reply{
		Response: &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseDeferredChannelMessageWithSource,
			Data: data,
		},
		Then: then,
	}
}

// deferUpdate acknowledges a button or select menu at once without changing
// anything, and runs then afterwards; its result edits the message the
// component is on.
func deferUpdate(then Followup) Reply {
	return Reply{
		Response: &discordgo.InteractionResponse{Type: discordgo.InteractionResponseDeferredMessageUpdate},
		Then:     then,
	}
}
