package httpapi

import (
	"errors"
	"net/http"
	"time"

	"squadsync-be/internal/app"
	"squadsync-be/internal/gateway"
)

func (s *server) registerGoogleNaptar(mux *http.ServeMux) {
	mux.HandleFunc("GET /felhasznalo/google-naptar", withFelhasznalo(s.getGoogleNaptar))
	mux.HandleFunc("POST /felhasznalo/google-naptar", withFelhasznalo(s.connectGoogleNaptar))
	mux.HandleFunc("DELETE /felhasznalo/google-naptar", withFelhasznalo(s.disconnectGoogleNaptar))
}

// googleNaptarJSON is the wire form of a GoogleNaptarStatus. Keep it in sync
// by hand with the GoogleNaptar type in squadsync-fe/src/lib/backend.ts.
type googleNaptarJSON struct {
	// State is not_configured, disconnected, connected or broken.
	State       string     `json:"state"`
	ConnectedAt *time.Time `json:"connectedAt"`
}

func (s *server) getGoogleNaptar(w http.ResponseWriter, r *http.Request, felhasznaloID string) {
	st, err := s.app.GoogleNaptar(r.Context(), felhasznaloID)
	if err != nil {
		s.writeGoogleNaptarError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, googleNaptarJSON{State: string(st.State), ConnectedAt: st.ConnectedAt})
}

// connectGoogleNaptar finishes the connecting flow: Next.js forwards the
// code from Google's redirect. 400 means Google refused the code; 503
// means Google Calendar isn't configured.
func (s *server) connectGoogleNaptar(w http.ResponseWriter, r *http.Request, felhasznaloID string) {
	var body struct {
		Code string `json:"code"`
	}
	if !decodeJSON(w, r, &body) {
		return
	}
	if err := s.app.ConnectGoogleNaptar(r.Context(), felhasznaloID, body.Code); err != nil {
		s.writeGoogleNaptarError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *server) disconnectGoogleNaptar(w http.ResponseWriter, r *http.Request, felhasznaloID string) {
	if err := s.app.DisconnectGoogleNaptar(r.Context(), felhasznaloID); err != nil {
		s.writeGoogleNaptarError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *server) writeGoogleNaptarError(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, gateway.ErrNotConfigured):
		writeError(w, http.StatusServiceUnavailable, "google calendar is not configured")
	case errors.Is(err, app.ErrGoogleCodeRejected):
		writeError(w, http.StatusBadRequest, err.Error())
	default:
		s.writeAppError(w, r, err)
	}
}
