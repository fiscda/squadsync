// Command squadsync runs the Go backend: the internal HTTP API on localhost,
// the Discord bot and the once-a-minute loop of timed jobs.
//
// Usage:
//
//	squadsync           migrate the database, then serve
//	squadsync migrate   migrate the database and exit
//
// Configuration comes from environment variables (see ../../../.env.example):
//
//	DATABASE_PATH        SQLite file shared with Next.js (required)
//	INTERNAL_API_SECRET  shared secret Next.js sends on every call (required)
//	BACKEND_PORT         port on 127.0.0.1 to listen on (default 8081)
//	DISCORD_BOT_TOKEN    the bot's token; without it Go runs without the bot
//	DISCORD_DEV_GUILD_ID register slash commands on this server only (optional)
//	BETTER_AUTH_URL      public URL of the web app, for login links, links in
//	                     Discord messages and the linking redirect URI (required
//	                     with the bot)
//	DISCORD_CLIENT_ID    the Discord application's OAuth2 client id, for linking
//	DISCORD_CLIENT_SECRET  and its secret; without both, linking is unavailable
//	GOOGLE_CLIENT_ID     the Google OAuth client id, for Google Calendar sync
//	GOOGLE_CLIENT_SECRET   and its secret; without both (or BETTER_AUTH_URL),
//	                     connecting Google Calendar is unavailable
package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/bwmarrin/discordgo"

	"squadsync-be/internal/app"
	"squadsync-be/internal/bot"
	"squadsync-be/internal/clock"
	"squadsync-be/internal/gateway"
	"squadsync-be/internal/httpapi"
	"squadsync-be/internal/store"
)

const minSecretLength = 32

// discordShutdownTimeout bounds how long shutdown waits for queued Discord
// messages.
const discordShutdownTimeout = 10 * time.Second

// jobInterval is how often due timed jobs run; jobTickTimeout bounds one
// run of them.
const (
	jobInterval    = time.Minute
	jobTickTimeout = 50 * time.Second
)

type config struct {
	dbPath string
	secret string
	port   string

	botToken   string
	devGuildID string
	publicURL  string

	discordClientID     string
	discordClientSecret string

	googleClientID     string
	googleClientSecret string
}

func main() {
	log := slog.New(slog.NewTextHandler(os.Stderr, nil))
	if err := run(log, os.Args[1:]); err != nil {
		log.Error("squadsync failed", "err", err)
		os.Exit(1)
	}
}

func run(log *slog.Logger, args []string) error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	migrateOnly := len(args) > 0 && args[0] == "migrate"
	if len(args) > 0 && !migrateOnly {
		return fmt.Errorf("unknown command %q (want no command, or \"migrate\")", args[0])
	}

	cfg, err := loadConfig(migrateOnly)
	if err != nil {
		return err
	}

	if err := os.MkdirAll(filepath.Dir(cfg.dbPath), 0o755); err != nil {
		return fmt.Errorf("create database directory: %w", err)
	}
	s, err := store.Open(cfg.dbPath)
	if err != nil {
		return err
	}
	defer s.Close()
	if err := s.Migrate(ctx); err != nil {
		return err
	}
	log.Info("database migrated", "path", cfg.dbPath)
	if migrateOnly {
		return nil
	}

	var discord gateway.DiscordMessenger = gateway.Disabled{}
	var session *discordgo.Session
	if cfg.botToken != "" {
		if session, err = bot.NewSession(cfg.botToken); err != nil {
			return err
		}
		discord = bot.Messenger{Session: session}
	}

	var discordOAuth gateway.DiscordOAuth = gateway.Disabled{}
	if cfg.discordClientID != "" && cfg.discordClientSecret != "" && cfg.publicURL != "" {
		discordOAuth = &gateway.DiscordOAuthClient{
			ClientID:     cfg.discordClientID,
			ClientSecret: cfg.discordClientSecret,
		}
	} else {
		log.Info("DISCORD_CLIENT_ID, DISCORD_CLIENT_SECRET or BETTER_AUTH_URL is not set: linking Discord accounts is unavailable")
	}

	var calendar gateway.GoogleCalendar = gateway.Disabled{}
	if cfg.googleClientID != "" && cfg.googleClientSecret != "" && cfg.publicURL != "" {
		calendar = &gateway.GoogleCalendarClient{
			ClientID:     cfg.googleClientID,
			ClientSecret: cfg.googleClientSecret,
		}
	} else {
		log.Info("GOOGLE_CLIENT_ID, GOOGLE_CLIENT_SECRET or BETTER_AUTH_URL is not set: Google Calendar sync is unavailable")
	}

	a := app.New(app.Deps{
		Store:    s,
		Clock:    clock.System{},
		Discord:  discord,
		Calendar: calendar,
		Spotify:  gateway.Disabled{},

		DiscordOAuth: discordOAuth,
		WebURL:       cfg.publicURL,
		Log:          log,
		AsyncDiscord: true,
	})

	if session == nil {
		log.Info("DISCORD_BOT_TOKEN is not set: running without the Discord bot")
	} else {
		router := bot.NewRouter(bot.Deps{
			App:         a,
			LoginURL:    cfg.publicURL,
			Log:         log,
			ChannelName: bot.ChannelNameFromCache(session),
		})
		// Connects in the background: a Discord outage never stops the API.
		stopBot := bot.Start(ctx, session, router, cfg.devGuildID, log)
		defer stopBot()
	}

	// Localhost only: Next.js is the sole public surface (ADR-0004).
	addr := net.JoinHostPort("127.0.0.1", cfg.port)
	srv := &http.Server{
		Addr:              addr,
		Handler:           httpapi.New(a, cfg.secret, log),
		ReadHeaderTimeout: 5 * time.Second,
	}

	errc := make(chan error, 1)
	go func() {
		log.Info("internal API listening", "addr", addr)
		errc <- srv.ListenAndServe()
	}()

	jobsStopped := make(chan struct{})
	go func() {
		defer close(jobsStopped)
		runJobs(ctx, a, log)
	}()

	select {
	case err := <-errc:
		return err
	case <-ctx.Done():
	}

	log.Info("shutting down")
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	// Let the job run in progress finish: its Discord work is queued
	// before WaitDiscord stops taking more.
	<-jobsStopped
	// Let queued Discord messages go out before the bot disconnects, for at
	// most discordShutdownTimeout; the rest are abandoned.
	waitCtx, cancelWait := context.WithTimeout(context.Background(), discordShutdownTimeout)
	defer cancelWait()
	a.WaitDiscord(waitCtx)
	return nil
}

// runJobs runs the due timed jobs at once (catching up on any missed while
// the server was down) and then every jobInterval, until ctx ends. Once ctx
// ends, a run stops between jobs: the job in progress finishes, the rest
// wait for the next start.
func runJobs(ctx context.Context, a *app.App, log *slog.Logger) {
	tick := func() {
		tctx, cancel := context.WithTimeout(ctx, jobTickTimeout)
		defer cancel()
		err := a.RunDueJobs(tctx)
		switch {
		case errors.Is(err, context.Canceled) && ctx.Err() != nil:
			log.Info("timed jobs stopped for shutdown")
		case err != nil:
			log.Error("run timed jobs", "err", err)
		}
	}
	ticker := time.NewTicker(jobInterval)
	defer ticker.Stop()
	for {
		tick()
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func loadConfig(migrateOnly bool) (config, error) {
	cfg := config{
		dbPath: os.Getenv("DATABASE_PATH"),
		secret: os.Getenv("INTERNAL_API_SECRET"),
		port:   os.Getenv("BACKEND_PORT"),

		botToken:   strings.TrimSpace(os.Getenv("DISCORD_BOT_TOKEN")),
		devGuildID: strings.TrimSpace(os.Getenv("DISCORD_DEV_GUILD_ID")),
		publicURL:  strings.TrimRight(strings.TrimSpace(os.Getenv("BETTER_AUTH_URL")), "/"),

		discordClientID:     strings.TrimSpace(os.Getenv("DISCORD_CLIENT_ID")),
		discordClientSecret: strings.TrimSpace(os.Getenv("DISCORD_CLIENT_SECRET")),

		googleClientID:     strings.TrimSpace(os.Getenv("GOOGLE_CLIENT_ID")),
		googleClientSecret: strings.TrimSpace(os.Getenv("GOOGLE_CLIENT_SECRET")),
	}
	if cfg.dbPath == "" {
		return config{}, errors.New("DATABASE_PATH is required")
	}
	if cfg.port == "" {
		cfg.port = "8081"
	}
	if !migrateOnly && len(cfg.secret) < minSecretLength {
		return config{}, fmt.Errorf("INTERNAL_API_SECRET must be at least %d characters", minSecretLength)
	}
	if !migrateOnly && cfg.botToken != "" && cfg.publicURL == "" {
		return config{}, errors.New("BETTER_AUTH_URL is required with DISCORD_BOT_TOKEN: the bot sends it as the login link")
	}
	return cfg, nil
}
