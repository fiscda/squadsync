package gateway_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"squadsync-be/internal/gateway"
)

// fakeDiscord stands in for Discord's API on a local test server.
func fakeDiscord(t *testing.T) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("POST /oauth2/token", func(w http.ResponseWriter, r *http.Request) {
		id, secret, ok := r.BasicAuth()
		if !ok || id != "client-id" || secret != "client-secret" {
			http.Error(w, `{"error":"invalid_client"}`, http.StatusUnauthorized)
			return
		}
		if r.FormValue("grant_type") != "authorization_code" || r.FormValue("code") != "good-code" ||
			r.FormValue("redirect_uri") != "http://localhost:3000/fiok/discord/callback" {
			http.Error(w, `{"error":"invalid_grant"}`, http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"access_token":"tok","token_type":"Bearer","scope":"identify"}`))
	})
	mux.HandleFunc("GET /users/@me", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer tok" {
			http.Error(w, `{"message":"401: Unauthorized"}`, http.StatusUnauthorized)
			return
		}
		_, _ = w.Write([]byte(`{"id":"123456","username":"anna_phone","global_name":"Anna telefon"}`))
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

func TestDiscordOAuthClientExchangesTheCodeAndReadsTheAccount(t *testing.T) {
	srv := fakeDiscord(t)
	c := &gateway.DiscordOAuthClient{ClientID: "client-id", ClientSecret: "client-secret", BaseURL: srv.URL}

	got, err := c.ExchangeCode(context.Background(), "good-code", "http://localhost:3000/fiok/discord/callback")
	if err != nil {
		t.Fatalf("ExchangeCode: %v", err)
	}
	if got != (gateway.DiscordIdentity{ID: "123456", Name: "Anna telefon"}) {
		t.Fatalf("identity = %+v, want 123456 Anna telefon", got)
	}
}

func TestDiscordOAuthClientReportsARefusedCode(t *testing.T) {
	srv := fakeDiscord(t)
	c := &gateway.DiscordOAuthClient{ClientID: "client-id", ClientSecret: "client-secret", BaseURL: srv.URL}

	if _, err := c.ExchangeCode(context.Background(), "good-code", "https://elsewhere.example/callback"); !errors.Is(err, gateway.ErrDiscordCodeRejected) {
		t.Errorf("wrong redirect: err = %v, want ErrDiscordCodeRejected", err)
	}
	if _, err := c.ExchangeCode(context.Background(), "bad-code", "http://localhost:3000/fiok/discord/callback"); !errors.Is(err, gateway.ErrDiscordCodeRejected) {
		t.Errorf("bad code: err = %v, want ErrDiscordCodeRejected", err)
	}
}
