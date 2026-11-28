package domain

import "time"

// KiegeszitoKind names one Kiegeszito. Its values are stored as text, with a
// CHECK in the kiegeszito table.
type KiegeszitoKind string

// The Kiegeszitok an Esemeny can have.
const (
	Koltsegek       KiegeszitoKind = "koltsegek"
	Autok           KiegeszitoKind = "autok"
	Szallas         KiegeszitoKind = "szallas"
	Felszereles     KiegeszitoKind = "felszereles"
	Kaja            KiegeszitoKind = "kaja"
	LejatszasiLista KiegeszitoKind = "lejatszasi_lista"
)

// KiegeszitoKinds are every Kiegeszito in the order they are shown: on the
// web and in the summary line of the Esemeny's Discord message.
var KiegeszitoKinds = []KiegeszitoKind{Koltsegek, Autok, Szallas, Felszereles, Kaja, LejatszasiLista}

// Valid tells whether k is one of the KiegeszitoKinds.
func (k KiegeszitoKind) Valid() bool {
	for _, kind := range KiegeszitoKinds {
		if k == kind {
			return true
		}
	}
	return false
}

// ClaimHolder is who a claim in a Kiegeszito (a seat, a bed, part of a
// Tetel, a Szavazas backing) is for: a Felhasznalo (a Tag, or a Vendeg who
// is a Felhasznalo), or a name-only Vendeg, whose claims their Tag makes.
// Exactly one of the two is set.
type ClaimHolder struct {
	FelhasznaloID string
	// VendegID is a name-only Vendeg's id.
	VendegID string
}

// HolderOf is the ClaimHolder of a Resztvevo: their VendegID for a
// name-only Vendeg, their FelhasznaloID otherwise.
func HolderOf(r Resztvevo) ClaimHolder {
	if r.VendegID != "" {
		return ClaimHolder{VendegID: r.VendegID}
	}
	return ClaimHolder{FelhasznaloID: r.FelhasznaloID}
}

// Tetel is one thing on an Esemeny's Felszereles list.
type Tetel struct {
	ID        string
	EsemenyID string
	Name      string
	// Quantity is how many are needed, at least 1.
	Quantity int
	// MindenkiHozza marks a Tetel everyone brings for themselves: it is a
	// reminder, not something to claim.
	MindenkiHozza bool
	// AddedBy is the Felhasznalo who added it; they and the Szervezo may
	// edit or delete it.
	AddedBy   string
	CreatedAt time.Time
}

// Vallalas is a claim of part of a Tetel's quantity, made by a Resztvevo
// for themselves or by a Tag for their name-only Vendeg.
type Vallalas struct {
	TetelID string
	Holder  ClaimHolder
	// Name is the holder's name, for showing.
	Name     string
	Quantity int
}
