package httpapi

import (
	"errors"
	"net/http"
	"time"

	"squadsync-be/internal/app"
	"squadsync-be/internal/domain"
)

func (s *server) registerVendeg(mux *http.ServeMux) {
	mux.HandleFunc("POST /esemeny/{id}/meghivo", withFelhasznalo(s.createEsemenyMeghivo))
	mux.HandleFunc("GET /esemeny/{id}/meghivo", withFelhasznalo(s.listEsemenyMeghivok))
	mux.HandleFunc("DELETE /esemeny/{id}/meghivo/{meghivoID}", withFelhasznalo(s.revokeEsemenyMeghivo))
	mux.HandleFunc("POST /esemeny/{id}/vendeg", withFelhasznalo(s.addVendeg))
	mux.HandleFunc("DELETE /esemeny/{id}/vendeg/{vendegID}", withFelhasznalo(s.removeVendeg))
	mux.HandleFunc("PUT /esemeny/{id}/idopont/{idopontID}/vendeg/{vendegID}/visszajelzes", withFelhasznalo(s.setVendegVisszajelzes))

	// Routes for the holder of an Esemeny-meghivo link: the preview needs
	// no Felhasznalo, as it runs before the login.
	mux.HandleFunc("GET /esemeny-meghivo/{token}", s.previewEsemenyMeghivo)
	mux.HandleFunc("GET /esemeny-meghivo/{token}/esemeny", withFelhasznalo(s.memberEsemenyOfMeghivo))
	mux.HandleFunc("POST /esemeny-meghivo/{token}/join", withFelhasznalo(s.joinEsemeny))
}

// The wire forms below are kept in sync by hand with the types in
// squadsync-fe/src/lib/backend.ts.

type esemenyMeghivoJSON struct {
	ID        string    `json:"id"`
	Token     string    `json:"token"`
	CreatedAt time.Time `json:"createdAt"`
}

func toEsemenyMeghivoJSON(m domain.EsemenyMeghivo) esemenyMeghivoJSON {
	return esemenyMeghivoJSON{ID: m.ID, Token: m.Token, CreatedAt: m.CreatedAt}
}

type vendegJSON struct {
	ID string `json:"id"`
	// Kind is "felhasznalo" (joined through an Esemeny-meghivo) or "nev"
	// (a name added by a Tag).
	Kind          string `json:"kind"`
	FelhasznaloID string `json:"felhasznaloId"`
	Name          string `json:"name"`
	// AddedBy is the Tag who added a name-only Vendeg, "" otherwise.
	AddedBy     string    `json:"addedBy"`
	AddedByName string    `json:"addedByName"`
	CreatedAt   time.Time `json:"createdAt"`
	// CanAnswer: the caller answers and claims for them.
	CanAnswer bool `json:"canAnswer"`
	// CanRemove: the caller may remove them.
	CanRemove bool `json:"canRemove"`
}

func toVendegJSON(v app.VendegAllas) vendegJSON {
	return vendegJSON{
		ID:            v.ID,
		Kind:          string(v.Kind),
		FelhasznaloID: v.FelhasznaloID,
		Name:          v.Name,
		AddedBy:       v.AddedBy,
		AddedByName:   v.AddedByName,
		CreatedAt:     v.CreatedAt,
		CanAnswer:     v.CanAnswer,
		CanRemove:     v.CanRemove,
	}
}

func (s *server) createEsemenyMeghivo(w http.ResponseWriter, r *http.Request, felhasznaloID string) {
	m, err := s.app.CreateEsemenyMeghivo(r.Context(), felhasznaloID, r.PathValue("id"))
	if err != nil {
		s.writeVendegError(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, toEsemenyMeghivoJSON(m))
}

func (s *server) listEsemenyMeghivok(w http.ResponseWriter, r *http.Request, felhasznaloID string) {
	list, err := s.app.EsemenyMeghivok(r.Context(), felhasznaloID, r.PathValue("id"))
	if err != nil {
		s.writeVendegError(w, r, err)
		return
	}
	out := make([]esemenyMeghivoJSON, 0, len(list))
	for _, m := range list {
		out = append(out, toEsemenyMeghivoJSON(m))
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *server) revokeEsemenyMeghivo(w http.ResponseWriter, r *http.Request, felhasznaloID string) {
	if err := s.app.RevokeEsemenyMeghivo(r.Context(), felhasznaloID, r.PathValue("id"), r.PathValue("meghivoID")); err != nil {
		s.writeVendegError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *server) addVendeg(w http.ResponseWriter, r *http.Request, felhasznaloID string) {
	var body struct {
		Name string `json:"name"`
	}
	if !decodeJSON(w, r, &body) {
		return
	}
	v, err := s.app.AddVendeg(r.Context(), felhasznaloID, r.PathValue("id"), body.Name)
	if err != nil {
		s.writeVendegError(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, toVendegJSON(app.VendegAllas{Vendeg: v, CanAnswer: true, CanRemove: true}))
}

func (s *server) removeVendeg(w http.ResponseWriter, r *http.Request, felhasznaloID string) {
	if err := s.app.RemoveVendeg(r.Context(), felhasznaloID, r.PathValue("id"), r.PathValue("vendegID")); err != nil {
		s.writeVendegError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *server) setVendegVisszajelzes(w http.ResponseWriter, r *http.Request, felhasznaloID string) {
	var body struct {
		Valasz string `json:"valasz"`
	}
	if !decodeJSON(w, r, &body) {
		return
	}
	err := s.app.SetVendegVisszajelzes(r.Context(), felhasznaloID, r.PathValue("id"), r.PathValue("idopontID"), r.PathValue("vendegID"), domain.Valasz(body.Valasz))
	if err != nil {
		s.writeVendegError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *server) previewEsemenyMeghivo(w http.ResponseWriter, r *http.Request) {
	p, err := s.app.EsemenyMeghivoPreview(r.Context(), r.PathValue("token"))
	if err != nil {
		s.writeVendegError(w, r, err)
		return
	}
	// All that the holder of a link may learn before joining.
	writeJSON(w, http.StatusOK, struct {
		Title        string `json:"title"`
		SzervezoName string `json:"szervezoName"`
	}{p.Title, p.SzervezoName})
}

func (s *server) memberEsemenyOfMeghivo(w http.ResponseWriter, r *http.Request, felhasznaloID string) {
	id, err := s.app.MemberEsemenyOfMeghivo(r.Context(), felhasznaloID, r.PathValue("token"))
	if err != nil {
		s.writeVendegError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, struct {
		EsemenyID string `json:"esemenyId"`
	}{id})
}

func (s *server) joinEsemeny(w http.ResponseWriter, r *http.Request, felhasznaloID string) {
	res, err := s.app.JoinEsemeny(r.Context(), felhasznaloID, r.PathValue("token"))
	if err != nil {
		s.writeVendegError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, struct {
		EsemenyID string `json:"esemenyId"`
		Joined    bool   `json:"joined"`
	}{res.EsemenyID, res.Joined})
}

// writeVendegError maps the errors of Vendegek, then falls back to the
// Esemeny's.
func (s *server) writeVendegError(w http.ResponseWriter, r *http.Request, err error) {
	for _, c := range []struct {
		err    error
		status int
		code   string
	}{
		// A link that no longer works: 410, with the reason for the web.
		{app.ErrEsemenyMeghivoRevoked, http.StatusGone, "revoked"},
		{app.ErrEsemenyMeghivoExpired, http.StatusGone, "expired"},
		{app.ErrVendegClosed, http.StatusConflict, "vendeg_closed"},
		{app.ErrVendegHasKiadas, http.StatusConflict, "vendeg_has_kiadas"},
	} {
		if errors.Is(err, c.err) {
			writeCodedError(w, c.status, err, c.code)
			return
		}
	}
	if errors.Is(err, app.ErrFelhasznaloUnknown) {
		writeError(w, http.StatusForbidden, err.Error())
		return
	}
	s.writeEsemenyError(w, r, err)
}
