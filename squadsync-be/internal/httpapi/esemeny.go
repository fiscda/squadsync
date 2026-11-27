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
	mux.HandleFunc("PUT /esemeny/{id}/idopont/{idopontID}/visszajelzes", withFelhasznalo(s.setVisszajelzes))
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
	FelhasznaloID string    `json:"felhasznaloId"`
	Name          string    `json:"name"`
	Valasz        string    `json:"valasz"`
	IsTag         bool      `json:"isTag"`
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
	// Helyezes is null when no Tag has answered.
	Helyezes *int `json:"helyezes"`
	// Sajat is the caller's own answer, null if none.
	Sajat          *string            `json:"sajat"`
	Visszajelzesek []visszajelzesJSON `json:"visszajelzesek"`
}

type esemenyDetailJSON struct {
	esemenyJSON
	CsoportName  string        `json:"csoportName"`
	SzervezoName string        `json:"szervezoName"`
	Idopontok    []idopontJSON `json:"idopontok"`
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
		Visszajelzesek: make([]visszajelzesJSON, 0, len(a.Visszajelzesek)),
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
			Name:          v.FelhasznaloName,
			Valasz:        string(v.Valasz),
			IsTag:         v.IsTag,
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
	}
	if !decodeJSON(w, r, &body) {
		return
	}
	in := app.EsemenyInput{
		Title:       body.Title,
		Description: body.Description,
		Helyszin:    domain.Helyszin{Name: body.Helyszin.Name, URL: body.Helyszin.URL},
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
	writeJSON(w, http.StatusOK, esemenyDetailJSON{
		esemenyJSON:  toEsemenyJSON(d.Esemeny),
		CsoportName:  d.CsoportName,
		SzervezoName: d.SzervezoName,
		Idopontok:    idopontok,
	})
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

func (s *server) writeEsemenyError(w http.ResponseWriter, r *http.Request, err error) {
	if errors.Is(err, app.ErrIdopontNotAnswerable) {
		writeError(w, http.StatusConflict, err.Error())
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
