package httpapi

import (
	"errors"
	"net/http"
	"time"

	"squadsync-be/internal/app"
	"squadsync-be/internal/domain"
)

func (s *server) registerFelszereles(mux *http.ServeMux) {
	mux.HandleFunc("GET /esemeny/{id}/felszereles", withFelhasznalo(s.getFelszereles))
	mux.HandleFunc("POST /esemeny/{id}/felszereles/tetel", withFelhasznalo(s.addTetel))
	mux.HandleFunc("PUT /esemeny/{id}/felszereles/tetel/{tetelID}", withFelhasznalo(s.updateTetel))
	mux.HandleFunc("DELETE /esemeny/{id}/felszereles/tetel/{tetelID}", withFelhasznalo(s.deleteTetel))
	mux.HandleFunc("PUT /esemeny/{id}/felszereles/tetel/{tetelID}/vallalas", withFelhasznalo(s.setVallalas))
}

// The wire forms below are kept in sync by hand with
// squadsync-fe/src/lib/kiegeszito/felszereles.ts.

type tetelJSON struct {
	ID            string    `json:"id"`
	Name          string    `json:"name"`
	Quantity      int       `json:"quantity"`
	MindenkiHozza bool      `json:"mindenkiHozza"`
	AddedBy       string    `json:"addedBy"`
	CreatedAt     time.Time `json:"createdAt"`
}

type vallalasJSON struct {
	FelhasznaloID string `json:"felhasznaloId"`
	// VendegID is a name-only Vendeg's id (#13), "" otherwise.
	VendegID string `json:"vendegId"`
	Name     string `json:"name"`
	Quantity int    `json:"quantity"`
}

type tetelAllasJSON struct {
	tetelJSON
	Vallalt    int            `json:"vallalt"`
	Hianyzik   int            `json:"hianyzik"`
	Vallalasok []vallalasJSON `json:"vallalasok"`
	Sajat      int            `json:"sajat"`
	CanEdit    bool           `json:"canEdit"`
}

type felszerelesJSON struct {
	Tetelek     []tetelAllasJSON `json:"tetelek"`
	Hianyzik    int              `json:"hianyzik"`
	Emlekezteto []string         `json:"emlekezteto"`
	CanAdd      bool             `json:"canAdd"`
	CanClaim    bool             `json:"canClaim"`
}

type tetelInputJSON struct {
	Name string `json:"name"`
	// Quantity 0 (or missing) means 1.
	Quantity      int  `json:"quantity"`
	MindenkiHozza bool `json:"mindenkiHozza"`
}

func (in tetelInputJSON) input() app.TetelInput {
	return app.TetelInput{Name: in.Name, Quantity: in.Quantity, MindenkiHozza: in.MindenkiHozza}
}

func toTetelJSON(t domain.Tetel) tetelJSON {
	return tetelJSON{ID: t.ID, Name: t.Name, Quantity: t.Quantity, MindenkiHozza: t.MindenkiHozza, AddedBy: t.AddedBy, CreatedAt: t.CreatedAt}
}

func (s *server) getFelszereles(w http.ResponseWriter, r *http.Request, felhasznaloID string) {
	f, err := s.app.Felszereles(r.Context(), felhasznaloID, r.PathValue("id"))
	if err != nil {
		s.writeFelszerelesError(w, r, err)
		return
	}
	out := felszerelesJSON{
		Tetelek:     make([]tetelAllasJSON, 0, len(f.Tetelek)),
		Hianyzik:    f.Hianyzik,
		Emlekezteto: f.Emlekezteto,
		CanAdd:      f.CanAdd,
		CanClaim:    f.CanClaim,
	}
	for _, t := range f.Tetelek {
		allas := tetelAllasJSON{
			tetelJSON:  toTetelJSON(t.Tetel),
			Vallalt:    t.Vallalt,
			Hianyzik:   t.Hianyzik,
			Vallalasok: make([]vallalasJSON, 0, len(t.Vallalasok)),
			Sajat:      t.Sajat,
			CanEdit:    t.CanEdit,
		}
		for _, v := range t.Vallalasok {
			allas.Vallalasok = append(allas.Vallalasok, vallalasJSON{
				FelhasznaloID: v.Holder.FelhasznaloID, VendegID: v.Holder.VendegID, Name: v.Name, Quantity: v.Quantity,
			})
		}
		out.Tetelek = append(out.Tetelek, allas)
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *server) addTetel(w http.ResponseWriter, r *http.Request, felhasznaloID string) {
	var body tetelInputJSON
	if !decodeJSON(w, r, &body) {
		return
	}
	t, err := s.app.AddTetel(r.Context(), felhasznaloID, r.PathValue("id"), body.input())
	if err != nil {
		s.writeFelszerelesError(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, toTetelJSON(t))
}

func (s *server) updateTetel(w http.ResponseWriter, r *http.Request, felhasznaloID string) {
	var body tetelInputJSON
	if !decodeJSON(w, r, &body) {
		return
	}
	if err := s.app.UpdateTetel(r.Context(), felhasznaloID, r.PathValue("id"), r.PathValue("tetelID"), body.input()); err != nil {
		s.writeFelszerelesError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *server) deleteTetel(w http.ResponseWriter, r *http.Request, felhasznaloID string) {
	if err := s.app.DeleteTetel(r.Context(), felhasznaloID, r.PathValue("id"), r.PathValue("tetelID")); err != nil {
		s.writeFelszerelesError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *server) setVallalas(w http.ResponseWriter, r *http.Request, felhasznaloID string) {
	var body struct {
		// Quantity 0 removes the claim.
		Quantity int `json:"quantity"`
		// VendegID claims for the caller's name-only Vendeg (#13).
		VendegID string `json:"vendegId"`
	}
	if !decodeJSON(w, r, &body) {
		return
	}
	err := s.app.SetVallalas(r.Context(), felhasznaloID, r.PathValue("id"), r.PathValue("tetelID"),
		app.VallalasInput{Quantity: body.Quantity, VendegID: body.VendegID})
	if err != nil {
		s.writeFelszerelesError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *server) writeFelszerelesError(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, app.ErrTetelMindenkiHozza):
		writeCodedError(w, http.StatusConflict, err, "mindenki_hozza")
	case errors.Is(err, app.ErrVallalasTooMuch):
		writeCodedError(w, http.StatusConflict, err, "too_much")
	case errors.Is(err, app.ErrTetelClaimed):
		writeCodedError(w, http.StatusConflict, err, "tetel_claimed")
	default:
		s.writeKiegeszitoError(w, r, err)
	}
}
