package domain

import "time"

// VendegKind tells the two kinds of Vendeg apart. Its values are stored as
// text, with a CHECK in the vendeg table.
type VendegKind string

// The kinds of Vendeg.
const (
	// VendegFelhasznalo is a Felhasznalo who joined through an
	// Esemeny-meghivo. They answer and claim for themselves.
	VendegFelhasznalo VendegKind = "felhasznalo"
	// VendegNev is a name added by a Tag, who answers and claims for them.
	VendegNev VendegKind = "nev"
)

// Vendeg is someone who attends one Esemeny without being a Tag of its
// Csoport.
type Vendeg struct {
	ID        string
	EsemenyID string
	Kind      VendegKind
	// FelhasznaloID is set for a VendegFelhasznalo, "" for a VendegNev.
	FelhasznaloID string
	// Name is the name a Tag gave a VendegNev; for a VendegFelhasznalo,
	// reads fill in the Felhasznalo's name.
	Name string
	// AddedBy is the Tag who added a VendegNev, "" for a VendegFelhasznalo.
	AddedBy string
	// AddedByName is filled in by reads, for showing.
	AddedByName string
	CreatedAt   time.Time
}

// Holder is the ClaimHolder of the Vendeg's Visszajelzesek and claims: a
// Felhasznalo Vendeg holds them as themselves, a name-only one by id.
func (v Vendeg) Holder() ClaimHolder {
	if v.Kind == VendegNev {
		return ClaimHolder{VendegID: v.ID}
	}
	return ClaimHolder{FelhasznaloID: v.FelhasznaloID}
}

// EsemenyMeghivo is a reusable link that makes whoever opens it and logs in
// a Vendeg of one Esemeny. It stops working once revoked, or once the
// Esemeny is Lezajlott or Elmaradt.
type EsemenyMeghivo struct {
	ID        string
	EsemenyID string
	// Token is the unguessable secret in the link.
	Token     string
	CreatedBy string
	CreatedAt time.Time
	// RevokedAt is nil until it is revoked.
	RevokedAt *time.Time
}

// Revoked tells whether the link has been revoked.
func (m EsemenyMeghivo) Revoked() bool { return m.RevokedAt != nil }
