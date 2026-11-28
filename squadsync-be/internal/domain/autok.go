package domain

import "time"

// Auto is a car offered for an Esemeny by one of its Resztvevok, its Sofor.
type Auto struct {
	ID        string
	EsemenyID string
	SoforID   string
	// SoforName is the Sofor's name, for showing.
	SoforName string
	// Seats is how many free seats it has, not counting the Sofor (1 to 8).
	Seats int
	// Note is the Sofor's free text, such as "indulás 8:00, Keleti"; ""
	// for none.
	Note      string
	CreatedAt time.Time
}

// Utas is a passenger: someone with a seat in an Auto. A Resztvevo claims
// one for themselves, a Tag for their name-only Vendeg.
type Utas struct {
	AutoID string
	Holder ClaimHolder
	// Name is the passenger's name (a name-only Vendeg's too), for showing.
	Name string
}

// SajatUton marks a Resztvevo (or a name-only Vendeg) who gets there
// on their own: they hold no seat and need none.
type SajatUton struct {
	Holder ClaimHolder
	// Name is the holder's name, for showing.
	Name string
}
