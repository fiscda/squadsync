package httpapi

import (
	"errors"
	"net/http"

	"squadsync-be/internal/app"
	"squadsync-be/internal/domain"
)

// The Kiegeszito framework's routes. Each Kiegeszito registers its own
// routes in its own file and maps its errors with writeKiegeszitoError.
func (s *server) registerKiegeszito(mux *http.ServeMux) {
	mux.HandleFunc("PUT /esemeny/{id}/kiegeszito/{kind}", withFelhasznalo(s.setKiegeszito))
}

// kiegeszitoJSON is one Kiegeszito of an Esemeny, in esemenyDetailJSON.
type kiegeszitoJSON struct {
	Kind    string `json:"kind"`
	Enabled bool   `json:"enabled"`
	// Switchable: the Szervezo can switch it in the Esemeny's state.
	Switchable bool `json:"switchable"`
}

func toKiegeszitokJSON(list []app.KiegeszitoAllapot) []kiegeszitoJSON {
	out := make([]kiegeszitoJSON, 0, len(list))
	for _, k := range list {
		out = append(out, kiegeszitoJSON{Kind: string(k.Kind), Enabled: k.Enabled, Switchable: k.Switchable})
	}
	return out
}

func (s *server) setKiegeszito(w http.ResponseWriter, r *http.Request, felhasznaloID string) {
	var body struct {
		Enabled bool `json:"enabled"`
	}
	if !decodeJSON(w, r, &body) {
		return
	}
	err := s.app.SetKiegeszito(r.Context(), felhasznaloID, r.PathValue("id"), domain.KiegeszitoKind(r.PathValue("kind")), body.Enabled)
	if err != nil {
		s.writeKiegeszitoError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// writeKiegeszitoError maps the framework's errors, with a machine-readable
// code, then falls back to the Esemeny's. A Kiegeszito maps its own errors
// first and then calls this.
func (s *server) writeKiegeszitoError(w http.ResponseWriter, r *http.Request, err error) {
	for _, c := range []struct {
		err    error
		status int
		code   string
	}{
		{app.ErrKiegeszitoOff, http.StatusConflict, "kiegeszito_off"},
		{app.ErrKiegeszitoLocked, http.StatusConflict, "kiegeszito_locked"},
		{app.ErrKiegeszitoInUse, http.StatusConflict, "kiegeszito_in_use"},
		{app.ErrSetupClosed, http.StatusConflict, "setup_closed"},
		{app.ErrClaimingClosed, http.StatusConflict, "claiming_closed"},
		{app.ErrNotResztvevo, http.StatusForbidden, "not_resztvevo"},
		{app.ErrNotPermitted, http.StatusForbidden, "not_permitted"},
	} {
		if errors.Is(err, c.err) {
			writeCodedError(w, c.status, err, c.code)
			return
		}
	}
	s.writeEsemenyError(w, r, err)
}

// writeCodedError writes {"error": ..., "code": ...}.
func writeCodedError(w http.ResponseWriter, status int, err error, code string) {
	writeJSON(w, status, map[string]string{"error": err.Error(), "code": code})
}
