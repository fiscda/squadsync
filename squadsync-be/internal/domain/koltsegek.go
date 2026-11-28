package domain

import "time"

// KiadasAllapot tells whether a Kiadas is only planned or already paid.
type KiadasAllapot string

// The states of a Kiadas.
const (
	// Tervezett is a planned Kiadas: its amount is an estimate, and it
	// doesn't count in the balances nor in the Elszamolas.
	Tervezett KiadasAllapot = "tervezett"
	// Kifizetve is a paid Kiadas with its real amount.
	Kifizetve KiadasAllapot = "kifizetve"
)

// Valid tells whether s is a known KiadasAllapot.
func (s KiadasAllapot) Valid() bool {
	return s == Tervezett || s == Kifizetve
}

// Kiadas is one shared cost in an Esemeny's Koltsegek.
type Kiadas struct {
	ID          string
	EsemenyID   string
	Description string
	// Amount is in whole HUF: the estimate while Tervezett, the real amount
	// once Kifizetve.
	Amount  int
	PayerID string
	// PayerName is filled in by reads, for showing.
	PayerName string
	Allapot   KiadasAllapot
	// Egyenlo means the split is equal: the shares are recomputed whenever
	// the amount or the split changes. Otherwise they are custom amounts.
	Egyenlo bool
	// CreatedBy recorded it; they, the payer and the Szervezo may edit or
	// delete it.
	CreatedBy string
	CreatedAt time.Time
	UpdatedAt time.Time
}

// KiadasResz is one person's share of a Kiadas, in whole HUF. A Kiadas's
// shares add up to its amount, and its holders are fixed when it is
// created (drop-outs never change them). A name-only Vendeg's (#13) share
// is charged to the Tag who added them.
type KiadasResz struct {
	KiadasID string
	Holder   ClaimHolder
	// Name is the holder's name, for showing.
	Name   string
	Amount int
}

// Elszamolas is a calculated Elszamolas of an Esemeny: the one-time
// calculation of the Utalasok that settle its kifizetve Kiadasok. While it
// exists the Kiadasok are locked.
type Elszamolas struct {
	EsemenyID    string
	KiszamolvaAt time.Time
	// LezartaID is the Szervezo who closed it without every confirmation,
	// "" when the last confirmation calculated it.
	LezartaID string
}

// Utalas is one transfer of a calculated Elszamolas: FromID pays ToID
// Amount whole HUF. It is megvolt once either of them (or the Szervezo)
// marks it done.
type Utalas struct {
	ID        string
	EsemenyID string
	FromID    string
	ToID      string
	// FromName and ToName are filled in by reads, for showing.
	FromName string
	ToName   string
	Amount   int
	// MegvoltAt is when it was marked done, nil while it isn't.
	MegvoltAt *time.Time
	// MegvoltBy marked it done, "" while it isn't.
	MegvoltBy string
	// MegvoltByName is filled in by reads, for showing.
	MegvoltByName string
}
