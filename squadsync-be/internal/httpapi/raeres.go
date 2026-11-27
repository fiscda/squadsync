package httpapi

import (
	"errors"
	"net/http"

	"squadsync-be/internal/app"
	"squadsync-be/internal/domain"
)

// No route returns another Felhasznalo's Raeres: fellow Tags only ever get
// counts, and only through a Csoport or Esemeny they are a Tag of.
func (s *server) registerRaeres(mux *http.ServeMux) {
	mux.HandleFunc("GET /raeres", withFelhasznalo(s.getRaeres))
	mux.HandleFunc("PUT /raeres", withFelhasznalo(s.setRaeres))
	mux.HandleFunc("GET /csoport/{id}/raeres", withFelhasznalo(s.getHeatmap))
	mux.HandleFunc("GET /esemeny/{id}/raeres", withFelhasznalo(s.getEsemenyRaeres))
}

// The wire forms below are kept in sync by hand with the types in
// squadsync-fe/src/lib/backend.ts. Days are Budapest dates like "2026-12-05".

type raeresDayJSON struct {
	Day     string `json:"day"`
	Allapot string `json:"allapot"`
}

type heatmapDayJSON struct {
	Day    string `json:"day"`
	Szabad int    `json:"szabad"`
	Osszes int    `json:"osszes"`
}

type idopontRaeresJSON struct {
	IdopontID string `json:"idopontId"`
	Szabad    int    `json:"szabad"`
	Osszes    int    `json:"osszes"`
	// Sajat is the caller's own Raeres over the covered days: "szabad",
	// "foglalt", or null when unknown or mixed.
	Sajat *string `json:"sajat"`
}

func (s *server) getRaeres(w http.ResponseWriter, r *http.Request, felhasznaloID string) {
	days, err := s.app.Raeres(r.Context(), felhasznaloID, r.URL.Query().Get("from"), r.URL.Query().Get("to"))
	if err != nil {
		s.writeAppError(w, r, err)
		return
	}
	out := make([]raeresDayJSON, 0, len(days))
	for _, d := range days {
		out = append(out, raeresDayJSON{Day: d.Day, Allapot: string(d.Allapot)})
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *server) setRaeres(w http.ResponseWriter, r *http.Request, felhasznaloID string) {
	var body struct {
		From string `json:"from"`
		To   string `json:"to"`
		// Allapot is "szabad", "foglalt", or "" to clear the days.
		Allapot string `json:"allapot"`
	}
	if !decodeJSON(w, r, &body) {
		return
	}
	err := s.app.SetRaeres(r.Context(), felhasznaloID, body.From, body.To, domain.RaeresAllapot(body.Allapot))
	if errors.Is(err, app.ErrFelhasznaloUnknown) {
		writeError(w, http.StatusForbidden, err.Error())
		return
	}
	if err != nil {
		s.writeAppError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *server) getHeatmap(w http.ResponseWriter, r *http.Request, felhasznaloID string) {
	days, err := s.app.CsoportHeatmap(r.Context(), felhasznaloID, r.PathValue("id"), r.URL.Query().Get("from"), r.URL.Query().Get("to"))
	if err != nil {
		s.writeAppError(w, r, err)
		return
	}
	out := make([]heatmapDayJSON, 0, len(days))
	for _, d := range days {
		out = append(out, heatmapDayJSON{Day: d.Day, Szabad: d.Szabad, Osszes: d.Osszes})
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *server) getEsemenyRaeres(w http.ResponseWriter, r *http.Request, felhasznaloID string) {
	list, err := s.app.EsemenyRaeres(r.Context(), felhasznaloID, r.PathValue("id"))
	if err != nil {
		s.writeAppError(w, r, err)
		return
	}
	out := make([]idopontRaeresJSON, 0, len(list))
	for _, i := range list {
		j := idopontRaeresJSON{IdopontID: i.IdopontID, Szabad: i.Szabad, Osszes: i.Osszes}
		if i.Sajat != "" {
			v := string(i.Sajat)
			j.Sajat = &v
		}
		out = append(out, j)
	}
	writeJSON(w, http.StatusOK, out)
}
