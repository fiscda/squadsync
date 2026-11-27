package app

import (
	"context"
	"errors"

	"squadsync-be/internal/gateway"
)

// Every DM the app sends goes through this file, to every Discord account
// linked to the Felhasznalo (ADR-0002). Sending is best effort: a failure is
// logged and never fails the use case that caused it.

// dmAccounts returns the Discord ids to DM the Felhasznalo on about the
// Csoport: every linked account. This is the one place to skip a Tag who
// muted the Csoport. csoportID is unused until #11 adds DM muting per Tag;
// every caller already passes it so that #11 only changes this function.
func (a *App) dmAccounts(ctx context.Context, csoportID, felhasznaloID string) []string {
	accounts, err := a.store.ListDiscordAccounts(ctx, felhasznaloID)
	if err != nil {
		a.log.Error("list discord accounts for a dm", "felhasznalo", felhasznaloID, "csoport", csoportID, "err", err)
		return nil
	}
	ids := make([]string, len(accounts))
	for n, acc := range accounts {
		ids[n] = acc.DiscordID
	}
	return ids
}

// sendVoteRequestDM asks the Felhasznalo, on every Discord account, to
// answer a new Esemeny of the Csoport.
func (a *App) sendVoteRequestDM(ctx context.Context, csoportID, felhasznaloID string, r gateway.VoteRequest) {
	for _, discordID := range a.dmAccounts(ctx, csoportID, felhasznaloID) {
		cctx, cancel := context.WithTimeout(ctx, discordCallTimeout)
		err := a.discord.SendVoteRequestDM(cctx, discordID, r)
		cancel()
		a.logDiscordError("send vote request dm", err, "felhasznalo", felhasznaloID, "discord_id", discordID)
	}
}

// sendEsemenyDM sends an Esemeny's message of the Csoport, with the vote
// request above it, to every Discord account of the Felhasznalo. It returns
// the messages that were sent.
func (a *App) sendEsemenyDM(ctx context.Context, csoportID, felhasznaloID string, r gateway.VoteRequest, m gateway.EsemenyMessage) []gateway.MessageRef {
	var sent []gateway.MessageRef
	for _, discordID := range a.dmAccounts(ctx, csoportID, felhasznaloID) {
		cctx, cancel := context.WithTimeout(ctx, discordCallTimeout)
		ref, err := a.discord.SendEsemenyDM(cctx, discordID, r, m)
		cancel()
		if err != nil {
			a.logDiscordError("send esemeny dm", err, "felhasznalo", felhasznaloID, "discord_id", discordID, "esemeny", m.EsemenyID)
			continue
		}
		sent = append(sent, ref)
	}
	return sent
}

// logDiscordError logs a failed Discord call, if err is set. Without a bot
// token every call fails with ErrNotConfigured, which isn't worth an error.
func (a *App) logDiscordError(what string, err error, args ...any) {
	if err == nil {
		return
	}
	args = append(args, "err", err)
	if errors.Is(err, gateway.ErrNotConfigured) {
		a.log.Debug(what+": discord is not configured", args...)
		return
	}
	a.log.Error(what, args...)
}
