package app

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"

	"squadsync-be/internal/domain"
	"squadsync-be/internal/store"
)

// MaxCsoportNameLength is the longest Csoport name, in characters.
const MaxCsoportNameLength = 60

// ErrCsoportMeghivoRevoked means the Csoport-meghivo exists but was revoked,
// so it no longer works.
var ErrCsoportMeghivoRevoked = errors.New("csoport-meghivo revoked")

// CsoportDetail is a Csoport as one of its Tags sees it.
type CsoportDetail struct {
	domain.Csoport
	// Admin tells whether the viewing Tag is an Admin.
	Admin bool
	Tags  []domain.TagWithName
}

// CreateCsoport creates a Csoport. The creator becomes its first Tag, with
// the Admin role.
func (a *App) CreateCsoport(ctx context.Context, felhasznaloID, name string) (domain.Csoport, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return domain.Csoport{}, fmt.Errorf("%w: a csoport needs a name", ErrInvalidInput)
	}
	if utf8.RuneCountInString(name) > MaxCsoportNameLength {
		return domain.Csoport{}, fmt.Errorf("%w: a csoport name is at most %d characters", ErrInvalidInput, MaxCsoportNameLength)
	}

	var csoport domain.Csoport
	err := a.store.InTx(ctx, func(q store.Queries) error {
		if err := requireFelhasznalo(ctx, q, felhasznaloID); err != nil {
			return err
		}
		now := a.clock.Now()
		csoport = domain.Csoport{ID: randomID(), Name: name, CreatedAt: now}
		if err := q.InsertCsoport(ctx, csoport); err != nil {
			return err
		}
		return q.InsertTag(ctx, domain.Tag{
			CsoportID:     csoport.ID,
			FelhasznaloID: felhasznaloID,
			Admin:         true,
			CreatedAt:     now,
		})
	})
	if err != nil {
		return domain.Csoport{}, err
	}
	return csoport, nil
}

// CsoportokOf lists every Csoport the Felhasznalo is a Tag of, in the order
// they became a Tag of them.
func (a *App) CsoportokOf(ctx context.Context, felhasznaloID string) ([]domain.CsoportSummary, error) {
	return a.store.ListCsoportokOf(ctx, felhasznaloID)
}

// Csoport returns a Csoport with its Tags. Only Tags may see it: anyone else
// gets ErrNotFound, so a Csoport's existence isn't leaked.
func (a *App) Csoport(ctx context.Context, felhasznaloID, csoportID string) (CsoportDetail, error) {
	tag, err := requireTag(ctx, a.store.Queries, csoportID, felhasznaloID)
	if err != nil {
		return CsoportDetail{}, err
	}
	csoport, err := a.store.FindCsoport(ctx, csoportID)
	if err != nil {
		return CsoportDetail{}, notFound(err)
	}
	tags, err := a.store.ListTags(ctx, csoportID)
	if err != nil {
		return CsoportDetail{}, err
	}
	return CsoportDetail{Csoport: csoport, Admin: tag.Admin, Tags: tags}, nil
}

// CreateCsoportMeghivo makes a new reusable Csoport-meghivo. Any Tag may do
// it; anyone else gets ErrNotFound.
func (a *App) CreateCsoportMeghivo(ctx context.Context, felhasznaloID, csoportID string) (domain.CsoportMeghivo, error) {
	if _, err := requireTag(ctx, a.store.Queries, csoportID, felhasznaloID); err != nil {
		return domain.CsoportMeghivo{}, err
	}
	m := domain.CsoportMeghivo{
		ID:        randomID(),
		CsoportID: csoportID,
		Token:     randomToken(),
		CreatedBy: felhasznaloID,
		CreatedAt: a.clock.Now(),
	}
	if err := a.store.InsertCsoportMeghivo(ctx, m); err != nil {
		return domain.CsoportMeghivo{}, err
	}
	return m, nil
}

// CsoportMeghivok lists the Csoport's Csoport-meghivok that still work,
// newest first. Only Tags may see them; anyone else gets ErrNotFound.
func (a *App) CsoportMeghivok(ctx context.Context, felhasznaloID, csoportID string) ([]domain.CsoportMeghivo, error) {
	if _, err := requireTag(ctx, a.store.Queries, csoportID, felhasznaloID); err != nil {
		return nil, err
	}
	return a.store.ListActiveCsoportMeghivok(ctx, csoportID)
}

// RevokeCsoportMeghivo stops a Csoport-meghivo from working. Any Tag of its
// Csoport may do it; anyone else gets ErrNotFound, as does an id that
// belongs to another Csoport. Revoking twice is fine.
func (a *App) RevokeCsoportMeghivo(ctx context.Context, felhasznaloID, csoportID, meghivoID string) error {
	return a.store.InTx(ctx, func(q store.Queries) error {
		if _, err := requireTag(ctx, q, csoportID, felhasznaloID); err != nil {
			return err
		}
		m, err := q.FindCsoportMeghivo(ctx, meghivoID)
		if err != nil {
			return notFound(err)
		}
		if m.CsoportID != csoportID {
			return ErrNotFound
		}
		return q.RevokeCsoportMeghivo(ctx, m.ID, a.clock.Now())
	})
}

// CsoportMeghivoPreview returns the Csoport behind a link token, so the join
// page can say which Csoport it is about. Whoever holds the link may see the
// Csoport's name. An unknown token gives ErrNotFound, a revoked one
// ErrCsoportMeghivoRevoked.
func (a *App) CsoportMeghivoPreview(ctx context.Context, token string) (domain.Csoport, error) {
	return a.workingCsoport(ctx, a.store.Queries, token)
}

// JoinResult is the outcome of opening a Csoport-meghivo.
type JoinResult struct {
	Csoport domain.Csoport
	// Joined is false when the Felhasznalo already was a Tag, in which case
	// nothing changed.
	Joined bool
}

// JoinCsoport makes the Felhasznalo a Tag of the Csoport behind the link
// token. A Felhasznalo who already is a Tag keeps their Tag, role included.
// An unknown token gives ErrNotFound, a revoked one ErrCsoportMeghivoRevoked.
func (a *App) JoinCsoport(ctx context.Context, felhasznaloID, token string) (JoinResult, error) {
	var result JoinResult
	err := a.store.InTx(ctx, func(q store.Queries) error {
		if err := requireFelhasznalo(ctx, q, felhasznaloID); err != nil {
			return err
		}
		csoport, err := a.workingCsoport(ctx, q, token)
		if err != nil {
			return err
		}
		result.Csoport = csoport

		_, err = q.FindTag(ctx, csoport.ID, felhasznaloID)
		if err == nil {
			return nil
		}
		if !errors.Is(err, store.ErrNotFound) {
			return err
		}
		result.Joined = true
		return q.InsertTag(ctx, domain.Tag{
			CsoportID:     csoport.ID,
			FelhasznaloID: felhasznaloID,
			CreatedAt:     a.clock.Now(),
		})
	})
	if err != nil {
		return JoinResult{}, err
	}
	return result, nil
}

// workingCsoport finds the Csoport behind a link token, refusing an unknown
// or revoked link.
func (a *App) workingCsoport(ctx context.Context, q store.Queries, token string) (domain.Csoport, error) {
	m, err := q.FindCsoportMeghivoByToken(ctx, token)
	if err != nil {
		return domain.Csoport{}, notFound(err)
	}
	if m.Revoked() {
		return domain.Csoport{}, ErrCsoportMeghivoRevoked
	}
	csoport, err := q.FindCsoport(ctx, m.CsoportID)
	if err != nil {
		return domain.Csoport{}, notFound(err)
	}
	return csoport, nil
}

// requireTag returns the Felhasznalo's Tag in the Csoport. Anyone without one
// gets ErrNotFound, whether or not the Csoport exists.
func requireTag(ctx context.Context, q store.Queries, csoportID, felhasznaloID string) (domain.Tag, error) {
	tag, err := q.FindTag(ctx, csoportID, felhasznaloID)
	if err != nil {
		return domain.Tag{}, notFound(err)
	}
	return tag, nil
}

func requireFelhasznalo(ctx context.Context, q store.Queries, id string) error {
	if _, err := q.FindFelhasznalo(ctx, id); err != nil {
		return notFound(err)
	}
	return nil
}

// notFound turns the store's not-found into the application's.
func notFound(err error) error {
	if errors.Is(err, store.ErrNotFound) {
		return ErrNotFound
	}
	return err
}

// randomID returns a random 128-bit id as hex.
func randomID() string {
	return hex.EncodeToString(randomBytes(16))
}

// randomToken returns an unguessable 256-bit secret for a link, URL-safe.
func randomToken() string {
	return base64.RawURLEncoding.EncodeToString(randomBytes(32))
}

func randomBytes(n int) []byte {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		panic(fmt.Sprintf("read random bytes: %v", err)) // crypto/rand doesn't fail on supported platforms
	}
	return b
}
