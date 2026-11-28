package httpapi

import (
	"errors"
	"net/http"
	"time"

	"squadsync-be/internal/app"
	"squadsync-be/internal/domain"
)

func (s *server) registerEsemeny(mux *http.ServeMux) {
	mux.HandleFunc("POST /csoport/{id}/esemeny", withFelhasznalo(s.proposeEsemeny))
	mux.HandleFunc("GET /csoport/{id}/esemeny", withFelhasznalo(s.listEsemenyek))
	mux.HandleFunc("GET /esemeny/{id}", withFelhasznalo(s.getEsemeny))
	mux.HandleFunc("PUT /esemeny/{id}", withFelhasznalo(s.editEsemeny))
	mux.HandleFunc("POST /esemeny/{id}/masolas", withFelhasznalo(s.copyEsemeny))
	mux.HandleFunc("POST /esemeny/{id}/idopont", withFelhasznalo(s.addIdopont))
	mux.HandleFunc("DELETE /esemeny/{id}/idopont/{idopontID}", withFelhasznalo(s.removeIdopont))
	mux.HandleFunc("PUT /esemeny/{id}/idopont/{idopontID}/visszajelzes", withFelhasznalo(s.setVisszajelzes))
	mux.HandleFunc("POST /esemeny/{id}/veglegesites", withFelhasznalo(s.confirmIdopont))
	mux.HandleFunc("POST /esemeny/{id}/ujranyitas", withFelhasznalo(s.reopenEgyeztetes))
	mux.HandleFunc("POST /esemeny/{id}/lemondas", withFelhasznalo(s.cancelEsemeny))
}

// The wire forms below are kept in sync by hand with the types in
// squadsync-fe/src/lib/backend.ts.

type helyszinJSON struct {
	Name string `json:"name"`
	URL  string `json:"url"`
}

type esemenyJSON struct {
	ID          string       `json:"id"`
	CsoportID   string       `json:"csoportId"`
	Title       string       `json:"title"`
	Description string       `json:"description"`
	Helyszin    helyszinJSON `json:"helyszin"`
	SzervezoID  string       `json:"szervezoId"`
	Allapot     string       `json:"allapot"`
	CreatedAt   time.Time    `json:"createdAt"`
}

type esemenySummaryJSON struct {
	esemenyJSON
	IdopontCount int        `json:"idopontCount"`
	FirstStart   *time.Time `json:"firstStart"`
}

type visszajelzesJSON struct {
	// FelhasznaloID is "" for a name-only Vendeg, whose id is VendegID.
	FelhasznaloID string `json:"felhasznaloId"`
	VendegID      string `json:"vendegId"`
	Name          string `json:"name"`
	Valasz        string `json:"valasz"`
	IsTag         bool   `json:"isTag"`
	// IsVendeg: a Vendeg of the Esemeny (shown as "vendég" unless IsTag).
	IsVendeg bool `json:"isVendeg"`
	// VendegAddedBy is the Tag who answers for a name-only Vendeg.
	VendegAddedBy string    `json:"vendegAddedBy"`
	UpdatedAt     time.Time `json:"updatedAt"`
}

type idopontJSON struct {
	ID    string     `json:"id"`
	Start time.Time  `json:"start"`
	End   *time.Time `json:"end"`
	// EgeszNapos: Start and End are Budapest midnights; show dates only.
	EgeszNapos bool   `json:"egeszNapos"`
	AddedBy    string `json:"addedBy"`
	Igen       int    `json:"igen"`
	Talan      int    `json:"talan"`
	Nem        int    `json:"nem"`
	// VendegIgen, VendegTalan and VendegNem count the Vendegek, on top of
	// igen, talan and nem (the Tags'), and never toward the Helyezes.
	VendegIgen  int `json:"vendegIgen"`
	VendegTalan int `json:"vendegTalan"`
	VendegNem   int `json:"vendegNem"`
	// Helyezes is null when no Tag has answered.
	Helyezes *int `json:"helyezes"`
	// Sajat is the caller's own answer, null if none.
	Sajat          *string            `json:"sajat"`
	Visszajelzesek []visszajelzesJSON `json:"visszajelzesek"`
	// Answerable: it takes Visszajelzesek now.
	Answerable bool `json:"answerable"`
	// Confirmed: the Esemeny's confirmed Idopont (Veglegesitve or Lezajlott).
	Confirmed bool `json:"confirmed"`
	// Removable: the caller may remove it now.
	Removable bool `json:"removable"`
}

type resztvevoJSON struct {
	// FelhasznaloID is "" for a name-only Vendeg, whose id is VendegID.
	FelhasznaloID string `json:"felhasznaloId"`
	VendegID      string `json:"vendegId"`
	Name          string `json:"name"`
	IsTag         bool   `json:"isTag"`
	IsVendeg      bool   `json:"isVendeg"`
	// AddedBy is the Tag of a name-only Vendeg, "" otherwise.
	AddedBy string `json:"addedBy"`
}

type esemenyDetailJSON struct {
	esemenyJSON
	CsoportName  string        `json:"csoportName"`
	SzervezoName string        `json:"szervezoName"`
	Idopontok    []idopontJSON `json:"idopontok"`
	// ConfirmedIdopontID is set only while the Esemeny has Resztvevok
	// (Veglegesitve, Lezajlott), null otherwise.
	ConfirmedIdopontID *string `json:"confirmedIdopontId"`
	// Resztvevok are empty unless the Esemeny is Veglegesitve or Lezajlott.
	Resztvevok []resztvevoJSON `json:"resztvevok"`
	// Kiegeszitok are every Kiegeszito that exists, in display order.
	Kiegeszitok []kiegeszitoJSON `json:"kiegeszitok"`
	// Hatarido is the voting deadline, null for none.
	Hatarido *time.Time `json:"hatarido"`
	// ViewerIsVendeg: the caller is a Vendeg, who sees the Esemeny without
	// its Csoport (csoportName is "").
	ViewerIsVendeg bool `json:"viewerIsVendeg"`
	// Vendegek are the Esemeny's Vendegek, ordered by name.
	Vendegek []vendegJSON `json:"vendegek"`
}

func toEsemenyJSON(e domain.Esemeny) esemenyJSON {
	return esemenyJSON{
		ID:          e.ID,
		CsoportID:   e.CsoportID,
		Title:       e.Title,
		Description: e.Description,
		Helyszin:    helyszinJSON{Name: e.Helyszin.Name, URL: e.Helyszin.URL},
		SzervezoID:  e.SzervezoID,
		Allapot:     string(e.Allapot),
		CreatedAt:   e.CreatedAt,
	}
}

func toIdopontJSON(a app.IdopontAllas) idopontJSON {
	out := idopontJSON{
		ID:             a.ID,
		Start:          a.Start,
		End:            a.End,
		EgeszNapos:     a.EgeszNapos,
		AddedBy:        a.AddedBy,
		Igen:           a.Igen,
		Talan:          a.Talan,
		Nem:            a.Nem,
		VendegIgen:     a.VendegIgen,
		VendegTalan:    a.VendegTalan,
		VendegNem:      a.VendegNem,
		Visszajelzesek: make([]visszajelzesJSON, 0, len(a.Visszajelzesek)),
		Answerable:     a.Answerable,
		Confirmed:      a.Confirmed,
		Removable:      a.Removable,
	}
	if a.Helyezes > 0 {
		h := int(a.Helyezes)
		out.Helyezes = &h
	}
	if a.Sajat != "" {
		v := string(a.Sajat)
		out.Sajat = &v
	}
	for _, v := range a.Visszajelzesek {
		out.Visszajelzesek = append(out.Visszajelzesek, visszajelzesJSON{
			FelhasznaloID: v.FelhasznaloID,
			VendegID:      v.VendegID,
			Name:          v.FelhasznaloName,
			Valasz:        string(v.Valasz),
			IsTag:         v.IsTag,
			IsVendeg:      v.IsVendeg,
			VendegAddedBy: v.VendegAddedBy,
			UpdatedAt:     v.UpdatedAt,
		})
	}
	return out
}

func (s *server) proposeEsemeny(w http.ResponseWriter, r *http.Request, felhasznaloID string) {
	var body struct {
		Title       string       `json:"title"`
		Description string       `json:"description"`
		Helyszin    helyszinJSON `json:"helyszin"`
		// Idopontok are Budapest wall clock times, like "2026-12-05T20:00",
		// or with egeszNapos the first and last day, like "2026-12-05".
		Idopontok []struct {
			Start      string `json:"start"`
			End        string `json:"end"`
			EgeszNapos bool   `json:"egeszNapos"`
		} `json:"idopontok"`
		// Hatarido is an optional Budapest wall clock time, "" for none.
		Hatarido string `json:"hatarido"`
	}
	if !decodeJSON(w, r, &body) {
		return
	}
	in := app.EsemenyInput{
		Title:       body.Title,
		Description: body.Description,
		Helyszin:    domain.Helyszin{Name: body.Helyszin.Name, URL: body.Helyszin.URL},
		Hatarido:    body.Hatarido,
	}
	for _, i := range body.Idopontok {
		in.Idopontok = append(in.Idopontok, app.IdopontInput{Start: i.Start, End: i.End, EgeszNapos: i.EgeszNapos})
	}
	e, err := s.app.ProposeEsemeny(r.Context(), felhasznaloID, r.PathValue("id"), in)
	if err != nil {
		s.writeEsemenyError(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, toEsemenyJSON(e))
}

func (s *server) listEsemenyek(w http.ResponseWriter, r *http.Request, felhasznaloID string) {
	list, err := s.app.EsemenyekOfCsoport(r.Context(), felhasznaloID, r.PathValue("id"))
	if err != nil {
		s.writeEsemenyError(w, r, err)
		return
	}
	out := make([]esemenySummaryJSON, 0, len(list))
	for _, e := range list {
		out = append(out, esemenySummaryJSON{esemenyJSON: toEsemenyJSON(e.Esemeny), IdopontCount: e.IdopontCount, FirstStart: e.FirstStart})
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *server) getEsemeny(w http.ResponseWriter, r *http.Request, felhasznaloID string) {
	d, err := s.app.Esemeny(r.Context(), felhasznaloID, r.PathValue("id"))
	if err != nil {
		s.writeEsemenyError(w, r, err)
		return
	}
	idopontok := make([]idopontJSON, 0, len(d.Idopontok))
	for _, a := range d.Idopontok {
		idopontok = append(idopontok, toIdopontJSON(a))
	}
	resztvevok := make([]resztvevoJSON, 0, len(d.Resztvevok))
	for _, r := range d.Resztvevok {
		resztvevok = append(resztvevok, resztvevoJSON{
			FelhasznaloID: r.FelhasznaloID, VendegID: r.VendegID, Name: r.Name,
			IsTag: r.IsTag, IsVendeg: r.IsVendeg, AddedBy: r.AddedBy,
		})
	}
	vendegek := make([]vendegJSON, 0, len(d.Vendegek))
	for _, v := range d.Vendegek {
		vendegek = append(vendegek, toVendegJSON(v))
	}
	writeJSON(w, http.StatusOK, esemenyDetailJSON{
		esemenyJSON:        toEsemenyJSON(d.Esemeny),
		CsoportName:        d.CsoportName,
		SzervezoName:       d.SzervezoName,
		Idopontok:          idopontok,
		Resztvevok:         resztvevok,
		ConfirmedIdopontID: nullable(d.ConfirmedIdopont),
		Kiegeszitok:        toKiegeszitokJSON(d.Kiegeszitok),
		Hatarido:           d.Hatarido,
		ViewerIsVendeg:     d.ViewerIsVendeg,
		Vendegek:           vendegek,
	})
}

// nullable is nil for "", so JSON shows null.
func nullable(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

func (s *server) confirmIdopont(w http.ResponseWriter, r *http.Request, felhasznaloID string) {
	var body struct {
		IdopontID string `json:"idopontId"`
	}
	if !decodeJSON(w, r, &body) {
		return
	}
	if err := s.app.ConfirmIdopont(r.Context(), felhasznaloID, r.PathValue("id"), body.IdopontID); err != nil {
		s.writeEsemenyError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *server) reopenEgyeztetes(w http.ResponseWriter, r *http.Request, felhasznaloID string) {
	if err := s.app.ReopenEgyeztetes(r.Context(), felhasznaloID, r.PathValue("id")); err != nil {
		s.writeEsemenyError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *server) cancelEsemeny(w http.ResponseWriter, r *http.Request, felhasznaloID string) {
	if err := s.app.CancelEsemeny(r.Context(), felhasznaloID, r.PathValue("id")); err != nil {
		s.writeEsemenyError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *server) setVisszajelzes(w http.ResponseWriter, r *http.Request, felhasznaloID string) {
	var body struct {
		Valasz string `json:"valasz"`
	}
	if !decodeJSON(w, r, &body) {
		return
	}
	err := s.app.SetVisszajelzes(r.Context(), felhasznaloID, r.PathValue("id"), r.PathValue("idopontID"), domain.Valasz(body.Valasz))
	if err != nil {
		s.writeEsemenyError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *server) editEsemeny(w http.ResponseWriter, r *http.Request, felhasznaloID string) {
	var body struct {
		Title       string       `json:"title"`
		Description string       `json:"description"`
		Helyszin    helyszinJSON `json:"helyszin"`
	}
	if !decodeJSON(w, r, &body) {
		return
	}
	err := s.app.EditEsemeny(r.Context(), felhasznaloID, r.PathValue("id"), app.EsemenyDetailsInput{
		Title:       body.Title,
		Description: body.Description,
		Helyszin:    domain.Helyszin{Name: body.Helyszin.Name, URL: body.Helyszin.URL},
	})
	if err != nil {
		s.writeEsemenyError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *server) addIdopont(w http.ResponseWriter, r *http.Request, felhasznaloID string) {
	var body struct {
		Start      string `json:"start"`
		End        string `json:"end"`
		EgeszNapos bool   `json:"egeszNapos"`
	}
	if !decodeJSON(w, r, &body) {
		return
	}
	i, err := s.app.AddIdopont(r.Context(), felhasznaloID, r.PathValue("id"), app.IdopontInput{Start: body.Start, End: body.End, EgeszNapos: body.EgeszNapos})
	if err != nil {
		s.writeEsemenyError(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]string{"id": i.ID})
}

func (s *server) removeIdopont(w http.ResponseWriter, r *http.Request, felhasznaloID string) {
	if err := s.app.RemoveIdopont(r.Context(), felhasznaloID, r.PathValue("id"), r.PathValue("idopontID")); err != nil {
		s.writeEsemenyError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *server) copyEsemeny(w http.ResponseWriter, r *http.Request, felhasznaloID string) {
	e, err := s.app.CopyEsemeny(r.Context(), felhasznaloID, r.PathValue("id"))
	if err != nil {
		s.writeEsemenyError(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, toEsemenyJSON(e))
}

func (s *server) writeEsemenyError(w http.ResponseWriter, r *http.Request, err error) {
	if writeHataridoNotFuture(w, err) {
		return
	}
	for _, c := range []struct {
		err    error
		status int
		code   string
	}{
		{app.ErrEsemenyElmaradt, http.StatusConflict, "esemeny_elmaradt"},
		{app.ErrNotEgyeztetes, http.StatusConflict, "not_egyeztetes"},
		{app.ErrTooManyIdopontok, http.StatusConflict, "too_many_idopontok"},
		{app.ErrIdopontHasAnswers, http.StatusConflict, "idopont_has_answers"},
		{app.ErrLastIdopont, http.StatusConflict, "last_idopont"},
		{app.ErrNotPermitted, http.StatusForbidden, "not_permitted"},
	} {
		if errors.Is(err, c.err) {
			writeCodedError(w, c.status, err, c.code)
			return
		}
	}
	switch {
	case errors.Is(err, app.ErrIdopontNotAnswerable), errors.Is(err, app.ErrInvalidTransition):
		writeError(w, http.StatusConflict, err.Error())
		return
	case errors.Is(err, app.ErrNotSzervezo):
		writeError(w, http.StatusForbidden, err.Error())
		return
	}
	// Checked before the generic ErrInvalidInput it wraps, so the web can
	// explain it.
	if errors.Is(err, app.ErrNonexistentLocalTime) {
		writeError(w, http.StatusUnprocessableEntity, err.Error())
		return
	}
	s.writeAppError(w, r, err)
}
