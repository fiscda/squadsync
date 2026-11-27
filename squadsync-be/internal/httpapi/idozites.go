package httpapi

import (
	"errors"
	"net/http"
	"time"

	"squadsync-be/internal/app"
)

// Timed settings: a Csoport's Emlekezteto offsets and an Esemeny's voting
// Hatarido. Reading them is part of GET /csoport/{id} and GET /esemeny/{id}.
func (s *server) registerIdozites(mux *http.ServeMux) {
	mux.HandleFunc("PUT /csoport/{id}/emlekezteto", withFelhasznalo(s.setEmlekeztetok))
	mux.HandleFunc("PUT /esemeny/{id}/hatarido", withFelhasznalo(s.setHatarido))
}

// percek turns Emlekezteto offsets into minutes, for JSON.
func percek(offsets []time.Duration) []int {
	out := make([]int, len(offsets))
	for n, o := range offsets {
		out[n] = int(o / time.Minute)
	}
	return out
}

func (s *server) setEmlekeztetok(w http.ResponseWriter, r *http.Request, felhasznaloID string) {
	var body struct {
		// Percek are minutes before the start, each one of
		// app.EmlekeztetoValasztek; [] for none.
		Percek []int `json:"percek"`
	}
	if !decodeJSON(w, r, &body) {
		return
	}
	offsets := make([]time.Duration, len(body.Percek))
	for n, p := range body.Percek {
		offsets[n] = time.Duration(p) * time.Minute
	}
	if err := s.app.SetEmlekeztetok(r.Context(), felhasznaloID, r.PathValue("id"), offsets); err != nil {
		s.writeCsoportError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *server) setHatarido(w http.ResponseWriter, r *http.Request, felhasznaloID string) {
	var body struct {
		// Hatarido is a Budapest wall clock time like "2026-12-05T20:00",
		// or "" to clear it.
		Hatarido string `json:"hatarido"`
	}
	if !decodeJSON(w, r, &body) {
		return
	}
	if err := s.app.SetHatarido(r.Context(), felhasznaloID, r.PathValue("id"), body.Hatarido); err != nil {
		s.writeEsemenyError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// writeHataridoNotFuture answers a Hatarido that isn't in the future, with
// a code the web can tell apart from other 400s. It reports whether it did.
func writeHataridoNotFuture(w http.ResponseWriter, err error) bool {
	if !errors.Is(err, app.ErrHataridoNotFuture) {
		return false
	}
	writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error(), "code": "hatarido_not_future"})
	return true
}
