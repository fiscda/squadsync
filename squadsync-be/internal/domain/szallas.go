package domain

import "time"

// Szoba is a room of an Esemeny's Szallas.
type Szoba struct {
	ID        string
	EsemenyID string
	Name      string
	// Position orders the Szobak, lowest first.
	Position  int
	CreatedAt time.Time
}

// Agy is a bed in a Szoba, sleeping one or two people.
type Agy struct {
	ID        string
	SzobaID   string
	EsemenyID string
	// Capacity is the femohely: 1 for a single bed, 2 for a double.
	Capacity int
	// Label is an optional short text such as "emeletes ágy, felső"; ""
	// for none.
	Label string
	// Position orders the Agyak within the Szoba, lowest first.
	Position  int
	CreatedAt time.Time
}

// Foglalas is a spot taken in an Agy. A Resztvevo claims one for
// themselves, a Tag for their name-only Vendeg (#13).
type Foglalas struct {
	AgyID  string
	Holder ClaimHolder
	// Name is the sleeper's name, for showing. A name-only Vendeg's is empty
	// until #13 joins vendeg here.
	Name string
}

// MasholAlszik marks a Resztvevo (or, #13, a name-only Vendeg) who sleeps
// somewhere else: they hold no spot and need none.
type MasholAlszik struct {
	Holder ClaimHolder
	// Name is the holder's name, for showing.
	Name string
}
