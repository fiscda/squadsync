package domain

import "time"

// KiadasAllapot tells whether a Kiadas is only planned or already paid.
type KiadasAllapot string

// The states of a Kiadas.
const (
	// Tervezett is a planned Kiadas: its amount is an estimate, and it
	// doesn't count in the balances (nor, #22, in the Elszamolas).
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
