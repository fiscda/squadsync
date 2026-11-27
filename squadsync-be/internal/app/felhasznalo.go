package app

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"squadsync-be/internal/domain"
	"squadsync-be/internal/gateway"
	"squadsync-be/internal/store"
)

// Errors of the Felhasznalo area.
var (
	// ErrDiscordAccountInUse means the Discord account already belongs to
	// another Felhasznalo.
	ErrDiscordAccountInUse = errors.New("discord account already belongs to another felhasznalo")
	// ErrDiscordAccountNotOwn means an existing Felhasznalo logged in with a
	// Discord account that isn't one of theirs. Better Auth must never attach
	// a second Discord account to a user; Go owns linking (ADR-0002).
	ErrDiscordAccountNotOwn = errors.New("discord account is not one of the felhasznalo's accounts")
	// ErrMainDiscordAccount means the main Discord account can't be
	// unlinked: it is the one the Felhasznalo logs in to the web with.
	ErrMainDiscordAccount = errors.New("the main discord account can't be unlinked")
	// ErrDiscordCodeRejected means Discord refused the authorization code of
	// a linking flow (invalid, expired, reused, or for another redirect URI).
	ErrDiscordCodeRejected = fmt.Errorf("%w: discord rejected the authorization code", ErrInvalidInput)
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
// unchanged, so calling it again on a later login reuses it. The Discord
// account must then be one of theirs, or it fails with
// ErrDiscordAccountNotOwn.
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
		account, err := q.FindDiscordAccount(ctx, in.DiscordID)
		accountKnown := err == nil
		if err != nil && !errors.Is(err, store.ErrNotFound) {
			return err
		}

		existing, err := q.FindFelhasznalo(ctx, in.FelhasznaloID)
		if err == nil {
			if !accountKnown || account.FelhasznaloID != existing.ID {
				return ErrDiscordAccountNotOwn
			}
			result = existing
			return nil
		}
		if !errors.Is(err, store.ErrNotFound) {
			return err
		}

		if accountKnown {
			return ErrDiscordAccountInUse
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
			Name:          in.Name,
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

// LinkCallbackPath is the Next.js route Discord redirects to at the end of
// the linking flow. It must be registered as a redirect in the Discord
// application (README.md).
const LinkCallbackPath = "/fiok/discord/callback"

// LinkRedirectURI is the redirect URI of the linking flow: the public URL of
// the web app plus LinkCallbackPath. Go owns it, so a caller can't make Go
// exchange a code for another redirect URI. It fails with
// gateway.ErrNotConfigured when the public URL isn't set.
func (a *App) LinkRedirectURI() (string, error) {
	base := strings.TrimRight(strings.TrimSpace(a.publicURL), "/")
	if base == "" {
		return "", fmt.Errorf("public url for the link redirect: %w", gateway.ErrNotConfigured)
	}
	return base + LinkCallbackPath, nil
}

// LinkResult is the outcome of linking a Discord account.
type LinkResult struct {
	// Account is the linked account, now one of the Felhasznalo's.
	Account domain.DiscordAccount
	// AlreadyLinked is true when it already was theirs, so nothing changed.
	AlreadyLinked bool
	// RemovedFelhasznaloID is the id of the empty Felhasznalo the account
	// was taken from and who was then removed, or "". Better Auth still has
	// a user with this id; Next.js should end its sessions.
	RemovedFelhasznaloID string
}

// LinkDiscordAccount links a further Discord account to a Felhasznalo
// (ADR-0002). code comes from the Discord OAuth flow that Next.js ran, whose
// redirect URI is LinkRedirectURI; Go exchanges the code with Discord to
// learn which account authorized it.
//
//   - An account that is already theirs changes nothing.
//   - An account that belongs to another Felhasznalo who has a Tag or any
//     other data is refused with ErrDiscordAccountInUse: nothing is merged.
//   - An account whose Felhasznalo has nothing but that one account (they
//     only ever logged in once) is taken over, and that empty Felhasznalo
//     is removed.
//
// It fails with ErrNotFound for an unknown Felhasznalo and with
// ErrDiscordCodeRejected when Discord refuses the code.
func (a *App) LinkDiscordAccount(ctx context.Context, felhasznaloID, code string) (LinkResult, error) {
	code = strings.TrimSpace(code)
	if code == "" {
		return LinkResult{}, fmt.Errorf("%w: code is required", ErrInvalidInput)
	}
	if _, err := a.Felhasznalo(ctx, felhasznaloID); err != nil {
		return LinkResult{}, err
	}
	redirectURI, err := a.LinkRedirectURI()
	if err != nil {
		return LinkResult{}, err
	}

	identity, err := a.discordOAuth.ExchangeCode(ctx, code, redirectURI)
	if errors.Is(err, gateway.ErrDiscordCodeRejected) {
		return LinkResult{}, fmt.Errorf("%w (%v)", ErrDiscordCodeRejected, err)
	}
	if err != nil {
		return LinkResult{}, fmt.Errorf("exchange discord code: %w", err)
	}

	var result LinkResult
	err = a.store.InTx(ctx, func(q store.Queries) error {
		// Checked again inside the transaction: they may have just been
		// removed by someone else's takeover.
		if _, err := q.FindFelhasznalo(ctx, felhasznaloID); err != nil {
			if errors.Is(err, store.ErrNotFound) {
				return ErrNotFound
			}
			return err
		}

		linked := domain.DiscordAccount{
			DiscordID:     identity.ID,
			FelhasznaloID: felhasznaloID,
			IsMain:        false,
			Name:          identity.Name,
			CreatedAt:     a.clock.Now(),
		}

		existing, err := q.FindDiscordAccount(ctx, identity.ID)
		if errors.Is(err, store.ErrNotFound) {
			result.Account = linked
			return q.InsertDiscordAccount(ctx, linked)
		}
		if err != nil {
			return err
		}

		if existing.FelhasznaloID == felhasznaloID {
			result = LinkResult{Account: existing, AlreadyLinked: true}
			return nil
		}

		empty, err := isEmptyFelhasznalo(ctx, q, existing.FelhasznaloID)
		if err != nil {
			return err
		}
		if !empty {
			return ErrDiscordAccountInUse
		}
		if err := q.MoveDiscordAccount(ctx, linked); err != nil {
			return err
		}
		if err := q.DeleteFelhasznalo(ctx, existing.FelhasznaloID); err != nil {
			return err
		}
		result = LinkResult{Account: linked, RemovedFelhasznaloID: existing.FelhasznaloID}
		return nil
	})
	if err != nil {
		return LinkResult{}, err
	}
	return result, nil
}

// isEmptyFelhasznalo reports whether a Felhasznalo has nothing but a single
// Discord account: no Tag, no further account, no data of any kind.
func isEmptyFelhasznalo(ctx context.Context, q store.Queries, felhasznaloID string) (bool, error) {
	accounts, err := q.ListDiscordAccounts(ctx, felhasznaloID)
	if err != nil {
		return false, err
	}
	if len(accounts) != 1 {
		return false, nil
	}
	hasData, err := q.FelhasznaloHasData(ctx, felhasznaloID)
	if err != nil {
		return false, err
	}
	return !hasData, nil
}

// UnlinkDiscordAccount removes one of the Felhasznalo's linked Discord
// accounts. It then becomes unknown to the app: the bot sends it a login
// link, and it gets no more DMs. The main account can't be unlinked
// (ErrMainDiscordAccount). An account that isn't theirs is ErrNotFound.
func (a *App) UnlinkDiscordAccount(ctx context.Context, felhasznaloID, discordID string) error {
	return a.store.InTx(ctx, func(q store.Queries) error {
		account, err := q.FindDiscordAccount(ctx, discordID)
		if errors.Is(err, store.ErrNotFound) {
			return ErrNotFound
		}
		if err != nil {
			return err
		}
		if account.FelhasznaloID != felhasznaloID {
			return ErrNotFound
		}
		if account.IsMain {
			return ErrMainDiscordAccount
		}
		return q.DeleteDiscordAccount(ctx, discordID)
	})
}

// WebLoginDecision says whether a Discord account may log in to the web.
type WebLoginDecision struct {
	Allowed bool
	// MainAccountName names the main account to log in with instead, when
	// login is refused.
	MainAccountName string
}

// RefusalMessage is the message shown when the login is refused.
func (d WebLoginDecision) RefusalMessage() string {
	if d.Allowed {
		return ""
	}
	return fmt.Sprintf("Ez a fiók %s-hez van kapcsolva, lépj be azzal.", d.MainAccountName)
}

// WebLogin decides whether a Discord account may log in to the web. Only a
// Felhasznalo's main account logs in; one that Go knows as a linked
// (non-main) account is refused, naming the main account (ADR-0002). An
// unknown account is allowed: its first login creates a Felhasznalo.
//
// Better Auth asks this before creating a user and before signing in an
// existing one.
func (a *App) WebLogin(ctx context.Context, discordID string) (WebLoginDecision, error) {
	discordID = strings.TrimSpace(discordID)
	if discordID == "" {
		return WebLoginDecision{}, fmt.Errorf("%w: discord id is required", ErrInvalidInput)
	}
	account, err := a.store.FindDiscordAccount(ctx, discordID)
	if errors.Is(err, store.ErrNotFound) {
		return WebLoginDecision{Allowed: true}, nil
	}
	if err != nil {
		return WebLoginDecision{}, err
	}
	if account.IsMain {
		return WebLoginDecision{Allowed: true}, nil
	}

	accounts, err := a.store.ListDiscordAccounts(ctx, account.FelhasznaloID)
	if err != nil {
		return WebLoginDecision{}, err
	}
	name := ""
	for _, acc := range accounts {
		if acc.IsMain {
			name = acc.Name
		}
	}
	if name == "" {
		f, err := a.Felhasznalo(ctx, account.FelhasznaloID)
		if err != nil {
			return WebLoginDecision{}, err
		}
		name = f.Name
	}
	return WebLoginDecision{Allowed: false, MainAccountName: name}, nil
}
