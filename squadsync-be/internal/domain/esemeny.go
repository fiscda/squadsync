package domain

import "time"

// Allapot is the state of an Esemeny. Its values are stored as text, so new
// states only need a new constant (and the CHECK in the esemeny table).
type Allapot string

// The states of an Esemeny: Egyeztetes → Veglegesitve → Lezajlott, or
// Elmaradt.
const (
	// Egyeztetes means its Idopontok are being answered.
	Egyeztetes Allapot = "egyeztetes"
	// Veglegesitve means one Idopont is confirmed.
	Veglegesitve Allapot = "veglegesitve"
	// Elmaradt means it was cancelled. It is final.
	Elmaradt Allapot = "elmaradt"
	// Lezajlott means the confirmed Idopont has ended (or started, if it has
	// no end).
	Lezajlott Allapot = "lezajlott"
)

// Esemeny is a single gathering of one Csoport.
type Esemeny struct {
	ID          string
	CsoportID   string
	Title       string
	Description string
	Helyszin    Helyszin
	// SzervezoID is the Felhasznalo responsible for the Esemeny. It starts as
	// whoever proposed it.
	SzervezoID string
	Allapot    Allapot
	// ConfirmedIdopontID is the stored confirmed Idopont: set in
	// Veglegesitve and Lezajlott, "" in Egyeztetes. Elmaradt keeps it only
	// as a record of what was planned, so read it through the core's
	// confirmedIdopont, which ignores it there.
	ConfirmedIdopontID string
	CreatedAt          time.Time
}

// Helyszin is where an Esemeny takes place. Both parts are optional; an
// empty Helyszin means none was given.
type Helyszin struct {
	Name string
	// URL is an absolute http(s) URL, meant for a Google Maps link.
	URL string
}

// Idopont is one candidate time of an Esemeny. Times are in UTC.
type Idopont struct {
	ID        string
	EsemenyID string
	Start     time.Time
	// End is nil when the Idopont has only a start. Otherwise it is after
	// Start, possibly several days later.
	End *time.Time
	// EgeszNapos marks a whole-day Idopont. It runs from Budapest midnight
	// of its first day (Start) to Budapest midnight after its last day (End,
	// always set), and is shown as dates only.
	EgeszNapos bool
	// AddedBy is the Felhasznalo who added the Idopont.
	AddedBy   string
	CreatedAt time.Time
}

// Valasz is the content of a Visszajelzes.
type Valasz string

// The three possible answers to an Idopont.
const (
	Igen  Valasz = "igen"
	Talan Valasz = "talan"
	Nem   Valasz = "nem"
)

// Valid tells whether v is one of igen, talan or nem.
func (v Valasz) Valid() bool {
	return v == Igen || v == Talan || v == Nem
}

// Visszajelzes is one answer to one Idopont, given by a Felhasznalo, or by
// a Tag for a name-only Vendeg they added. Exactly one of FelhasznaloID and
// VendegID is set. There is at most one per Idopont and person; answering
// again changes it.
type Visszajelzes struct {
	IdopontID     string
	FelhasznaloID string
	// VendegID is the name-only Vendeg the answer is for, "" otherwise.
	VendegID  string
	Valasz    Valasz
	UpdatedAt time.Time
}

// Holder is who the Visszajelzes is for, as a ClaimHolder.
func (v Visszajelzes) Holder() ClaimHolder {
	if v.VendegID != "" {
		return ClaimHolder{VendegID: v.VendegID}
	}
	return ClaimHolder{FelhasznaloID: v.FelhasznaloID}
}

// VisszajelzesOfEsemeny is a Visszajelzes as an Esemeny's page shows it:
// with the name of whoever it is for, whether they are a Tag of the
// Esemeny's Csoport (only Tags count toward the Helyezes) and whether they
// are a Vendeg of the Esemeny. A name-only Vendeg's has an empty
// FelhasznaloID, their name as FelhasznaloName, and VendegAddedBy set.
type VisszajelzesOfEsemeny struct {
	Visszajelzes
	FelhasznaloName string
	IsTag           bool
	// IsVendeg tells whether they are a Vendeg of the Esemeny, of either
	// kind. Someone can be both a Tag and a Vendeg (a Vendeg who later
	// joined the Csoport); they count as a Tag.
	IsVendeg bool
	// VendegAddedBy is the Tag who added a name-only Vendeg, and who alone
	// answers for them; "" otherwise.
	VendegAddedBy string
}

// Resztvevo is someone with igen on the confirmed Idopont of a Veglegesitve
// (or Lezajlott) Esemeny: a Tag, a Vendeg who is a Felhasznalo (FelhasznaloID
// set, IsTag false, IsVendeg true), or a name-only Vendeg (FelhasznaloID
// empty, VendegID and AddedBy set).
type Resztvevo struct {
	FelhasznaloID string
	// VendegID is a name-only Vendeg's id, "" otherwise.
	VendegID string
	Name     string
	IsTag    bool
	// IsVendeg tells whether they are a Vendeg rather than a Tag.
	IsVendeg bool
	// AddedBy is the Tag who added a name-only Vendeg, "" otherwise: their
	// Tag claims and confirms for them and pays their share.
	AddedBy string
}

// EsemenySummary is an Esemeny in a Csoport's list.
type EsemenySummary struct {
	Esemeny
	IdopontCount int
	// FirstStart is the earliest Idopont's start, nil if it has none.
	FirstStart *time.Time
}
