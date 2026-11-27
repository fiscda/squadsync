package app

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"squadsync-be/internal/domain"
	"squadsync-be/internal/store"
)

// FirstLoginInput is what Next.js knows right after a Felhasznalo's first web
// login through Better Auth.
type FirstLoginInput struct {
	// FelhasznaloID is Better Auth's user.id.
	FelhasznaloID string
	// DiscordID is the Discord user id of the account they logged in with,
	// which becomes their main Discord account.
	DiscordID string
	// Name is the display name to greet them with.
	Name string
}

// FirstLogin creates the Felhasznalo after their first web login, with the
// same id as Better Auth's user, and records their main Discord account
// (ADR-0002).
//
// It is idempotent: if the Felhasznalo already exists, it is returned
// unchanged, so calling it again on a later login reuses it.
func (a *App) FirstLogin(ctx context.Context, in FirstLoginInput) (domain.Felhasznalo, error) {
	in.FelhasznaloID = strings.TrimSpace(in.FelhasznaloID)
	in.DiscordID = strings.TrimSpace(in.DiscordID)
	in.Name = strings.TrimSpace(in.Name)
	if in.FelhasznaloID == "" || in.DiscordID == "" {
		return domain.Felhasznalo{}, fmt.Errorf("%w: felhasznalo id and discord id are required", ErrInvalidInput)
	}
	if in.Name == "" {
		in.Name = "Ismeretlen"
	}

	var result domain.Felhasznalo
	err := a.store.InTx(ctx, func(q store.Queries) error {
		existing, err := q.FindFelhasznalo(ctx, in.FelhasznaloID)
		if err == nil {
			result = existing
			return nil
		}
		if !errors.Is(err, store.ErrNotFound) {
			return err
		}

		account, err := q.FindDiscordAccount(ctx, in.DiscordID)
		if err == nil && account.FelhasznaloID != in.FelhasznaloID {
			return ErrDiscordAccountInUse
		}
		if err != nil && !errors.Is(err, store.ErrNotFound) {
			return err
		}

		now := a.clock.Now()
		result = domain.Felhasznalo{ID: in.FelhasznaloID, Name: in.Name, CreatedAt: now}
		if err := q.InsertFelhasznalo(ctx, result); err != nil {
			return err
		}
		return q.InsertDiscordAccount(ctx, domain.DiscordAccount{
			DiscordID:     in.DiscordID,
			FelhasznaloID: in.FelhasznaloID,
			IsMain:        true,
			CreatedAt:     now,
		})
	})
	if err != nil {
		return domain.Felhasznalo{}, err
	}
	return result, nil
}

// Felhasznalo returns the Felhasznalo with the given id, or ErrNotFound.
func (a *App) Felhasznalo(ctx context.Context, id string) (domain.Felhasznalo, error) {
	f, err := a.store.FindFelhasznalo(ctx, id)
	if errors.Is(err, store.ErrNotFound) {
		return domain.Felhasznalo{}, ErrNotFound
	}
	return f, err
}

// FelhasznaloByDiscordID resolves a Discord account (main or linked) to its
// Felhasznalo, or returns ErrNotFound for an unknown account. Every bot
// interaction starts here (ADR-0002).
func (a *App) FelhasznaloByDiscordID(ctx context.Context, discordID string) (domain.Felhasznalo, error) {
	account, err := a.store.FindDiscordAccount(ctx, discordID)
	if errors.Is(err, store.ErrNotFound) {
		return domain.Felhasznalo{}, ErrNotFound
	}
	if err != nil {
		return domain.Felhasznalo{}, err
	}
	return a.Felhasznalo(ctx, account.FelhasznaloID)
}

// DiscordAccounts returns every Discord account of a Felhasznalo, the main
// one first.
func (a *App) DiscordAccounts(ctx context.Context, felhasznaloID string) ([]domain.DiscordAccount, error) {
	return a.store.ListDiscordAccounts(ctx, felhasznaloID)
}
