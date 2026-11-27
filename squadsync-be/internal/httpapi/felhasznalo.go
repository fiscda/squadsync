package httpapi

import (
	"errors"
	"net/http"
	"time"

	"squadsync-be/internal/app"
	"squadsync-be/internal/domain"
)

func (s *server) registerFelhasznalo(mux *http.ServeMux) {
	mux.HandleFunc("POST /felhasznalo/first-login", withFelhasznalo(s.firstLogin))
	mux.HandleFunc("GET /felhasznalo", withFelhasznalo(s.getFelhasznalo))
}

// felhasznaloJSON is the wire form of a Felhasznalo. Keep it in sync by hand
// with the Felhasznalo type in squadsync-fe/src/lib/backend.ts.
type felhasznaloJSON struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	CreatedAt time.Time `json:"createdAt"`
}

func toFelhasznaloJSON(f domain.Felhasznalo) felhasznaloJSON {
	return felhasznaloJSON{ID: f.ID, Name: f.Name, CreatedAt: f.CreatedAt}
}

func (s *server) firstLogin(w http.ResponseWriter, r *http.Request, felhasznaloID string) {
	var body struct {
		DiscordID string `json:"discordId"`
		Name      string `json:"name"`
	}
	if !decodeJSON(w, r, &body) {
		return
	}
	f, err := s.app.FirstLogin(r.Context(), app.FirstLoginInput{
		FelhasznaloID: felhasznaloID,
		DiscordID:     body.DiscordID,
		Name:          body.Name,
	})
	if err != nil {
		s.writeFelhasznaloError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, toFelhasznaloJSON(f))
}

func (s *server) getFelhasznalo(w http.ResponseWriter, r *http.Request, felhasznaloID string) {
	f, err := s.app.Felhasznalo(r.Context(), felhasznaloID)
	if err != nil {
		s.writeFelhasznaloError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, toFelhasznaloJSON(f))
}

func (s *server) writeFelhasznaloError(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, app.ErrDiscordAccountInUse), errors.Is(err, app.ErrDiscordAccountNotOwn):
		writeError(w, http.StatusConflict, err.Error())
	default:
		s.writeAppError(w, r, err)
	}
}
