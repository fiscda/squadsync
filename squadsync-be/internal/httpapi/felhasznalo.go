package httpapi

import (
	"errors"
	"net/http"
	"time"

	"squadsync-be/internal/app"
	"squadsync-be/internal/domain"
	"squadsync-be/internal/gateway"
)

func (s *server) registerFelhasznalo(mux *http.ServeMux) {
	mux.HandleFunc("POST /felhasznalo/first-login", withFelhasznalo(s.firstLogin))
	mux.HandleFunc("GET /felhasznalo", withFelhasznalo(s.getFelhasznalo))
	mux.HandleFunc("GET /felhasznalo/discord-account", withFelhasznalo(s.listDiscordAccounts))
	mux.HandleFunc("POST /felhasznalo/discord-account", withFelhasznalo(s.linkDiscordAccount))
	mux.HandleFunc("DELETE /felhasznalo/discord-account/{discordID}", withFelhasznalo(s.unlinkDiscordAccount))

	// Asked by Better Auth while a login is in progress, before any
	// Felhasznalo is known: it needs only the secret.
	mux.HandleFunc("GET /discord-account/{discordID}/web-login", s.webLogin)
}

// felhasznaloJSON is the wire form of a Felhasznalo. Keep it in sync by hand
// with the Felhasznalo type in squadsync-fe/src/lib/backend.ts.
type felhasznaloJSON struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	CreatedAt time.Time `json:"createdAt"`
}

// discordAccountJSON is the wire form of a DiscordAccount. Keep it in sync by
// hand with the DiscordAccount type in squadsync-fe/src/lib/backend.ts.
type discordAccountJSON struct {
	DiscordID string    `json:"discordId"`
	Name      string    `json:"name"`
	Main      bool      `json:"main"`
	CreatedAt time.Time `json:"createdAt"`
}

func toDiscordAccountJSON(a domain.DiscordAccount) discordAccountJSON {
	return discordAccountJSON{DiscordID: a.DiscordID, Name: a.Name, Main: a.IsMain, CreatedAt: a.CreatedAt}
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

func (s *server) listDiscordAccounts(w http.ResponseWriter, r *http.Request, felhasznaloID string) {
	accounts, err := s.app.DiscordAccounts(r.Context(), felhasznaloID)
	if err != nil {
		s.writeFelhasznaloError(w, r, err)
		return
	}
	out := make([]discordAccountJSON, 0, len(accounts))
	for _, a := range accounts {
		out = append(out, toDiscordAccountJSON(a))
	}
	writeJSON(w, http.StatusOK, out)
}

// linkDiscordAccount finishes the linking flow: Next.js forwards the code
// from Discord's redirect. 409 means the account belongs to someone with
// data; 400 means Discord refused the code; 503 means linking isn't
// configured.
func (s *server) linkDiscordAccount(w http.ResponseWriter, r *http.Request, felhasznaloID string) {
	var body struct {
		Code string `json:"code"`
	}
	if !decodeJSON(w, r, &body) {
		return
	}
	res, err := s.app.LinkDiscordAccount(r.Context(), felhasznaloID, body.Code)
	if err != nil {
		s.writeFelhasznaloError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, struct {
		Account              discordAccountJSON `json:"account"`
		AlreadyLinked        bool               `json:"alreadyLinked"`
		RemovedFelhasznaloID string             `json:"removedFelhasznaloId"`
	}{toDiscordAccountJSON(res.Account), res.AlreadyLinked, res.RemovedFelhasznaloID})
}

func (s *server) unlinkDiscordAccount(w http.ResponseWriter, r *http.Request, felhasznaloID string) {
	if err := s.app.UnlinkDiscordAccount(r.Context(), felhasznaloID, r.PathValue("discordID")); err != nil {
		s.writeFelhasznaloError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// webLogin answers whether a Discord account may log in to the web, and if
// not, the message to show.
func (s *server) webLogin(w http.ResponseWriter, r *http.Request) {
	d, err := s.app.WebLogin(r.Context(), r.PathValue("discordID"))
	if err != nil {
		s.writeFelhasznaloError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, struct {
		Allowed bool   `json:"allowed"`
		Message string `json:"message"`
	}{d.Allowed, d.RefusalMessage()})
}

func (s *server) writeFelhasznaloError(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, app.ErrDiscordAccountInUse), errors.Is(err, app.ErrDiscordAccountNotOwn):
		writeError(w, http.StatusConflict, err.Error())
	case errors.Is(err, app.ErrMainDiscordAccount):
		writeError(w, http.StatusConflict, err.Error())
	case errors.Is(err, gateway.ErrNotConfigured):
		writeError(w, http.StatusServiceUnavailable, "discord login is not configured")
	default:
		s.writeAppError(w, r, err)
	}
}
