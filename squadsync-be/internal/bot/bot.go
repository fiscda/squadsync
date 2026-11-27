package bot

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/bwmarrin/discordgo"

	"squadsync-be/internal/gateway"
)

const (
	// handlerTimeout bounds a handler's work, leaving time to send the reply
	// within Discord's 3-second limit.
	handlerTimeout = 2500 * time.Millisecond
	// respondTimeout bounds sending the reply itself.
	respondTimeout = 5 * time.Second
	// followupTimeout bounds the slow part of a deferred reply. Discord
	// keeps the interaction token valid for 15 minutes.
	followupTimeout = 14 * time.Minute

	// Retry delays when the gateway can't be reached at startup.
	firstRetryDelay = 5 * time.Second
	maxRetryDelay   = 5 * time.Minute
)

// NewSession prepares a gateway session for the bot token without
// connecting. It asks only for the Guilds intent, which isn't privileged and
// keeps channel names in the session's cache.
func NewSession(token string) (*discordgo.Session, error) {
	s, err := discordgo.New("Bot " + token)
	if err != nil {
		return nil, fmt.Errorf("create discord session: %w", err)
	}
	s.Identify.Intents = discordgo.IntentsGuilds
	return s, nil
}

// ChannelNameFromCache returns a ChannelName lookup that reads the session's
// cache only, never the API.
func ChannelNameFromCache(s *discordgo.Session) func(channelID string) string {
	return func(channelID string) string {
		if c, err := s.State.Channel(channelID); err == nil {
			return c.Name
		}
		return ""
	}
}

// Start connects the session to Discord's gateway in the background and
// serves interactions with the router. If Discord can't be reached, it logs
// that and keeps retrying with backoff, so the rest of Go keeps working.
// Once connected, discordgo reconnects by itself.
//
// Slash commands are registered on the guild devGuildID if it is set (they
// update at once), and globally otherwise.
//
// The returned function stops retrying and closes the connection.
func Start(ctx context.Context, s *discordgo.Session, r *Router, devGuildID string, log *slog.Logger) (stop func()) {
	ctx, cancel := context.WithCancel(ctx)
	reg := &commandRegistrar{router: r, guildID: devGuildID, log: log}
	s.AddHandler(func(s *discordgo.Session, ready *discordgo.Ready) {
		log.Info("discord bot online", "user", ready.User.Username)
		reg.register(ctx, s, ready)
	})
	s.AddHandler(func(s *discordgo.Session, ic *discordgo.InteractionCreate) {
		serve(ctx, s, r, ic.Interaction, log)
	})

	var connected bool
	done := make(chan struct{})
	go func() {
		defer close(done)
		delay := firstRetryDelay
		for {
			err := s.Open()
			if err == nil {
				connected = true
				return
			}
			log.Error("connect discord bot; retrying", "err", err, "retry_in", delay)
			select {
			case <-ctx.Done():
				return
			case <-time.After(delay):
			}
			delay = min(2*delay, maxRetryDelay)
		}
	}()

	return func() {
		cancel()
		<-done
		if connected {
			if err := s.Close(); err != nil {
				log.Error("close discord bot", "err", err)
			}
		}
	}
}

// serve answers one interaction: the immediate reply first, then the slow
// part of a deferred one.
func serve(ctx context.Context, s *discordgo.Session, r *Router, i *discordgo.Interaction, log *slog.Logger) {
	hctx, cancel := context.WithTimeout(ctx, handlerTimeout)
	routed := r.Route(hctx, i)
	cancel()

	// A fresh context: the handler may have used up its own.
	rctx, cancel := context.WithTimeout(ctx, respondTimeout)
	err := s.InteractionRespond(i, routed.Response, discordgo.WithContext(rctx))
	cancel()
	if err != nil {
		log.Error("respond to discord interaction", "err", err)
		return
	}
	if routed.Complete == nil {
		return
	}

	fctx, cancel := context.WithTimeout(ctx, followupTimeout)
	defer cancel()
	edit := routed.Complete(fctx)
	if _, err := s.InteractionResponseEdit(i, edit, discordgo.WithContext(fctx)); err != nil {
		log.Error("complete deferred discord interaction", "err", err)
	}
}

// commandRegistrar registers the slash commands once per process, trying
// again on the next Ready if it failed.
type commandRegistrar struct {
	router  *Router
	guildID string
	log     *slog.Logger

	mu         sync.Mutex
	registered bool
}

func (c *commandRegistrar) register(ctx context.Context, s *discordgo.Session, ready *discordgo.Ready) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.registered {
		return
	}
	appID := ready.User.ID // a bot's user id is its application id
	if ready.Application != nil {
		appID = ready.Application.ID
	}
	cmds, err := s.ApplicationCommandBulkOverwrite(appID, c.guildID, c.router.Commands(), discordgo.WithContext(ctx))
	if err != nil {
		c.log.Error("register discord commands; will retry on the next connect", "err", err)
		return
	}
	c.registered = true
	scope := "global"
	if c.guildID != "" {
		scope = "guild " + c.guildID
	}
	c.log.Info("discord commands registered", "count", len(cmds), "scope", scope)
}

// Messenger sends messages through the bot. It is the real
// gateway.DiscordMessenger.
type Messenger struct {
	Session *discordgo.Session
}

var _ gateway.DiscordMessenger = Messenger{}

// SendDM implements gateway.DiscordMessenger.
func (m Messenger) SendDM(ctx context.Context, discordID string, content string) error {
	ch, err := m.Session.UserChannelCreate(discordID, discordgo.WithContext(ctx))
	if err != nil {
		return fmt.Errorf("open dm channel: %w", err)
	}
	if _, err := m.Session.ChannelMessageSendComplex(ch.ID, &discordgo.MessageSend{
		Content:         content,
		AllowedMentions: &discordgo.MessageAllowedMentions{},
	}, discordgo.WithContext(ctx)); err != nil {
		return fmt.Errorf("send dm: %w", err)
	}
	return nil
}
