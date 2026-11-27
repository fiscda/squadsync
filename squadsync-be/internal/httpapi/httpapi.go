// Package httpapi is the internal HTTP API that Next.js calls on localhost
// (ADR-0004). It is a thin translator: it checks the shared secret, reads the
// Felhasznalo id header, decodes JSON, calls one use case and encodes the
// result. Rules belong in package app, not here.
//
// Every request must carry the shared secret, or it gets 401:
//
//	X-Internal-Secret: <INTERNAL_API_SECRET>
//
// Routes that act for a Felhasznalo are wrapped in withFelhasznalo and also
// need the header below, or they get 400. Routes that run before a
// Felhasznalo exists (such as a pre-login check) need only the secret.
//
//	X-Felhasznalo-Id: <Better Auth user.id>
//
// Each area registers its routes in its own file with a register<Area>
// function, called from New.
package httpapi

import (
	"crypto/subtle"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"

	"squadsync-be/internal/app"
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
	s.registerFelhasznalo(mux)

	return requireSecret(secret, mux)
}

type server struct {
	app *app.App
	log *slog.Logger
}

// requireSecret rejects every request without the correct shared secret.
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

// felhasznaloHandler handles a request made for one Felhasznalo.
type felhasznaloHandler func(w http.ResponseWriter, r *http.Request, felhasznaloID string)

// withFelhasznalo requires the X-Felhasznalo-Id header and passes its value
// to h.
func withFelhasznalo(h felhasznaloHandler) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := r.Header.Get(HeaderFelhasznaloID)
		if id == "" {
			writeError(w, http.StatusBadRequest, "missing "+HeaderFelhasznaloID+" header")
			return
		}
		h(w, r, id)
	}
}

// decodeJSON reads a small JSON request body into v, answering 400 itself
// when it can't. It reports whether decoding succeeded.
func decodeJSON(w http.ResponseWriter, r *http.Request, v any) bool {
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<16)).Decode(v); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return false
	}
	return true
}

// writeAppError maps a use-case error to a response. Area-specific errors
// are mapped in their area's file before falling back to this.
func (s *server) writeAppError(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, app.ErrNotFound):
		writeError(w, http.StatusNotFound, err.Error())
	case errors.Is(err, app.ErrInvalidInput):
		writeError(w, http.StatusBadRequest, err.Error())
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
