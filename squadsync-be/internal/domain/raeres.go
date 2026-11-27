package domain

// RaeresAllapot is what a Felhasznalo says about one day. A day they haven't
// marked is unknown, which is "" and is never stored.
type RaeresAllapot string

// The marks of a Raeres day.
const (
	Szabad  RaeresAllapot = "szabad"
	Foglalt RaeresAllapot = "foglalt"
)

// Valid tells whether the mark is szabad or foglalt.
func (a RaeresAllapot) Valid() bool { return a == Szabad || a == Foglalt }

// RaeresDay is one marked day of a Felhasznalo's Raeres. Day is a Budapest
// date like "2026-12-05".
type RaeresDay struct {
	Day     string
	Allapot RaeresAllapot
}
