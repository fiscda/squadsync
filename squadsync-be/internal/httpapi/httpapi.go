// Package httpapi is the internal HTTP API that Next.js calls on localhost
// (ADR-0004). It is a thin translator: it checks the shared secret, reads the
// Felhasznalo id header, decodes JSON, calls one use case and encodes the
// result. Rules belong in package app, not here.
//
// Every request must carry:
//
//	X-Internal-Secret: <INTERNAL_API_SECRET>
//	X-Felhasznalo-Id:  <Better Auth user.id>
//
// Requests without the correct secret get 401; without a Felhasznalo id, 400.
package httpapi

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"time"

	"squadsync-be/internal/app"
	"squadsync-be/internal/domain"
)

// Header names shared with the Next.js client (squadsync-fe/src/lib/backend.ts).
const (
	HeaderSecret        = "X-Internal-Secret"
	HeaderFelhasznaloID = "X-Felhasznalo-Id"
)

// New returns the internal API handler.
func New(a *app.App, secret string, log *slog.Logger) http.Handler {
	s := &server{app: a, log: log}

	mux := http.NewServeMux()
	mux.HandleFunc("POST /felhasznalo/first-login", s.firstLogin)
	mux.HandleFunc("GET /felhasznalo", s.getFelhasznalo)

	return requireSecret(secret, requireFelhasznaloID(mux))
}

type server struct {
	app *app.App
	log *slog.Logger
}

// --- middleware ---

type felhasznaloIDKey struct{}

func requireSecret(secret string, next http.Handler) http.Handler {
	want := []byte(secret)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got := []byte(r.Header.Get(HeaderSecret))
		if len(want) == 0 || subtle.ConstantTimeCompare(got, want) != 1 {
			writeError(w, http.StatusUnauthorized, "unauthorized")
			return
		}
		next.ServeHTTP(w, r)
	})
}

func requireFelhasznaloID(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := r.Header.Get(HeaderFelhasznaloID)
		if id == "" {
			writeError(w, http.StatusBadRequest, "missing "+HeaderFelhasznaloID+" header")
			return
		}
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), felhasznaloIDKey{}, id)))
	})
}

func felhasznaloID(r *http.Request) string {
	id, _ := r.Context().Value(felhasznaloIDKey{}).(string)
	return id
}

// --- handlers ---

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

func (s *server) firstLogin(w http.ResponseWriter, r *http.Request) {
	var body struct {
		DiscordID string `json:"discordId"`
		Name      string `json:"name"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<16)).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	f, err := s.app.FirstLogin(r.Context(), app.FirstLoginInput{
		FelhasznaloID: felhasznaloID(r),
		DiscordID:     body.DiscordID,
		Name:          body.Name,
	})
	if err != nil {
		s.writeAppError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, toFelhasznaloJSON(f))
}

func (s *server) getFelhasznalo(w http.ResponseWriter, r *http.Request) {
	f, err := s.app.Felhasznalo(r.Context(), felhasznaloID(r))
	if err != nil {
		s.writeAppError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, toFelhasznaloJSON(f))
}

// --- responses ---

func (s *server) writeAppError(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, app.ErrNotFound):
		writeError(w, http.StatusNotFound, err.Error())
	case errors.Is(err, app.ErrInvalidInput):
		writeError(w, http.StatusBadRequest, err.Error())
	case errors.Is(err, app.ErrDiscordAccountInUse):
		writeError(w, http.StatusConflict, err.Error())
	default:
		s.log.Error("internal API request failed", "method", r.Method, "path", r.URL.Path, "err", err)
		writeError(w, http.StatusInternalServerError, "internal error")
	}
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
