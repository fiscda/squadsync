package gateway_test

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"squadsync-be/internal/gateway"
)

// fakeGoogle stands in for Google's token and Calendar endpoints.
type fakeGoogle struct {
	mu       sync.Mutex
	revoked  bool
	events   map[string]map[string]any
	requests []string
}

func (f *fakeGoogle) server(t *testing.T) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("POST /token", func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		defer f.mu.Unlock()
		if r.FormValue("client_id") != "cid" || r.FormValue("client_secret") != "csecret" {
			http.Error(w, `{"error":"invalid_client"}`, http.StatusUnauthorized)
			return
		}
		switch r.FormValue("grant_type") {
		case "authorization_code":
			if r.FormValue("code") != "good" || r.FormValue("redirect_uri") != "http://localhost:3000/fiok/google/callback" {
				http.Error(w, `{"error":"invalid_grant"}`, http.StatusBadRequest)
				return
			}
			_, _ = w.Write([]byte(`{"access_token":"at-1","expires_in":3600,"refresh_token":"rt"}`))
		case "refresh_token":
			if f.revoked || r.FormValue("refresh_token") != "rt" {
				http.Error(w, `{"error":"invalid_grant","error_description":"Token has been expired or revoked."}`, http.StatusBadRequest)
				return
			}
			_, _ = w.Write([]byte(`{"access_token":"at-2","expires_in":3600}`))
		}
	})
	mux.HandleFunc("POST /revoke", func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		defer f.mu.Unlock()
		f.revoked = true
	})
	api := http.NewServeMux()
	api.HandleFunc("POST /calendars", func(w http.ResponseWriter, r *http.Request) {
		var body map[string]string
		_ = json.NewDecoder(r.Body).Decode(&body)
		if body["summary"] != "Squad Sync" || body["timeZone"] != "Europe/Budapest" {
			http.Error(w, "bad calendar", http.StatusBadRequest)
			return
		}
		_, _ = w.Write([]byte(`{"id":"cal@group.calendar.google.com"}`))
	})
	api.HandleFunc("PUT /calendars/{cal}/events/{id}", func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		defer f.mu.Unlock()
		if _, ok := f.events[r.PathValue("cal")][r.PathValue("id")]; !ok {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		f.store(r)
	})
	api.HandleFunc("POST /calendars/{cal}/events", func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		defer f.mu.Unlock()
		if r.PathValue("cal") != "cal@group.calendar.google.com" {
			http.Error(w, "no calendar", http.StatusNotFound)
			return
		}
		f.store(r)
	})
	api.HandleFunc("DELETE /calendars/{cal}/events/{id}", func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		defer f.mu.Unlock()
		if _, ok := f.events[r.PathValue("cal")][r.PathValue("id")]; !ok {
			http.Error(w, "gone", http.StatusGone)
			return
		}
		delete(f.events[r.PathValue("cal")], r.PathValue("id"))
		w.WriteHeader(http.StatusNoContent)
	})
	mux.HandleFunc("/calendar/v3/", func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		f.requests = append(f.requests, r.Method+" "+r.Header.Get("Authorization"))
		f.mu.Unlock()
		if r.Header.Get("Authorization") != "Bearer at-1" && r.Header.Get("Authorization") != "Bearer at-2" {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		http.StripPrefix("/calendar/v3", api).ServeHTTP(w, r)
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

// store keeps the event body (the caller holds the lock).
func (f *fakeGoogle) store(r *http.Request) {
	body, _ := io.ReadAll(r.Body)
	var ev map[string]any
	_ = json.Unmarshal(body, &ev)
	if f.events == nil {
		f.events = map[string]map[string]any{}
	}
	cal := r.PathValue("cal")
	if f.events[cal] == nil {
		f.events[cal] = map[string]any{}
	}
	f.events[cal][ev["id"].(string)] = ev
}

func TestGoogleCalendarClientAgainstAFakeGoogle(t *testing.T) {
	f := &fakeGoogle{}
	srv := f.server(t)
	c := &gateway.GoogleCalendarClient{
		ClientID: "cid", ClientSecret: "csecret",
		TokenURL: srv.URL + "/token", RevokeURL: srv.URL + "/revoke", APIBase: srv.URL + "/calendar/v3",
	}
	ctx := context.Background()

	if _, err := c.ExchangeCalendarCode(ctx, "bad", "http://localhost:3000/fiok/google/callback"); !errors.Is(err, gateway.ErrGoogleCodeRejected) {
		t.Fatalf("bad code: err = %v, want ErrGoogleCodeRejected", err)
	}
	rt, err := c.ExchangeCalendarCode(ctx, "good", "http://localhost:3000/fiok/google/callback")
	if err != nil || rt != "rt" {
		t.Fatalf("exchange = %q, %v; want rt", rt, err)
	}
	cal, err := c.CreateCalendar(ctx, rt, "Squad Sync", "Europe/Budapest")
	if err != nil || cal != "cal@group.calendar.google.com" {
		t.Fatalf("create calendar = %q, %v", cal, err)
	}

	budapest, _ := time.LoadLocation("Europe/Budapest")
	allDay := gateway.CalendarEvent{
		ID: "ssabc123", Title: "Balaton", AllDay: true, Link: "https://squadsync.example/esemenyek/1",
		Start: time.Date(2026, 12, 5, 0, 0, 0, 0, budapest), End: time.Date(2026, 12, 8, 0, 0, 0, 0, budapest),
	}
	// Created (PUT 404, then POST), then replaced (PUT).
	for range 2 {
		if err := c.UpsertEvent(ctx, rt, cal, allDay); err != nil {
			t.Fatalf("upsert: %v", err)
		}
	}
	ev := f.events[cal]["ssabc123"].(map[string]any)
	if start := ev["start"].(map[string]any); start["date"] != "2026-12-05" {
		t.Fatalf("start = %v, want date 2026-12-05", start)
	}
	if end := ev["end"].(map[string]any); end["date"] != "2026-12-08" {
		t.Fatalf("end = %v, want date 2026-12-08", end)
	}

	timed := gateway.CalendarEvent{ID: "sstimed", Title: "Este", Start: time.Date(2026, 12, 1, 19, 0, 0, 0, time.UTC), End: time.Date(2026, 12, 1, 22, 0, 0, 0, time.UTC)}
	if err := c.UpsertEvent(ctx, rt, cal, timed); err != nil {
		t.Fatalf("upsert timed: %v", err)
	}
	if start := f.events[cal]["sstimed"].(map[string]any)["start"].(map[string]any); start["dateTime"] != "2026-12-01T20:00:00+01:00" {
		t.Fatalf("start = %v, want Budapest time", start)
	}

	if err := c.UpsertEvent(ctx, rt, "other", timed); !errors.Is(err, gateway.ErrGoogleCalendarGone) {
		t.Fatalf("upsert into a missing calendar: err = %v, want ErrGoogleCalendarGone", err)
	}
	if err := c.DeleteEvent(ctx, rt, cal, "sstimed"); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if err := c.DeleteEvent(ctx, rt, cal, "sstimed"); err != nil {
		t.Fatalf("deleting again: %v, want no error", err)
	}

	// Revoked: the cached access token stops working and refreshing fails.
	if err := c.RevokeCalendarToken(ctx, rt); err != nil {
		t.Fatalf("revoke: %v", err)
	}
	if err := c.UpsertEvent(ctx, rt, cal, timed); !errors.Is(err, gateway.ErrGoogleTokenRevoked) {
		t.Fatalf("after revoking: err = %v, want ErrGoogleTokenRevoked", err)
	}
}
