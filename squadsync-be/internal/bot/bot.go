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

// interactionTimeout bounds the work for one interaction. Discord drops a
// reply that comes later than 3 seconds anyway.
const interactionTimeout = 3 * time.Second

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

// Start connects the session to Discord's gateway and serves interactions
// with the router until ctx ends. Slash commands are registered on the guild
// devGuildID if it is set (they update at once), and globally otherwise.
// The returned function closes the connection.
func Start(ctx context.Context, s *discordgo.Session, r *Router, devGuildID string, log *slog.Logger) (func(), error) {
	var registerOnce sync.Once
	s.AddHandler(func(s *discordgo.Session, ready *discordgo.Ready) {
		log.Info("discord bot online", "user", ready.User.Username)
		// Ready comes again after a full reconnect; the commands don't change.
		registerOnce.Do(func() {
			appID := ready.User.ID // a bot's user id is its application id
			if ready.Application != nil {
				appID = ready.Application.ID
			}
			cmds, err := s.ApplicationCommandBulkOverwrite(appID, devGuildID, r.Commands(), discordgo.WithContext(ctx))
			if err != nil {
				log.Error("register discord commands", "err", err)
				return
			}
			scope := "global"
			if devGuildID != "" {
				scope = "guild " + devGuildID
			}
			log.Info("discord commands registered", "count", len(cmds), "scope", scope)
		})
	})
	s.AddHandler(func(s *discordgo.Session, ic *discordgo.InteractionCreate) {
		ictx, cancel := context.WithTimeout(ctx, interactionTimeout)
		defer cancel()
		resp := r.Route(ictx, ic.Interaction)
		if err := s.InteractionRespond(ic.Interaction, resp, discordgo.WithContext(ictx)); err != nil {
			log.Error("respond to discord interaction", "err", err)
		}
	})

	if err := s.Open(); err != nil {
		return nil, fmt.Errorf("connect discord bot: %w", err)
	}
	return func() {
		if err := s.Close(); err != nil {
			log.Error("close discord bot", "err", err)
		}
	}, nil
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
