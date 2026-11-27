package httpapi_test

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"squadsync-be/internal/apptest"
	"squadsync-be/internal/httpapi"
)

const secret = "test-secret-0123456789abcdef0123456789"

func newAPI(t *testing.T) http.Handler {
	t.Helper()
	h := apptest.New(t)
	return httpapi.New(h.App, secret, slog.New(slog.NewTextHandler(io.Discard, nil)))
}

func request(method, path, body string, headers map[string]string) *http.Request {
	r := httptest.NewRequest(method, path, strings.NewReader(body))
	for k, v := range headers {
		r.Header.Set(k, v)
	}
	return r
}

func TestRequestsWithoutTheCorrectSecretAreRejected(t *testing.T) {
	api := newAPI(t)
	for name, headers := range map[string]map[string]string{
		"missing secret": {httpapi.HeaderFelhasznaloID: "ba-user-anna"},
		"wrong secret":   {httpapi.HeaderSecret: "nope", httpapi.HeaderFelhasznaloID: "ba-user-anna"},
	} {
		t.Run(name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			api.ServeHTTP(rec, request(http.MethodGet, "/felhasznalo", "", headers))
			if rec.Code != http.StatusUnauthorized {
				t.Fatalf("status = %d, want 401", rec.Code)
			}
		})
	}
}

func TestFelhasznaloRouteRequiresFelhasznaloID(t *testing.T) {
	api := newAPI(t)
	rec := httptest.NewRecorder()
	api.ServeHTTP(rec, request(http.MethodGet, "/felhasznalo", "", map[string]string{httpapi.HeaderSecret: secret}))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
}

func TestFirstLoginThenGetFelhasznalo(t *testing.T) {
	api := newAPI(t)
	headers := map[string]string{httpapi.HeaderSecret: secret, httpapi.HeaderFelhasznaloID: "ba-user-anna"}

	rec := httptest.NewRecorder()
	api.ServeHTTP(rec, request(http.MethodPost, "/felhasznalo/first-login", `{"discordId":"111","name":"Anna"}`, headers))
	if rec.Code != http.StatusOK {
		t.Fatalf("first-login status = %d, want 200 (body %s)", rec.Code, rec.Body)
	}

	rec = httptest.NewRecorder()
	api.ServeHTTP(rec, request(http.MethodGet, "/felhasznalo", "", headers))
	if rec.Code != http.StatusOK {
		t.Fatalf("get status = %d, want 200 (body %s)", rec.Code, rec.Body)
	}
	var got struct {
		ID   string `json:"id"`
		Name string `json:"name"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.ID != "ba-user-anna" || got.Name != "Anna" {
		t.Fatalf("GET /felhasznalo = %+v, want id ba-user-anna, name Anna", got)
	}
}
