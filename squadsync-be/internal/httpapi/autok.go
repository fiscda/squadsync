package httpapi

import (
	"errors"
	"net/http"
	"time"

	"squadsync-be/internal/app"
	"squadsync-be/internal/domain"
)

func (s *server) registerAutok(mux *http.ServeMux) {
	mux.HandleFunc("GET /esemeny/{id}/autok", withFelhasznalo(s.getAutok))
	mux.HandleFunc("POST /esemeny/{id}/autok/auto", withFelhasznalo(s.offerAuto))
	mux.HandleFunc("PUT /esemeny/{id}/autok/auto/{autoID}", withFelhasznalo(s.updateAuto))
	mux.HandleFunc("DELETE /esemeny/{id}/autok/auto/{autoID}", withFelhasznalo(s.deleteAuto))
	mux.HandleFunc("GET /esemeny/{id}/autok/auto/{autoID}/utasai", withFelhasznalo(s.getAutoUtasai))
	mux.HandleFunc("POST /esemeny/{id}/autok/auto/{autoID}/ules", withFelhasznalo(s.claimUles))
	mux.HandleFunc("PUT /esemeny/{id}/autok/auto/{autoID}/utas", withFelhasznalo(s.moveUtas))
	mux.HandleFunc("DELETE /esemeny/{id}/autok/ules", withFelhasznalo(s.releaseUles))
	mux.HandleFunc("PUT /esemeny/{id}/autok/sajat-uton", withFelhasznalo(s.setSajatUton))
}

// The wire forms below are kept in sync by hand with
// squadsync-fe/src/lib/kiegeszito/autok.ts.

type autoJSON struct {
	ID        string    `json:"id"`
	SoforID   string    `json:"soforId"`
	SoforName string    `json:"soforName"`
	Seats     int       `json:"seats"`
	Note      string    `json:"note"`
	CreatedAt time.Time `json:"createdAt"`
}

type utasJSON struct {
	FelhasznaloID string `json:"felhasznaloId"`
	// VendegID is a name-only Vendeg's id (#13), "" otherwise.
	VendegID string `json:"vendegId"`
	Name     string `json:"name"`
}

type autoAllasJSON struct {
	autoJSON
	Utasok    []utasJSON `json:"utasok"`
	Szabad    int        `json:"szabad"`
	CanEdit   bool       `json:"canEdit"`
	CanDelete bool       `json:"canDelete"`
}

type autokJSON struct {
	Autok           []autoAllasJSON `json:"autok"`
	SajatUton       []utasJSON      `json:"sajatUton"`
	HelyNelkul      []utasJSON      `json:"helyNelkul"`
	CanOffer        bool            `json:"canOffer"`
	CanClaim        bool            `json:"canClaim"`
	SajatAutoID     string          `json:"sajatAutoId"`
	SajatUlesAutoID string          `json:"sajatUlesAutoId"`
	SajatUtonVan    bool            `json:"sajatUtonVan"`
	IsSzervezo      bool            `json:"isSzervezo"`
}

type autoUtasaiJSON struct {
	Auto   autoJSON   `json:"auto"`
	Utasok []utasJSON `json:"utasok"`
}

func toAutoJSON(a domain.Auto) autoJSON {
	return autoJSON{ID: a.ID, SoforID: a.SoforID, SoforName: a.SoforName, Seats: a.Seats, Note: a.Note, CreatedAt: a.CreatedAt}
}

func toUtasokJSON(utasok []domain.Utas) []utasJSON {
	out := make([]utasJSON, 0, len(utasok))
	for _, u := range utasok {
		out = append(out, utasJSON{FelhasznaloID: u.Holder.FelhasznaloID, VendegID: u.Holder.VendegID, Name: u.Name})
	}
	return out
}

func (s *server) getAutok(w http.ResponseWriter, r *http.Request, felhasznaloID string) {
	a, err := s.app.Autok(r.Context(), felhasznaloID, r.PathValue("id"))
	if err != nil {
		s.writeAutokError(w, r, err)
		return
	}
	out := autokJSON{
		Autok:           make([]autoAllasJSON, 0, len(a.Autok)),
		SajatUton:       make([]utasJSON, 0, len(a.SajatUton)),
		HelyNelkul:      make([]utasJSON, 0, len(a.HelyNelkul)),
		CanOffer:        a.CanOffer,
		CanClaim:        a.CanClaim,
		SajatAutoID:     a.SajatAutoID,
		SajatUlesAutoID: a.SajatUlesAutoID,
		SajatUtonVan:    a.SajatUtonVan,
		IsSzervezo:      a.IsSzervezo,
	}
	for _, auto := range a.Autok {
		out.Autok = append(out.Autok, autoAllasJSON{
			autoJSON:  toAutoJSON(auto.Auto),
			Utasok:    toUtasokJSON(auto.Utasok),
			Szabad:    auto.Szabad,
			CanEdit:   auto.CanEdit,
			CanDelete: auto.CanDelete,
		})
	}
	for _, su := range a.SajatUton {
		out.SajatUton = append(out.SajatUton, utasJSON{FelhasznaloID: su.Holder.FelhasznaloID, VendegID: su.Holder.VendegID, Name: su.Name})
	}
	for _, r := range a.HelyNelkul {
		out.HelyNelkul = append(out.HelyNelkul, utasJSON{FelhasznaloID: r.FelhasznaloID, Name: r.Name})
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *server) getAutoUtasai(w http.ResponseWriter, r *http.Request, felhasznaloID string) {
	u, err := s.app.AutoUtasai(r.Context(), felhasznaloID, r.PathValue("id"), r.PathValue("autoID"))
	if err != nil {
		s.writeAutokError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, autoUtasaiJSON{Auto: toAutoJSON(u.Auto), Utasok: toUtasokJSON(u.Utasok)})
}

type autoInputJSON struct {
	Seats int    `json:"seats"`
	Note  string `json:"note"`
}

func (s *server) offerAuto(w http.ResponseWriter, r *http.Request, felhasznaloID string) {
	var body autoInputJSON
	if !decodeJSON(w, r, &body) {
		return
	}
	auto, err := s.app.OfferAuto(r.Context(), felhasznaloID, r.PathValue("id"), app.AutoInput{Seats: body.Seats, Note: body.Note})
	if err != nil {
		s.writeAutokError(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, toAutoJSON(auto))
}

func (s *server) updateAuto(w http.ResponseWriter, r *http.Request, felhasznaloID string) {
	var body autoInputJSON
	if !decodeJSON(w, r, &body) {
		return
	}
	err := s.app.UpdateAuto(r.Context(), felhasznaloID, r.PathValue("id"), r.PathValue("autoID"), app.AutoInput{Seats: body.Seats, Note: body.Note})
	if err != nil {
		s.writeAutokError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *server) deleteAuto(w http.ResponseWriter, r *http.Request, felhasznaloID string) {
	if err := s.app.DeleteAuto(r.Context(), felhasznaloID, r.PathValue("id"), r.PathValue("autoID")); err != nil {
		s.writeAutokError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// utasBody names whose seat a request is about; both empty means the caller.
type utasBody struct {
	FelhasznaloID string `json:"felhasznaloId"`
	// VendegID is the caller's name-only Vendeg (#13).
	VendegID string `json:"vendegId"`
}

func (b utasBody) input() app.UtasInput {
	return app.UtasInput{FelhasznaloID: b.FelhasznaloID, VendegID: b.VendegID}
}

func (s *server) claimUles(w http.ResponseWriter, r *http.Request, felhasznaloID string) {
	var body utasBody
	if !decodeJSON(w, r, &body) {
		return
	}
	if err := s.app.ClaimUles(r.Context(), felhasznaloID, r.PathValue("id"), r.PathValue("autoID"), body.input()); err != nil {
		s.writeAutokError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *server) moveUtas(w http.ResponseWriter, r *http.Request, felhasznaloID string) {
	var body utasBody
	if !decodeJSON(w, r, &body) {
		return
	}
	if err := s.app.MoveUtas(r.Context(), felhasznaloID, r.PathValue("id"), r.PathValue("autoID"), body.input()); err != nil {
		s.writeAutokError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// releaseUles takes a seat away: the caller's, or, with ?felhasznaloId= (the
// Szervezo only) or ?vendegId= (#13), someone else's.
func (s *server) releaseUles(w http.ResponseWriter, r *http.Request, felhasznaloID string) {
	in := app.UtasInput{FelhasznaloID: r.URL.Query().Get("felhasznaloId"), VendegID: r.URL.Query().Get("vendegId")}
	if err := s.app.ReleaseUles(r.Context(), felhasznaloID, r.PathValue("id"), in); err != nil {
		s.writeAutokError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *server) setSajatUton(w http.ResponseWriter, r *http.Request, felhasznaloID string) {
	var body struct {
		On bool `json:"on"`
		// VendegID is the caller's name-only Vendeg (#13).
		VendegID string `json:"vendegId"`
	}
	if !decodeJSON(w, r, &body) {
		return
	}
	err := s.app.SetSajatUton(r.Context(), felhasznaloID, r.PathValue("id"), app.SajatUtonInput{On: body.On, VendegID: body.VendegID})
	if err != nil {
		s.writeAutokError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *server) writeAutokError(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, app.ErrAutoFull):
		writeCodedError(w, http.StatusConflict, err, "autok_full")
	case errors.Is(err, app.ErrAlreadySofor):
		writeCodedError(w, http.StatusConflict, err, "autok_already_sofor")
	case errors.Is(err, app.ErrSoforCantRide):
		writeCodedError(w, http.StatusConflict, err, "autok_sofor_cant_ride")
	case errors.Is(err, app.ErrAlreadySeated):
		writeCodedError(w, http.StatusConflict, err, "autok_already_seated")
	case errors.Is(err, app.ErrAutoSeatsBelowUtasok):
		writeCodedError(w, http.StatusConflict, err, "autok_seats_below_utasok")
	default:
		s.writeKiegeszitoError(w, r, err)
	}
}
