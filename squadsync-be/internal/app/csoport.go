package app

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"squadsync-be/internal/domain"
	"squadsync-be/internal/store"
)

// MaxCsoportNameLength is the longest Csoport name, in characters.
const MaxCsoportNameLength = 60

// ErrCsoportMeghivoRevoked means the Csoport-meghivo exists but was revoked,
// so it no longer works.
var ErrCsoportMeghivoRevoked = errors.New("csoport-meghivo revoked")

// ErrFelhasznaloUnknown means the acting Felhasznalo doesn't exist. It is
// deliberately not ErrNotFound, so it can't be mistaken for an unknown
// Csoport or link.
var ErrFelhasznaloUnknown = errors.New("felhasznalo unknown")

// ErrNotAdmin means the acting Felhasznalo is a Tag of the Csoport but not
// an Admin, and the action needs one.
var ErrNotAdmin = errors.New("not an admin of the csoport")

// ErrLastAdmin means the action would leave the Csoport without an Admin:
// the last Admin can't drop the role or leave until another Tag is Admin.
var ErrLastAdmin = errors.New("the last admin can't give up the role or leave")

// ErrSzervezoOfActiveEsemeny means the Tag is the Szervezo of an Esemeny in
// Egyeztetes or Veglegesitve, so it can't leave the Csoport or be removed
// from it, until they hand the role over (see szervezo.go). It comes as a
// *SzervezoOfActiveEsemenyError naming the Esemenyek.
var ErrSzervezoOfActiveEsemeny = errors.New("tag is the szervezo of an active esemeny")

// CsoportDetail is a Csoport as one of its Tags sees it.
type CsoportDetail struct {
	domain.Csoport
	// Admin tells whether the viewing Tag is an Admin.
	Admin bool
	// Muted tells whether the viewing Tag muted the DMs of this Csoport.
	Muted bool
	Tags  []domain.TagWithName
	// Csatorna is the linked Discord channel, or nil if there is none.
	Csatorna *domain.Csatorna
	// Emlekeztetok are the Emlekezteto offsets, longest first.
	Emlekeztetok []time.Duration
}

// CreateCsoport creates a Csoport. The creator becomes its first Tag, with
// the Admin role.
func (a *App) CreateCsoport(ctx context.Context, felhasznaloID, name string) (domain.Csoport, error) {
	name, err := validCsoportName(name)
	if err != nil {
		return domain.Csoport{}, err
	}

	var csoport domain.Csoport
	err = a.store.InTx(ctx, func(q store.Queries) error {
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

func validCsoportName(name string) (string, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return "", fmt.Errorf("%w: a csoport needs a name", ErrInvalidInput)
	}
	if utf8.RuneCountInString(name) > MaxCsoportNameLength {
		return "", fmt.Errorf("%w: a csoport name is at most %d characters", ErrInvalidInput, MaxCsoportNameLength)
	}
	return name, nil
}

// RenameCsoport renames the Csoport, under the same rules as creating it.
// Only an Admin may: another Tag gets ErrNotAdmin, anyone else ErrNotFound.
func (a *App) RenameCsoport(ctx context.Context, felhasznaloID, csoportID, name string) (domain.Csoport, error) {
	var csoport domain.Csoport
	err := a.store.InTx(ctx, func(q store.Queries) error {
		if err := requireAdmin(ctx, q, csoportID, felhasznaloID); err != nil {
			return err
		}
		name, err := validCsoportName(name)
		if err != nil {
			return err
		}
		if err := q.RenameCsoport(ctx, csoportID, name); err != nil {
			return notFound(err)
		}
		csoport, err = q.FindCsoport(ctx, csoportID)
		return notFound(err)
	})
	if err != nil {
		return domain.Csoport{}, err
	}
	return csoport, nil
}

// SetAdmin grants or revokes the Admin role of another (or the same) Tag.
// Only an Admin may: another Tag gets ErrNotAdmin, anyone else ErrNotFound,
// as does a target who isn't a Tag. Revoking the role of the last Admin
// gives ErrLastAdmin.
func (a *App) SetAdmin(ctx context.Context, felhasznaloID, csoportID, targetID string, admin bool) error {
	return a.store.InTx(ctx, func(q store.Queries) error {
		if err := requireAdmin(ctx, q, csoportID, felhasznaloID); err != nil {
			return err
		}
		target, err := requireTag(ctx, q, csoportID, targetID)
		if err != nil {
			return err
		}
		if target.Admin == admin {
			return nil
		}
		if !admin {
			if err := requireOtherAdmin(ctx, q, csoportID); err != nil {
				return err
			}
		}
		return q.SetTagAdmin(ctx, csoportID, targetID, admin)
	})
}

// RemoveTag removes a Tag from the Csoport. Only an Admin may: another Tag
// gets ErrNotAdmin, anyone else ErrNotFound, as does a target who isn't a
// Tag.
//
// A target who is the Szervezo of active Esemenyek needs a new Szervezo for
// each that somebody else answered, named in szervezok (Esemeny id to the
// new Szervezo's Felhasznalo id); see reassignAllOf for the errors. An
// Esemeny nobody else answered is cancelled instead, since an Admin has no
// other say over it. The handovers, the cancellations, the release of the
// target's claims and the removal are one transaction: when any part is
// refused, nothing changes. Without active Esemenyek, szervezok may be nil.
func (a *App) RemoveTag(ctx context.Context, felhasznaloID, csoportID, targetID string, szervezok map[string]string) error {
	var left tagLeft
	var reassigned reassignment
	err := a.store.InTx(ctx, func(q store.Queries) error {
		if err := requireAdmin(ctx, q, csoportID, felhasznaloID); err != nil {
			return err
		}
		var err error
		if reassigned, err = a.reassignAllOf(ctx, q, csoportID, targetID, szervezok); err != nil {
			return err
		}
		left, err = a.deleteTag(ctx, q, csoportID, targetID)
		return err
	})
	if err != nil {
		return err
	}
	a.afterHandovers(reassigned.handovers)
	a.afterCancellations(felhasznaloID, reassigned.cancelled)
	a.afterTagLeft(left)
	return nil
}

// LeaveCsoport ends the Felhasznalo's own Tag. Any Tag may leave; anyone else
// gets ErrNotFound.
//
// The last Admin gets ErrLastAdmin unless they are the only Tag, and a
// Szervezo of an active Esemeny gets ErrSzervezoOfActiveEsemeny, until they
// hand the role over (HandOverSzervezo) or cancel it. When the
// last Tag leaves, the Csoport simply has no Tags; the next Felhasznalo to
// join it becomes its Admin (see JoinCsoport). The leaver's Visszajelzesek
// stay, but no longer count, and their claims in the Kiegeszitok are
// released (see deleteTag).
func (a *App) LeaveCsoport(ctx context.Context, felhasznaloID, csoportID string) error {
	var left tagLeft
	err := a.store.InTx(ctx, func(q store.Queries) error {
		var err error
		left, err = a.deleteTag(ctx, q, csoportID, felhasznaloID)
		return err
	})
	if err != nil {
		return err
	}
	a.afterTagLeft(left)
	return nil
}

// SetMuted mutes or unmutes the Felhasznalo's own DMs from the Csoport.
// Channel posts are unaffected. Anyone who isn't a Tag gets ErrNotFound.
func (a *App) SetMuted(ctx context.Context, felhasznaloID, csoportID string, muted bool) error {
	return a.store.InTx(ctx, func(q store.Queries) error {
		if _, err := requireTag(ctx, q, csoportID, felhasznaloID); err != nil {
			return err
		}
		return q.SetTagMuted(ctx, csoportID, felhasznaloID, muted)
	})
}

// tagLeft is what a Tag leaving changed beyond the Tag itself: the
// Esemenyek whose Resztvevo they were, and the Kiegeszito jobs their
// released claims queued.
type tagLeft struct {
	esemenyIDs []string
	jobs       []kiegeszitoJob
}

// afterTagLeft runs the released claims' jobs and re-renders the Esemenyek
// the Tag was a Resztvevo of, once the change commits.
func (a *App) afterTagLeft(left tagLeft) {
	if len(left.esemenyIDs) == 0 {
		return
	}
	a.afterCommit(func(ctx context.Context) {
		a.runKiegeszitoJobs(ctx, left.jobs)
		for _, id := range left.esemenyIDs {
			a.refreshEsemenyMessages(ctx, id)
		}
	})
}

// deleteTag removes the target's Tag unless that would leave the Csoport
// without an Admin or an active Esemeny without its Szervezo. Someone who
// is no longer a Tag is no Resztvevo, so in every Veglegesitve Esemeny of
// the Csoport they were a Resztvevo of, their claims in the Kiegeszitok are
// released first, as on a drop-out (without the Szervezo's DM).
func (a *App) deleteTag(ctx context.Context, q store.Queries, csoportID, targetID string) (tagLeft, error) {
	target, err := requireTag(ctx, q, csoportID, targetID)
	if err != nil {
		return tagLeft{}, err
	}
	if target.Admin {
		// A lone Tag has nobody to hand the role to, and may leave: then the
		// Csoport just has no Tags.
		tags, err := q.CountTags(ctx, csoportID)
		if err != nil {
			return tagLeft{}, err
		}
		if tags > 1 {
			if err := requireOtherAdmin(ctx, q, csoportID); err != nil {
				return tagLeft{}, err
			}
		}
	}
	szervezoOf, err := q.ListActiveEsemenyekOfSzervezo(ctx, csoportID, targetID)
	if err != nil {
		return tagLeft{}, err
	}
	if len(szervezoOf) > 0 {
		blocking := make([]EsemenyRef, len(szervezoOf))
		for n, e := range szervezoOf {
			blocking[n] = refOf(e)
		}
		return tagLeft{}, &SzervezoOfActiveEsemenyError{Esemenyek: blocking}
	}
	esemenyek, err := q.ListEsemenyekOfCsoport(ctx, csoportID)
	if err != nil {
		return tagLeft{}, err
	}
	var left tagLeft
	holder := domain.ClaimHolder{FelhasznaloID: targetID}
	for _, s := range esemenyek {
		if s.Allapot != domain.Veglegesitve {
			continue
		}
		resztvevo, err := isResztvevo(ctx, q, s.Esemeny, holder)
		if err != nil {
			return tagLeft{}, err
		}
		if !resztvevo {
			continue
		}
		_, jobs, err := a.releaseDropOut(ctx, q, s.Esemeny, targetID)
		if err != nil {
			return tagLeft{}, err
		}
		left.esemenyIDs = append(left.esemenyIDs, s.ID)
		left.jobs = append(left.jobs, jobs...)
	}
	if err := q.DeleteTag(ctx, csoportID, targetID); err != nil {
		return tagLeft{}, err
	}
	// No longer a Resztvevo anywhere in the Csoport: their calendar events go.
	return left, a.syncNaptarOfTagLeft(ctx, q, csoportID, targetID)
}

// requireOtherAdmin gives ErrLastAdmin unless the Csoport has at least two
// Admins, so that one of them can give up the role.
func requireOtherAdmin(ctx context.Context, q store.Queries, csoportID string) error {
	n, err := q.CountAdmins(ctx, csoportID)
	if err != nil {
		return err
	}
	if n < 2 {
		return ErrLastAdmin
	}
	return nil
}

// mutedCsoport tells whether the Felhasznalo muted the Csoport's DMs. Someone
// who isn't a Tag (any more) hasn't muted anything.
func (a *App) mutedCsoport(ctx context.Context, csoportID, felhasznaloID string) (bool, error) {
	tag, err := a.store.FindTag(ctx, csoportID, felhasznaloID)
	if errors.Is(err, store.ErrNotFound) {
		return false, nil
	}
	return tag.Muted, err
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
	csatorna, err := a.store.FindCsatorna(ctx, csoportID)
	if err != nil && !errors.Is(err, store.ErrNotFound) {
		return CsoportDetail{}, err
	}
	d := CsoportDetail{Csoport: csoport, Admin: tag.Admin, Muted: tag.Muted, Tags: tags}
	if err == nil {
		d.Csatorna = &csatorna
	}
	if d.Emlekeztetok, err = emlekeztetok(ctx, a.store.Queries, csoportID); err != nil {
		return CsoportDetail{}, err
	}
	return d, nil
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
// token. A Felhasznalo who joins a Csoport that has no Tags becomes its
// Admin. A Felhasznalo who already is a Tag keeps their Tag, role included,
// and gets the Csoport back even if the link has been revoked. Otherwise an
// unknown token gives ErrNotFound and a revoked one ErrCsoportMeghivoRevoked.
// An unknown Felhasznalo gives ErrFelhasznaloUnknown.
func (a *App) JoinCsoport(ctx context.Context, felhasznaloID, token string) (JoinResult, error) {
	var result JoinResult
	err := a.store.InTx(ctx, func(q store.Queries) error {
		if err := requireFelhasznalo(ctx, q, felhasznaloID); err != nil {
			return err
		}
		m, err := q.FindCsoportMeghivoByToken(ctx, token)
		if err != nil {
			return notFound(err)
		}
		csoport, err := q.FindCsoport(ctx, m.CsoportID)
		if err != nil {
			return notFound(err)
		}
		result.Csoport = csoport

		_, err = q.FindTag(ctx, csoport.ID, felhasznaloID)
		if err == nil {
			return nil
		}
		if !errors.Is(err, store.ErrNotFound) {
			return err
		}
		if m.Revoked() {
			return ErrCsoportMeghivoRevoked
		}
		result.Joined = true
		// A Csoport nobody is a Tag of has nobody to administer it, so
		// whoever joins it first becomes its Admin.
		tags, err := q.CountTags(ctx, csoport.ID)
		if err != nil {
			return err
		}
		if err := q.InsertTag(ctx, domain.Tag{
			CsoportID:     csoport.ID,
			FelhasznaloID: felhasznaloID,
			Admin:         tags == 0,
			CreatedAt:     a.clock.Now(),
		}); err != nil {
			return err
		}
		// Someone who comes back is a Resztvevo again where they said igen.
		return a.backfillNaptar(ctx, q, felhasznaloID)
	})
	if err != nil {
		return JoinResult{}, err
	}
	return result, nil
}

// TagCsoportOfMeghivo returns the id of the Csoport behind a link token if
// the Felhasznalo already is a Tag of it, even when the link was revoked, so
// the join page can send existing Tags straight in. Anyone else, and any
// unknown token, gets ErrNotFound.
func (a *App) TagCsoportOfMeghivo(ctx context.Context, felhasznaloID, token string) (string, error) {
	m, err := a.store.FindCsoportMeghivoByToken(ctx, token)
	if err != nil {
		return "", notFound(err)
	}
	if _, err := requireTag(ctx, a.store.Queries, m.CsoportID, felhasznaloID); err != nil {
		return "", err
	}
	return m.CsoportID, nil
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
	_, err := q.FindFelhasznalo(ctx, id)
	if errors.Is(err, store.ErrNotFound) {
		return ErrFelhasznaloUnknown
	}
	return err
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
