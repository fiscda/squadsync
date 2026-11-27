package app

import "squadsync-be/internal/domain"

// Resztvevok are the people with igen on the confirmed Idopont. This file
// is the one place that decides who they are; every rule that depends on
// them (the drop-out DM here, claims in Kiegeszitok in #16, the headcount
// with Vendegs in #13, calendar events in #24) goes through it.

// hasResztvevok tells whether an Esemeny in this state has Resztvevok: only
// once an Idopont is confirmed. Lezajlott keeps them, because the
// Elszamolas runs among the Resztvevok after the Esemeny took place.
// Egyeztetes and Elmaradt have none.
func hasResztvevok(allapot domain.Allapot) bool {
	return allapot == domain.Veglegesitve || allapot == domain.Lezajlott
}

// makesResztvevo tells whether answering valasz to the Idopont makes
// someone who counts (see countsAsResztvevo) a Resztvevo of the Esemeny.
// Comparing it before and after a change of Visszajelzes tells a drop-out
// (true, then false) from someone joining (false, then true).
func makesResztvevo(e domain.Esemeny, i domain.Idopont, valasz domain.Valasz) bool {
	return hasResztvevok(e.Allapot) && i.ID == e.ConfirmedIdopontID && valasz == domain.Igen
}

// countsAsResztvevo tells whether the giver of a Visszajelzes can be a
// Resztvevo at all: a Tag of the Esemeny's Csoport, not someone who has
// left it. #13 adds Vendegs here (Felhasznalok and name-only ones).
func countsAsResztvevo(v domain.VisszajelzesOfEsemeny) bool {
	return v.IsTag
}

// resztvevokOf picks the Resztvevok out of the Esemeny's Visszajelzesek
// (as ListVisszajelzesekOfEsemeny returns them, ordered by name). It never
// returns nil. A use case that needs them inside its transaction (claims in
// #16) reads q.ListVisszajelzesekOfEsemeny and passes it here.
func resztvevokOf(e domain.Esemeny, visszajelzesek []domain.VisszajelzesOfEsemeny) []domain.Resztvevo {
	out := []domain.Resztvevo{}
	if !hasResztvevok(e.Allapot) {
		return out
	}
	for _, v := range visszajelzesek {
		if v.IdopontID != e.ConfirmedIdopontID || v.Valasz != domain.Igen || !countsAsResztvevo(v) {
			continue
		}
		out = append(out, domain.Resztvevo{
			FelhasznaloID: v.FelhasznaloID,
			Name:          v.FelhasznaloName,
			IsTag:         v.IsTag,
		})
	}
	return out
}
