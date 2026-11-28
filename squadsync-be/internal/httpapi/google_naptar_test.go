package httpapi_test

import (
	"io"
	"log/slog"
	"net/http"
	"testing"

	"squadsync-be/internal/app"
	"squadsync-be/internal/apptest"
	"squadsync-be/internal/gateway"
	"squadsync-be/internal/httpapi"
)

type googleNaptarResp struct {
	State       string  `json:"state"`
	ConnectedAt *string `json:"connectedAt"`
}

func TestConnectAndDisconnectGoogleNaptarOverHTTP(t *testing.T) {
	h := apptest.New(t)
	api := httpapi.New(h.App, secret, slog.New(slog.NewTextHandler(io.Discard, nil)))
	login(t, api, "anna", "d-anna", "Anna")
	h.Calendar.Authorize("good", "refresh-anna")

	got := decode[googleNaptarResp](t, call(t, api, http.MethodGet, "/felhasznalo/google-naptar", "", as("anna")))
	if got.State != "disconnected" || got.ConnectedAt != nil {
		t.Fatalf("status = %+v, want disconnected", got)
	}

	// Connecting needs the caller, and a code Google accepts.
	expectStatus(t, call(t, api, http.MethodPost, "/felhasznalo/google-naptar", `{"code":"good"}`, map[string]string{httpapi.HeaderSecret: secret}), http.StatusBadRequest)
	expectStatus(t, call(t, api, http.MethodPost, "/felhasznalo/google-naptar", `{"code":"bad"}`, as("anna")), http.StatusBadRequest)
	expectStatus(t, call(t, api, http.MethodPost, "/felhasznalo/google-naptar", `{"code":"good"}`, as("anna")), http.StatusNoContent)
	// Go used its own redirect URI.
	ex := h.Calendar.CalendarExchanges()
	if last := ex[len(ex)-1]; last.RedirectURI != apptest.WebURL+"/fiok/google/callback" {
		t.Fatalf("exchange = %+v, want Go's redirect URI", last)
	}

	got = decode[googleNaptarResp](t, call(t, api, http.MethodGet, "/felhasznalo/google-naptar", "", as("anna")))
	if got.State != "connected" || got.ConnectedAt == nil {
		t.Fatalf("status = %+v, want connected", got)
	}

	expectStatus(t, call(t, api, http.MethodDelete, "/felhasznalo/google-naptar", "", as("anna")), http.StatusNoContent)
	got = decode[googleNaptarResp](t, call(t, api, http.MethodGet, "/felhasznalo/google-naptar", "", as("anna")))
	if got.State != "disconnected" {
		t.Fatalf("status = %+v, want disconnected", got)
	}
	// Disconnecting again is harmless.
	expectStatus(t, call(t, api, http.MethodDelete, "/felhasznalo/google-naptar", "", as("anna")), http.StatusNoContent)
	// An unknown Felhasznalo.
	expectStatus(t, call(t, api, http.MethodGet, "/felhasznalo/google-naptar", "", as("nobody")), http.StatusNotFound)
}

func TestGoogleNaptarWithoutConfigurationIsUnavailableOverHTTP(t *testing.T) {
	h := apptest.New(t)
	a := app.New(app.Deps{
		Store: h.Store, Clock: h.Clock, Discord: h.Discord, Calendar: gateway.Disabled{}, Spotify: h.Spotify,
		DiscordOAuth: h.DiscordOAuth, WebURL: apptest.WebURL,
	})
	api := httpapi.New(a, secret, slog.New(slog.NewTextHandler(io.Discard, nil)))
	login(t, api, "anna", "d-anna", "Anna")

	got := decode[googleNaptarResp](t, call(t, api, http.MethodGet, "/felhasznalo/google-naptar", "", as("anna")))
	if got.State != "not_configured" {
		t.Fatalf("status = %+v, want not_configured", got)
	}
	expectStatus(t, call(t, api, http.MethodPost, "/felhasznalo/google-naptar", `{"code":"good"}`, as("anna")), http.StatusServiceUnavailable)
}
