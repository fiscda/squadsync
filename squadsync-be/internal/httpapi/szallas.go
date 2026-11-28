package httpapi

import (
	"errors"
	"net/http"

	"squadsync-be/internal/app"
	"squadsync-be/internal/domain"
)

func (s *server) registerSzallas(mux *http.ServeMux) {
	mux.HandleFunc("GET /esemeny/{id}/szallas", withFelhasznalo(s.getSzallas))
	mux.HandleFunc("PUT /esemeny/{id}/szallas", withFelhasznalo(s.setSzallas))
	mux.HandleFunc("POST /esemeny/{id}/szallas/szoba", withFelhasznalo(s.addSzoba))
	mux.HandleFunc("PUT /esemeny/{id}/szallas/szoba/{szobaID}", withFelhasznalo(s.renameSzoba))
	mux.HandleFunc("DELETE /esemeny/{id}/szallas/szoba/{szobaID}", withFelhasznalo(s.deleteSzoba))
	mux.HandleFunc("POST /esemeny/{id}/szallas/szoba/{szobaID}/agy", withFelhasznalo(s.addAgy))
	mux.HandleFunc("PUT /esemeny/{id}/szallas/agy/{agyID}", withFelhasznalo(s.updateAgy))
	mux.HandleFunc("DELETE /esemeny/{id}/szallas/agy/{agyID}", withFelhasznalo(s.deleteAgy))
	mux.HandleFunc("POST /esemeny/{id}/szallas/agy/{agyID}/foglalas", withFelhasznalo(s.claimAgy))
	mux.HandleFunc("PUT /esemeny/{id}/szallas/agy/{agyID}/alvo", withFelhasznalo(s.moveAlvo))
	mux.HandleFunc("DELETE /esemeny/{id}/szallas/foglalas", withFelhasznalo(s.releaseFoglalas))
	mux.HandleFunc("PUT /esemeny/{id}/szallas/mashol-alszik", withFelhasznalo(s.setMasholAlszik))
}

// The wire forms below are kept in sync by hand with
// squadsync-fe/src/lib/kiegeszito/szallas.ts.

type szemelyJSON struct {
	FelhasznaloID string `json:"felhasznaloId"`
	// VendegID is a name-only Vendeg's id (#13), "" otherwise.
	VendegID string `json:"vendegId"`
	Name     string `json:"name"`
}

type agyJSON struct {
	ID       string `json:"id"`
	SzobaID  string `json:"szobaId"`
	Capacity int    `json:"capacity"`
	Label    string `json:"label"`
}

type agyAllasJSON struct {
	agyJSON
	Alvok  []szemelyJSON `json:"alvok"`
	Szabad int           `json:"szabad"`
}

type szobaJSON struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type szobaAllasJSON struct {
	szobaJSON
	Agyak []agyAllasJSON `json:"agyak"`
}

type szallasJSON struct {
	Note              string           `json:"note"`
	Szobak            []szobaAllasJSON `json:"szobak"`
	MasholAlszik      []szemelyJSON    `json:"masholAlszik"`
	AgyNelkul         []szemelyJSON    `json:"agyNelkul"`
	CanSetup          bool             `json:"canSetup"`
	CanClaim          bool             `json:"canClaim"`
	SajatAgyID        string           `json:"sajatAgyId"`
	SajatMasholAlszik bool             `json:"sajatMasholAlszik"`
	IsSzervezo        bool             `json:"isSzervezo"`
}

func toAgyJSON(a domain.Agy) agyJSON {
	return agyJSON{ID: a.ID, SzobaID: a.SzobaID, Capacity: a.Capacity, Label: a.Label}
}

func (s *server) getSzallas(w http.ResponseWriter, r *http.Request, felhasznaloID string) {
	z, err := s.app.Szallas(r.Context(), felhasznaloID, r.PathValue("id"))
	if err != nil {
		s.writeSzallasError(w, r, err)
		return
	}
	out := szallasJSON{
		Note:              z.Note,
		Szobak:            make([]szobaAllasJSON, 0, len(z.Szobak)),
		MasholAlszik:      make([]szemelyJSON, 0, len(z.MasholAlszik)),
		AgyNelkul:         make([]szemelyJSON, 0, len(z.AgyNelkul)),
		CanSetup:          z.CanSetup,
		CanClaim:          z.CanClaim,
		SajatAgyID:        z.SajatAgyID,
		SajatMasholAlszik: z.SajatMasholAlszik,
		IsSzervezo:        z.IsSzervezo,
	}
	for _, szoba := range z.Szobak {
		sz := szobaAllasJSON{szobaJSON: szobaJSON{ID: szoba.ID, Name: szoba.Name}, Agyak: make([]agyAllasJSON, 0, len(szoba.Agyak))}
		for _, agy := range szoba.Agyak {
			alvok := make([]szemelyJSON, 0, len(agy.Alvok))
			for _, g := range agy.Alvok {
				alvok = append(alvok, szemelyJSON{FelhasznaloID: g.Holder.FelhasznaloID, VendegID: g.Holder.VendegID, Name: g.Name})
			}
			sz.Agyak = append(sz.Agyak, agyAllasJSON{agyJSON: toAgyJSON(agy.Agy), Alvok: alvok, Szabad: agy.Szabad})
		}
		out.Szobak = append(out.Szobak, sz)
	}
	for _, m := range z.MasholAlszik {
		out.MasholAlszik = append(out.MasholAlszik, szemelyJSON{FelhasznaloID: m.Holder.FelhasznaloID, VendegID: m.Holder.VendegID, Name: m.Name})
	}
	for _, r := range z.AgyNelkul {
		out.AgyNelkul = append(out.AgyNelkul, szemelyJSON{FelhasznaloID: r.FelhasznaloID, Name: r.Name})
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *server) setSzallas(w http.ResponseWriter, r *http.Request, felhasznaloID string) {
	var body struct {
		Note string `json:"note"`
	}
	if !decodeJSON(w, r, &body) {
		return
	}
	if err := s.app.SetSzallas(r.Context(), felhasznaloID, r.PathValue("id"), app.SzallasInput{Note: body.Note}); err != nil {
		s.writeSzallasError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

type szobaBody struct {
	Name string `json:"name"`
}

func (s *server) addSzoba(w http.ResponseWriter, r *http.Request, felhasznaloID string) {
	var body szobaBody
	if !decodeJSON(w, r, &body) {
		return
	}
	szoba, err := s.app.AddSzoba(r.Context(), felhasznaloID, r.PathValue("id"), body.Name)
	if err != nil {
		s.writeSzallasError(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, szobaJSON{ID: szoba.ID, Name: szoba.Name})
}

func (s *server) renameSzoba(w http.ResponseWriter, r *http.Request, felhasznaloID string) {
	var body szobaBody
	if !decodeJSON(w, r, &body) {
		return
	}
	if err := s.app.RenameSzoba(r.Context(), felhasznaloID, r.PathValue("id"), r.PathValue("szobaID"), body.Name); err != nil {
		s.writeSzallasError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *server) deleteSzoba(w http.ResponseWriter, r *http.Request, felhasznaloID string) {
	if err := s.app.DeleteSzoba(r.Context(), felhasznaloID, r.PathValue("id"), r.PathValue("szobaID")); err != nil {
		s.writeSzallasError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

type agyBody struct {
	Capacity int    `json:"capacity"`
	Label    string `json:"label"`
}

func (s *server) addAgy(w http.ResponseWriter, r *http.Request, felhasznaloID string) {
	var body agyBody
	if !decodeJSON(w, r, &body) {
		return
	}
	agy, err := s.app.AddAgy(r.Context(), felhasznaloID, r.PathValue("id"), r.PathValue("szobaID"), app.AgyInput{Capacity: body.Capacity, Label: body.Label})
	if err != nil {
		s.writeSzallasError(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, toAgyJSON(agy))
}

func (s *server) updateAgy(w http.ResponseWriter, r *http.Request, felhasznaloID string) {
	var body agyBody
	if !decodeJSON(w, r, &body) {
		return
	}
	err := s.app.UpdateAgy(r.Context(), felhasznaloID, r.PathValue("id"), r.PathValue("agyID"), app.AgyInput{Capacity: body.Capacity, Label: body.Label})
	if err != nil {
		s.writeSzallasError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *server) deleteAgy(w http.ResponseWriter, r *http.Request, felhasznaloID string) {
	if err := s.app.DeleteAgy(r.Context(), felhasznaloID, r.PathValue("id"), r.PathValue("agyID")); err != nil {
		s.writeSzallasError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// foglalasBody names whose spot a request is about; both empty means the
// caller.
type foglalasBody struct {
	FelhasznaloID string `json:"felhasznaloId"`
	// VendegID is the caller's name-only Vendeg (#13).
	VendegID string `json:"vendegId"`
}

func (b foglalasBody) input() app.FoglalasInput {
	return app.FoglalasInput{FelhasznaloID: b.FelhasznaloID, VendegID: b.VendegID}
}

func (s *server) claimAgy(w http.ResponseWriter, r *http.Request, felhasznaloID string) {
	var body foglalasBody
	if !decodeJSON(w, r, &body) {
		return
	}
	if err := s.app.ClaimAgy(r.Context(), felhasznaloID, r.PathValue("id"), r.PathValue("agyID"), body.input()); err != nil {
		s.writeSzallasError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *server) moveAlvo(w http.ResponseWriter, r *http.Request, felhasznaloID string) {
	var body foglalasBody
	if !decodeJSON(w, r, &body) {
		return
	}
	if err := s.app.MoveAlvo(r.Context(), felhasznaloID, r.PathValue("id"), r.PathValue("agyID"), body.input()); err != nil {
		s.writeSzallasError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// releaseFoglalas takes a spot away: the caller's, or, with
// ?felhasznaloId= (the Szervezo only) or ?vendegId= (#13), someone else's.
func (s *server) releaseFoglalas(w http.ResponseWriter, r *http.Request, felhasznaloID string) {
	in := app.FoglalasInput{FelhasznaloID: r.URL.Query().Get("felhasznaloId"), VendegID: r.URL.Query().Get("vendegId")}
	if err := s.app.ReleaseFoglalas(r.Context(), felhasznaloID, r.PathValue("id"), in); err != nil {
		s.writeSzallasError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *server) setMasholAlszik(w http.ResponseWriter, r *http.Request, felhasznaloID string) {
	var body struct {
		On bool `json:"on"`
		// VendegID is the caller's name-only Vendeg (#13).
		VendegID string `json:"vendegId"`
	}
	if !decodeJSON(w, r, &body) {
		return
	}
	err := s.app.SetMasholAlszik(r.Context(), felhasznaloID, r.PathValue("id"), app.MasholAlszikInput{On: body.On, VendegID: body.VendegID})
	if err != nil {
		s.writeSzallasError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *server) writeSzallasError(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, app.ErrAgyFull):
		writeCodedError(w, http.StatusConflict, err, "szallas_agy_full")
	case errors.Is(err, app.ErrAlreadyInAgy):
		writeCodedError(w, http.StatusConflict, err, "szallas_already_in_agy")
	case errors.Is(err, app.ErrAgyCapacityBelowAlvok):
		writeCodedError(w, http.StatusConflict, err, "szallas_capacity_below_alvok")
	default:
		s.writeKiegeszitoError(w, r, err)
	}
}
