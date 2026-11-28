package app

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"

	"squadsync-be/internal/domain"
	"squadsync-be/internal/gateway"
	"squadsync-be/internal/store"
)

// Vendegek (#13): people who attend one Esemeny without being a Tag of its
// Csoport.
//
//   - A Felhasznalo becomes a Vendeg through an Esemeny-meghivo, a reusable
//     link any Tag creates and revokes. It stops working once the Esemeny
//     is Lezajlott or Elmaradt. A Tag, or someone who already is a Vendeg,
//     who opens it just lands on the Esemeny.
//   - A name-only Vendeg is a name any Tag adds. Only that Tag answers for
//     them and claims for them; that Tag or the Szervezo removes them.
//
// Who may do what is decided by requireEsemenyMember, which returns the
// caller's role, and by requireEsemenyTag for what stays Tag-only:
//
//   - Tags and Vendegek: see the Esemeny (Vendegek without its Csoport),
//     answer its Idopontok, see its Kiegeszitok and claim in them once
//     they are Resztvevok (claims follow requireClaimAllowed), record and
//     settle Kiadasok as a Resztvevo.
//   - Tags only: propose, add or remove Idopontok, copy, be or become the
//     Szervezo (so switch Kiegeszitok and set them up), add a Tetel during
//     Egyeztetes, create Esemeny-meghivok, add name-only Vendegek, see
//     anyone's Raeres or the Csoport.
//
// Counting: a Vendeg's Visszajelzes never counts toward the Helyezes (only
// Tags' do), but a Vendeg of either kind with igen on the confirmed Idopont
// is a Resztvevo (countsAsResztvevo), so in the headcount, the Kiegeszitok
// and the Koltsegek. "X/Y szabad aznap" counts Tags only.
//
// DMs: a Vendeg who is a Felhasznalo gets no vote request, but gets the
// Emlekeztetok (igen or talan), the state changes of the Esemeny (always by
// DM, as they aren't in the Csoport's Csatorna), the Autok and Szallas DMs
// that concern them, and the Elszamolas reminder and result. A name-only
// Vendeg has no Discord: they get nothing, and their Tag gets nothing extra
// on their behalf, except the Elszamolas reminder, which their Tag must act
// on.

// MaxVendegNameLength is the longest name of a name-only Vendeg.
const MaxVendegNameLength = 60

// Errors of Vendegek.
var (
	// ErrEsemenyMeghivoRevoked means the Esemeny-meghivo was revoked.
	ErrEsemenyMeghivoRevoked = errors.New("esemeny-meghivo revoked")
	// ErrEsemenyMeghivoExpired means the Esemeny-meghivo's Esemeny is
	// Lezajlott or Elmaradt, so it no longer works.
	ErrEsemenyMeghivoExpired = errors.New("esemeny-meghivo expired: the esemeny is lezajlott or elmaradt")
	// ErrVendegClosed means Vendegek can be invited or added only while the
	// Esemeny is in Egyeztetes or Veglegesitve.
	ErrVendegClosed = errors.New("vendegek join only while the esemeny is in egyeztetes or veglegesitve")
	// ErrVendegHasKiadas means the Vendeg can't be removed because they are
	// in a Kiadas (a share, or as its payer or creator): edit it first.
	ErrVendegHasKiadas = errors.New("the vendeg is in a kiadas")
)

// esemenyRole is how someone takes part in an Esemeny.
type esemenyRole int

const (
	// roleTag is a Tag of the Esemeny's Csoport.
	roleTag esemenyRole = iota + 1
	// roleVendeg is a Vendeg of the Esemeny who is a Felhasznalo.
	roleVendeg
)

// requireEsemenyMember returns the Esemeny and the Felhasznalo's role in it:
// roleTag for a Tag of its Csoport (even one who is also a Vendeg), or else
// roleVendeg for a Vendeg of the Esemeny. Anyone else gets ErrNotFound,
// whether or not the Esemeny exists. Use it where Vendegek are allowed, and
// requireEsemenyTag where only Tags are.
func requireEsemenyMember(ctx context.Context, q store.Queries, esemenyID, felhasznaloID string) (domain.Esemeny, esemenyRole, error) {
	e, err := q.FindEsemeny(ctx, esemenyID)
	if err != nil {
		return domain.Esemeny{}, 0, notFound(err)
	}
	if _, err := q.FindTag(ctx, e.CsoportID, felhasznaloID); err == nil {
		return e, roleTag, nil
	} else if !errors.Is(err, store.ErrNotFound) {
		return domain.Esemeny{}, 0, err
	}
	if _, err := q.FindVendegOfFelhasznalo(ctx, e.ID, felhasznaloID); err != nil {
		return domain.Esemeny{}, 0, notFound(err)
	}
	return e, roleVendeg, nil
}

// vendegJoinable tells whether Vendegek can join the Esemeny now: while it
// is in Egyeztetes or Veglegesitve.
func vendegJoinable(e domain.Esemeny) bool {
	return e.Allapot == domain.Egyeztetes || e.Allapot == domain.Veglegesitve
}

// VendegAllas is a Vendeg as one viewer sees it.
type VendegAllas struct {
	domain.Vendeg
	// CanAnswer tells whether the viewer answers for them: the Tag who
	// added a name-only Vendeg.
	CanAnswer bool
	// CanRemove tells whether the viewer may remove them (see RemoveVendeg).
	CanRemove bool
}

// vendegAllasok lists the Esemeny's Vendegek for the viewer.
func vendegAllasok(ctx context.Context, q store.Queries, e domain.Esemeny, viewerID string) ([]VendegAllas, error) {
	vendegek, err := q.ListVendegek(ctx, e.ID)
	if err != nil {
		return nil, err
	}
	out := make([]VendegAllas, 0, len(vendegek))
	for _, v := range vendegek {
		out = append(out, VendegAllas{
			Vendeg:    v,
			CanAnswer: v.Kind == domain.VendegNev && v.AddedBy == viewerID,
			CanRemove: mayRemoveVendeg(e, v, viewerID),
		})
	}
	return out, nil
}

// mayRemoveVendeg tells whether the Felhasznalo may remove the Vendeg: the
// Szervezo anyone, the Tag who added a name-only Vendeg theirs.
func mayRemoveVendeg(e domain.Esemeny, v domain.Vendeg, felhasznaloID string) bool {
	return e.SzervezoID == felhasznaloID || (v.Kind == domain.VendegNev && v.AddedBy == felhasznaloID)
}

// CreateEsemenyMeghivo makes a new reusable Esemeny-meghivo. Any Tag may,
// while the Esemeny is in Egyeztetes or Veglegesitve (ErrVendegClosed);
// anyone else gets ErrNotFound.
func (a *App) CreateEsemenyMeghivo(ctx context.Context, felhasznaloID, esemenyID string) (domain.EsemenyMeghivo, error) {
	var m domain.EsemenyMeghivo
	err := a.store.InTx(ctx, func(q store.Queries) error {
		e, err := requireEsemenyTag(ctx, q, esemenyID, felhasznaloID)
		if err != nil {
			return err
		}
		if !vendegJoinable(e) {
			return ErrVendegClosed
		}
		m = domain.EsemenyMeghivo{
			ID:        randomID(),
			EsemenyID: e.ID,
			Token:     randomToken(),
			CreatedBy: felhasznaloID,
			CreatedAt: a.clock.Now(),
		}
		return q.InsertEsemenyMeghivo(ctx, m)
	})
	if err != nil {
		return domain.EsemenyMeghivo{}, err
	}
	return m, nil
}

// EsemenyMeghivok lists the Esemeny's Esemeny-meghivok that still work,
// newest first: none once it is Lezajlott or Elmaradt. Only Tags may see
// them; anyone else gets ErrNotFound.
func (a *App) EsemenyMeghivok(ctx context.Context, felhasznaloID, esemenyID string) ([]domain.EsemenyMeghivo, error) {
	q := a.store.Queries
	e, err := requireEsemenyTag(ctx, q, esemenyID, felhasznaloID)
	if err != nil {
		return nil, err
	}
	if !vendegJoinable(e) {
		return []domain.EsemenyMeghivo{}, nil
	}
	list, err := q.ListActiveEsemenyMeghivok(ctx, e.ID)
	if list == nil && err == nil {
		list = []domain.EsemenyMeghivo{}
	}
	return list, err
}

// RevokeEsemenyMeghivo stops an Esemeny-meghivo from working. Any Tag of
// its Esemeny's Csoport may; anyone else gets ErrNotFound, as does an id
// that belongs to another Esemeny. Revoking twice is fine.
func (a *App) RevokeEsemenyMeghivo(ctx context.Context, felhasznaloID, esemenyID, meghivoID string) error {
	return a.store.InTx(ctx, func(q store.Queries) error {
		e, err := requireEsemenyTag(ctx, q, esemenyID, felhasznaloID)
		if err != nil {
			return err
		}
		m, err := q.FindEsemenyMeghivo(ctx, meghivoID)
		if err != nil {
			return notFound(err)
		}
		if m.EsemenyID != e.ID {
			return ErrNotFound
		}
		return q.RevokeEsemenyMeghivo(ctx, m.ID, a.clock.Now())
	})
}

// EsemenyMeghivoPreview is what the join page shows about an
// Esemeny-meghivo before joining.
type EsemenyMeghivoPreview struct {
	EsemenyID    string
	Title        string
	SzervezoName string
}

// workingEsemenyMeghivo finds the Esemeny behind a link token, refusing an
// unknown (ErrNotFound), revoked (ErrEsemenyMeghivoRevoked) or expired
// (ErrEsemenyMeghivoExpired) link.
func workingEsemenyMeghivo(ctx context.Context, q store.Queries, token string) (domain.Esemeny, error) {
	m, err := q.FindEsemenyMeghivoByToken(ctx, token)
	if err != nil {
		return domain.Esemeny{}, notFound(err)
	}
	e, err := q.FindEsemeny(ctx, m.EsemenyID)
	if err != nil {
		return domain.Esemeny{}, notFound(err)
	}
	if m.Revoked() {
		return domain.Esemeny{}, ErrEsemenyMeghivoRevoked
	}
	if !vendegJoinable(e) {
		return domain.Esemeny{}, ErrEsemenyMeghivoExpired
	}
	return e, nil
}

// EsemenyMeghivoPreview returns the Esemeny behind a link token, so the
// join page can say what it is about. Whoever holds the link may see its
// title and Szervezo. The errors are those of a link that doesn't work:
// ErrNotFound, ErrEsemenyMeghivoRevoked, ErrEsemenyMeghivoExpired.
func (a *App) EsemenyMeghivoPreview(ctx context.Context, token string) (EsemenyMeghivoPreview, error) {
	q := a.store.Queries
	e, err := workingEsemenyMeghivo(ctx, q, token)
	if err != nil {
		return EsemenyMeghivoPreview{}, err
	}
	szervezo, err := q.FindFelhasznalo(ctx, e.SzervezoID)
	if err != nil {
		return EsemenyMeghivoPreview{}, err
	}
	return EsemenyMeghivoPreview{EsemenyID: e.ID, Title: e.Title, SzervezoName: szervezo.Name}, nil
}

// EsemenyJoinResult is the outcome of opening an Esemeny-meghivo.
type EsemenyJoinResult struct {
	EsemenyID string
	// Joined is true when the Felhasznalo just became a Vendeg; false when
	// they already were one, or are a Tag, and nothing changed.
	Joined bool
}

// JoinEsemeny makes the Felhasznalo a Vendeg of the Esemeny behind the link
// token, of that Esemeny only. A Tag of its Csoport, or someone who already
// is a Vendeg, gets the Esemeny back unchanged, even if the link no longer
// works. Otherwise an unknown token gives ErrNotFound, a revoked one
// ErrEsemenyMeghivoRevoked, and one whose Esemeny is Lezajlott or Elmaradt
// ErrEsemenyMeghivoExpired. An unknown Felhasznalo gives
// ErrFelhasznaloUnknown.
func (a *App) JoinEsemeny(ctx context.Context, felhasznaloID, token string) (EsemenyJoinResult, error) {
	var result EsemenyJoinResult
	err := a.store.InTx(ctx, func(q store.Queries) error {
		if err := requireFelhasznalo(ctx, q, felhasznaloID); err != nil {
			return err
		}
		m, err := q.FindEsemenyMeghivoByToken(ctx, token)
		if err != nil {
			return notFound(err)
		}
		result.EsemenyID = m.EsemenyID
		if _, _, err := requireEsemenyMember(ctx, q, m.EsemenyID, felhasznaloID); err == nil {
			return nil
		} else if !errors.Is(err, ErrNotFound) {
			return err
		}
		e, err := workingEsemenyMeghivo(ctx, q, token)
		if err != nil {
			return err
		}
		result.Joined = true
		return q.InsertVendeg(ctx, domain.Vendeg{
			ID:            randomID(),
			EsemenyID:     e.ID,
			Kind:          domain.VendegFelhasznalo,
			FelhasznaloID: felhasznaloID,
			CreatedAt:     a.clock.Now(),
		})
	})
	if err != nil {
		return EsemenyJoinResult{}, err
	}
	return result, nil
}

// MemberEsemenyOfMeghivo returns the id of the Esemeny behind a link token
// if the Felhasznalo already takes part in it (a Tag or a Vendeg), even
// when the link no longer works, so the join page can send them straight
// in. Anyone else, and any unknown token, gets ErrNotFound.
func (a *App) MemberEsemenyOfMeghivo(ctx context.Context, felhasznaloID, token string) (string, error) {
	q := a.store.Queries
	m, err := q.FindEsemenyMeghivoByToken(ctx, token)
	if err != nil {
		return "", notFound(err)
	}
	if _, _, err := requireEsemenyMember(ctx, q, m.EsemenyID, felhasznaloID); err != nil {
		return "", err
	}
	return m.EsemenyID, nil
}

// AddVendeg adds a name-only Vendeg to the Esemeny. Any Tag may, while it
// is in Egyeztetes or Veglegesitve (ErrVendegClosed); that Tag then answers
// for them. Anyone else gets ErrNotFound; an empty or too long name
// ErrInvalidInput.
func (a *App) AddVendeg(ctx context.Context, felhasznaloID, esemenyID, name string) (domain.Vendeg, error) {
	name = strings.TrimSpace(name)
	if name == "" || utf8.RuneCountInString(name) > MaxVendegNameLength {
		return domain.Vendeg{}, fmt.Errorf("%w: a vendeg's name is 1 to %d characters", ErrInvalidInput, MaxVendegNameLength)
	}
	var v domain.Vendeg
	err := a.store.InTx(ctx, func(q store.Queries) error {
		e, err := requireEsemenyTag(ctx, q, esemenyID, felhasznaloID)
		if err != nil {
			return err
		}
		if !vendegJoinable(e) {
			return ErrVendegClosed
		}
		v = domain.Vendeg{
			ID:        randomID(),
			EsemenyID: e.ID,
			Kind:      domain.VendegNev,
			Name:      name,
			AddedBy:   felhasznaloID,
			CreatedAt: a.clock.Now(),
		}
		return q.InsertVendeg(ctx, v)
	})
	if err != nil {
		return domain.Vendeg{}, err
	}
	return v, nil
}

// RemoveVendeg removes a Vendeg from the Esemeny, with their Visszajelzesek
// and their claims in every Kiegeszito (released as on a drop-out, so a
// Sofor's passengers are told). The Szervezo may remove any Vendeg, the
// Tag who added a name-only Vendeg theirs; another Tag gets
// ErrNotPermitted, anyone else ErrNotFound, as does a Vendeg of another
// Esemeny. A Vendeg who is in a Kiadas gives ErrVendegHasKiadas.
func (a *App) RemoveVendeg(ctx context.Context, felhasznaloID, esemenyID, vendegID string) error {
	var jobs []kiegeszitoJob
	err := a.store.InTx(ctx, func(q store.Queries) error {
		e, err := requireEsemenyTag(ctx, q, esemenyID, felhasznaloID)
		if err != nil {
			return err
		}
		v, err := q.FindVendeg(ctx, vendegID)
		if err != nil {
			return notFound(err)
		}
		if v.EsemenyID != e.ID {
			return ErrNotFound
		}
		if !mayRemoveVendeg(e, v, felhasznaloID) {
			return ErrNotPermitted
		}
		holder := v.Holder()
		hasKiadas, err := q.HasKiadasOf(ctx, e.ID, holder)
		if err != nil {
			return err
		}
		if hasKiadas {
			return ErrVendegHasKiadas
		}
		// A Felhasznalo Vendeg who has joined the Csoport since keeps
		// everything: they take part as a Tag.
		if v.Kind == domain.VendegFelhasznalo {
			if _, err := q.FindTag(ctx, e.CsoportID, v.FelhasznaloID); err == nil {
				return q.DeleteVendeg(ctx, v.ID)
			} else if !errors.Is(err, store.ErrNotFound) {
				return err
			}
		}
		_, jobs, err = a.releaseClaims(ctx, q, e, func(h domain.ClaimHolder) bool { return h == holder })
		if err != nil {
			return err
		}
		if err := q.DeleteElszamolasMegerosites(ctx, e.ID, holder); err != nil {
			return err
		}
		if v.Kind == domain.VendegFelhasznalo {
			if err := q.DeleteVisszajelzesekOf(ctx, e.ID, v.FelhasznaloID); err != nil {
				return err
			}
		}
		return q.DeleteVendeg(ctx, v.ID)
	})
	if err != nil {
		return err
	}
	a.kiegeszitoChanged(esemenyID, jobs...)
	return nil
}

// SetVendegVisszajelzes records the answer of a name-only Vendeg to one
// Idopont, given by the Tag who added them, under the rules of
// SetVisszajelzes (a drop-out from igen releases the Vendeg's claims and
// tells the Szervezo). Another Tag gets ErrNotPermitted; anyone else, a
// Vendeg of another Esemeny or a Vendeg who is a Felhasznalo (they answer
// for themselves) gets ErrNotFound.
func (a *App) SetVendegVisszajelzes(ctx context.Context, felhasznaloID, esemenyID, idopontID, vendegID string, valasz domain.Valasz) error {
	return a.setVisszajelzes(ctx, felhasznaloID, esemenyID, idopontID, vendegID, valasz)
}

// answeringFor checks who a Visszajelzes is for, inside setVisszajelzes's
// transaction: the caller themselves (vendegID "", Tag or Vendeg), or the
// name-only Vendeg the calling Tag added. It returns the Esemeny, the
// holder and their name.
func answeringFor(ctx context.Context, q store.Queries, esemenyID, felhasznaloID, vendegID string) (domain.Esemeny, domain.ClaimHolder, string, error) {
	if vendegID == "" {
		e, _, err := requireEsemenyMember(ctx, q, esemenyID, felhasznaloID)
		if err != nil {
			return domain.Esemeny{}, domain.ClaimHolder{}, "", err
		}
		f, err := q.FindFelhasznalo(ctx, felhasznaloID)
		if err != nil {
			return domain.Esemeny{}, domain.ClaimHolder{}, "", err
		}
		return e, domain.ClaimHolder{FelhasznaloID: felhasznaloID}, f.Name, nil
	}
	e, err := requireEsemenyTag(ctx, q, esemenyID, felhasznaloID)
	if err != nil {
		return domain.Esemeny{}, domain.ClaimHolder{}, "", err
	}
	v, err := q.FindVendeg(ctx, vendegID)
	if err != nil {
		return domain.Esemeny{}, domain.ClaimHolder{}, "", notFound(err)
	}
	if v.EsemenyID != e.ID || v.Kind != domain.VendegNev {
		return domain.Esemeny{}, domain.ClaimHolder{}, "", ErrNotFound
	}
	if v.AddedBy != felhasznaloID {
		return domain.Esemeny{}, domain.ClaimHolder{}, "", ErrNotPermitted
	}
	return e, v.Holder(), v.Name, nil
}

// requireVendegClaim checks a claim for a name-only Vendeg (see
// requireClaimAllowedBy): the Vendeg belongs to the Esemeny (ErrNotFound
// otherwise), and actorID is the Tag who added them, or the Szervezo when
// allowed to move people (ErrNotPermitted otherwise).
func requireVendegClaim(ctx context.Context, q store.Queries, e domain.Esemeny, actorID, vendegID string, bySzervezo bool) error {
	v, err := q.FindVendeg(ctx, vendegID)
	if err != nil {
		return fmt.Errorf("%w: no such vendeg", notFound(err))
	}
	if v.EsemenyID != e.ID || v.Kind != domain.VendegNev {
		return fmt.Errorf("%w: no such vendeg", ErrNotFound)
	}
	if v.AddedBy != actorID && !bySzervezo {
		return ErrNotPermitted
	}
	return nil
}

// announceToVendegek DMs a change of the Esemeny's state to every Vendeg
// who is a Felhasznalo (but the actor, and anyone who is also a Tag: they
// get it as Tags). Vendegek aren't in the Csoport's Csatorna, so they get
// it by DM whether or not the Csoport has one.
func (a *App) announceToVendegek(ctx context.Context, e domain.Esemeny, actorID string, ann gateway.Announcement) {
	vendegek, err := a.store.ListVendegek(ctx, e.ID)
	if err != nil {
		a.log.Error("announce esemeny allapot to vendegek", "esemeny", e.ID, "err", err)
		return
	}
	for _, v := range vendegek {
		if v.Kind != domain.VendegFelhasznalo || v.FelhasznaloID == actorID {
			continue
		}
		if _, err := a.store.FindTag(ctx, e.CsoportID, v.FelhasznaloID); err == nil {
			continue
		}
		a.sendAnnouncementDM(ctx, e.CsoportID, v.FelhasznaloID, ann)
	}
}
