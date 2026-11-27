package app

import "slices"

// Helyezes is a place in a ranking: 1 is first. Zero means the entry has no
// place, because nobody has answered it.
type Helyezes int

// Pontszam is what an entry is ranked by. Higher is better, compared element
// by element: the first element decides, the next breaks ties, and so on. A
// nil Pontszam means nobody has answered the entry, so it gets no Helyezes.
//
// An Idopont scores {igen, talan, -nem}; a Javaslat would score {backers}.
type Pontszam []int

// Helyezesek ranks entries by their Pontszam and returns each entry's
// Helyezes, in the same order as the input. Entries with equal Pontszam share
// a place, and the next place skips accordingly (1., 1., 3.). Entries with a
// nil Pontszam get 0 and don't take up a place.
func Helyezesek(pontszamok []Pontszam) []Helyezes {
	out := make([]Helyezes, len(pontszamok))
	for i, p := range pontszamok {
		if p == nil {
			continue
		}
		better := 0
		for _, q := range pontszamok {
			if q != nil && slices.Compare(q, p) > 0 {
				better++
			}
		}
		out[i] = Helyezes(better + 1)
	}
	return out
}
