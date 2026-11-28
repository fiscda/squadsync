package gateway

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

// Google's OAuth and Calendar endpoints.
const (
	GoogleTokenURL        = "https://oauth2.googleapis.com/token"
	GoogleRevokeURL       = "https://oauth2.googleapis.com/revoke"
	GoogleCalendarAPIBase = "https://www.googleapis.com/calendar/v3"
)

// googleTimeZone is the time zone of the calendar and of all-day dates.
const googleTimeZone = "Europe/Budapest"

var budapest = func() *time.Location {
	loc, err := time.LoadLocation(googleTimeZone)
	if err != nil {
		panic(fmt.Sprintf("load time zone %s: %v", googleTimeZone, err))
	}
	return loc
}()

// GoogleCalendarClient is the real GoogleCalendar, on plain net/http. It
// uses the Google OAuth client of GOOGLE_CLIENT_ID and GOOGLE_CLIENT_SECRET
// and keeps access tokens in memory, refreshing them from the stored
// refresh token when they expire.
type GoogleCalendarClient struct {
	ClientID     string
	ClientSecret string
	// The URLs default to GoogleTokenURL, GoogleRevokeURL and
	// GoogleCalendarAPIBase.
	TokenURL  string
	RevokeURL string
	APIBase   string
	// HTTPClient defaults to a client with a 15-second timeout.
	HTTPClient *http.Client

	mu     sync.Mutex
	tokens map[string]googleAccessToken // by refresh token
}

var _ GoogleCalendar = (*GoogleCalendarClient)(nil)

type googleAccessToken struct {
	token   string
	expires time.Time
}

func (c *GoogleCalendarClient) client() *http.Client {
	if c.HTTPClient != nil {
		return c.HTTPClient
	}
	return &http.Client{Timeout: 15 * time.Second}
}

func orDefault(v, def string) string {
	if v == "" {
		return def
	}
	return v
}

// googleTokenResponse is the token endpoint's answer, success or error.
type googleTokenResponse struct {
	AccessToken  string `json:"access_token"`
	ExpiresIn    int    `json:"expires_in"`
	RefreshToken string `json:"refresh_token"`
	Error        string `json:"error"`
	Description  string `json:"error_description"`
}

// postToken calls the token endpoint with the form. A 400 or 401 with
// invalid_grant comes back as errInvalidGrant.
func (c *GoogleCalendarClient) postToken(ctx context.Context, form url.Values) (googleTokenResponse, error) {
	form.Set("client_id", c.ClientID)
	form.Set("client_secret", c.ClientSecret)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, orDefault(c.TokenURL, GoogleTokenURL), strings.NewReader(form.Encode()))
	if err != nil {
		return googleTokenResponse{}, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	status, body, err := c.do(req)
	if err != nil {
		return googleTokenResponse{}, err
	}
	var tr googleTokenResponse
	_ = json.Unmarshal(body, &tr)
	if status < 200 || status > 299 {
		if tr.Error == "invalid_grant" {
			return tr, fmt.Errorf("%w (%s)", errInvalidGrant, tr.Description)
		}
		return tr, fmt.Errorf("google token endpoint: status %d: %s", status, body)
	}
	return tr, nil
}

var errInvalidGrant = errors.New("invalid_grant")

func (c *GoogleCalendarClient) do(req *http.Request) (int, []byte, error) {
	res, err := c.client().Do(req)
	if err != nil {
		return 0, nil, err
	}
	defer res.Body.Close()
	body, err := io.ReadAll(io.LimitReader(res.Body, 1<<20))
	if err != nil {
		return 0, nil, err
	}
	return res.StatusCode, body, nil
}

func (c *GoogleCalendarClient) remember(refreshToken string, tr googleTokenResponse) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.tokens == nil {
		c.tokens = map[string]googleAccessToken{}
	}
	// A minute early, so a token never expires mid-request.
	c.tokens[refreshToken] = googleAccessToken{
		token:   tr.AccessToken,
		expires: time.Now().Add(time.Duration(tr.ExpiresIn)*time.Second - time.Minute),
	}
}

// ExchangeCalendarCode trades the code for tokens at Google's token
// endpoint and returns the refresh token.
func (c *GoogleCalendarClient) ExchangeCalendarCode(ctx context.Context, code, redirectURI string) (string, error) {
	tr, err := c.postToken(ctx, url.Values{
		"grant_type":   {"authorization_code"},
		"code":         {code},
		"redirect_uri": {redirectURI},
	})
	if errors.Is(err, errInvalidGrant) || tr.Error == "redirect_uri_mismatch" {
		return "", fmt.Errorf("%w: %v", ErrGoogleCodeRejected, err)
	}
	if err != nil {
		return "", fmt.Errorf("google code exchange: %w", err)
	}
	if tr.RefreshToken == "" {
		// Only without access_type=offline and prompt=consent.
		return "", errors.New("google code exchange: no refresh token returned")
	}
	if tr.AccessToken != "" {
		c.remember(tr.RefreshToken, tr)
	}
	return tr.RefreshToken, nil
}

// accessToken returns a valid access token for the refresh token, from the
// cache unless force.
func (c *GoogleCalendarClient) accessToken(ctx context.Context, refreshToken string, force bool) (string, error) {
	if !force {
		c.mu.Lock()
		t, ok := c.tokens[refreshToken]
		c.mu.Unlock()
		if ok && time.Now().Before(t.expires) {
			return t.token, nil
		}
	}
	tr, err := c.postToken(ctx, url.Values{
		"grant_type":    {"refresh_token"},
		"refresh_token": {refreshToken},
	})
	if errors.Is(err, errInvalidGrant) {
		c.forget(refreshToken)
		return "", fmt.Errorf("%w: %v", ErrGoogleTokenRevoked, err)
	}
	if err != nil {
		return "", fmt.Errorf("google token refresh: %w", err)
	}
	c.remember(refreshToken, tr)
	return tr.AccessToken, nil
}

func (c *GoogleCalendarClient) forget(refreshToken string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.tokens, refreshToken)
}

// api calls the Calendar API with the refresh token's access token, and
// returns the status and body. A 401 is retried once with a fresh token; a
// second one means the token is refused.
func (c *GoogleCalendarClient) api(ctx context.Context, refreshToken, method, path string, payload any) (int, []byte, error) {
	var body []byte
	if payload != nil {
		var err error
		if body, err = json.Marshal(payload); err != nil {
			return 0, nil, err
		}
	}
	for attempt := range 2 {
		token, err := c.accessToken(ctx, refreshToken, attempt > 0)
		if err != nil {
			return 0, nil, err
		}
		req, err := http.NewRequestWithContext(ctx, method, orDefault(c.APIBase, GoogleCalendarAPIBase)+path, bytes.NewReader(body))
		if err != nil {
			return 0, nil, err
		}
		req.Header.Set("Authorization", "Bearer "+token)
		if payload != nil {
			req.Header.Set("Content-Type", "application/json")
		}
		status, resBody, err := c.do(req)
		if err != nil {
			return 0, nil, err
		}
		if status != http.StatusUnauthorized {
			return status, resBody, nil
		}
	}
	c.forget(refreshToken)
	return 0, nil, fmt.Errorf("%w: calendar API answered 401", ErrGoogleTokenRevoked)
}

func googleStatusError(what string, status int, body []byte) error {
	return fmt.Errorf("google %s: status %d: %s", what, status, body)
}

// CreateCalendar creates a secondary calendar (POST /calendars).
func (c *GoogleCalendarClient) CreateCalendar(ctx context.Context, refreshToken, name, timeZone string) (string, error) {
	status, body, err := c.api(ctx, refreshToken, http.MethodPost, "/calendars", map[string]string{
		"summary":  name,
		"timeZone": timeZone,
	})
	if err != nil {
		return "", err
	}
	if status < 200 || status > 299 {
		return "", googleStatusError("create calendar", status, body)
	}
	var cal struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(body, &cal); err != nil || cal.ID == "" {
		return "", fmt.Errorf("google create calendar: no id in %s", body)
	}
	return cal.ID, nil
}

type googleEventTime struct {
	Date     string `json:"date,omitempty"`
	DateTime string `json:"dateTime,omitempty"`
	TimeZone string `json:"timeZone,omitempty"`
}

type googleEvent struct {
	ID          string          `json:"id"`
	Summary     string          `json:"summary"`
	Description string          `json:"description"`
	Location    string          `json:"location"`
	Start       googleEventTime `json:"start"`
	End         googleEventTime `json:"end"`
	// Status "confirmed" also restores an event deleted earlier.
	Status string `json:"status"`
	Source *struct {
		Title string `json:"title"`
		URL   string `json:"url"`
	} `json:"source,omitempty"`
}

func toGoogleEvent(e CalendarEvent) googleEvent {
	g := googleEvent{
		ID: e.ID, Summary: e.Title, Description: e.Description, Location: e.Location, Status: "confirmed",
	}
	if e.AllDay {
		// Google's end date is exclusive, as is End.
		g.Start = googleEventTime{Date: e.Start.In(budapest).Format(time.DateOnly)}
		g.End = googleEventTime{Date: e.End.In(budapest).Format(time.DateOnly)}
	} else {
		g.Start = googleEventTime{DateTime: e.Start.In(budapest).Format(time.RFC3339), TimeZone: googleTimeZone}
		g.End = googleEventTime{DateTime: e.End.In(budapest).Format(time.RFC3339), TimeZone: googleTimeZone}
	}
	if e.Link != "" {
		g.Source = &struct {
			Title string `json:"title"`
			URL   string `json:"url"`
		}{"Squad Sync", e.Link}
	}
	return g
}

func eventsPath(calendarID string) string {
	return "/calendars/" + url.PathEscape(calendarID) + "/events"
}

// UpsertEvent replaces the event (PUT), or creates it with our id (POST)
// when it doesn't exist. A 404 on creating means the calendar is gone.
func (c *GoogleCalendarClient) UpsertEvent(ctx context.Context, refreshToken, calendarID string, event CalendarEvent) error {
	g := toGoogleEvent(event)
	put := func() (int, []byte, error) {
		return c.api(ctx, refreshToken, http.MethodPut, eventsPath(calendarID)+"/"+url.PathEscape(event.ID), g)
	}
	status, body, err := put()
	if err != nil {
		return err
	}
	if status >= 200 && status <= 299 {
		return nil
	}
	if status != http.StatusNotFound && status != http.StatusGone {
		return googleStatusError("update event", status, body)
	}
	status, body, err = c.api(ctx, refreshToken, http.MethodPost, eventsPath(calendarID), g)
	switch {
	case err != nil:
		return err
	case status >= 200 && status <= 299:
		return nil
	case status == http.StatusNotFound:
		return fmt.Errorf("%w: %s", ErrGoogleCalendarGone, body)
	case status == http.StatusConflict:
		// It exists after all (another writer, or deleted and kept): replace.
		status, body, err = put()
		if err != nil {
			return err
		}
		if status >= 200 && status <= 299 {
			return nil
		}
		return googleStatusError("update event", status, body)
	default:
		return googleStatusError("create event", status, body)
	}
}

// DeleteEvent deletes the event; 404 and 410 mean it's already gone.
func (c *GoogleCalendarClient) DeleteEvent(ctx context.Context, refreshToken, calendarID, eventID string) error {
	status, body, err := c.api(ctx, refreshToken, http.MethodDelete, eventsPath(calendarID)+"/"+url.PathEscape(eventID), nil)
	if err != nil {
		return err
	}
	if (status >= 200 && status <= 299) || status == http.StatusNotFound || status == http.StatusGone {
		return nil
	}
	return googleStatusError("delete event", status, body)
}

// RevokeCalendarToken revokes the refresh token (and with it the access
// tokens) at Google's revoke endpoint.
func (c *GoogleCalendarClient) RevokeCalendarToken(ctx context.Context, refreshToken string) error {
	c.forget(refreshToken)
	form := url.Values{"token": {refreshToken}}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, orDefault(c.RevokeURL, GoogleRevokeURL), strings.NewReader(form.Encode()))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	status, body, err := c.do(req)
	if err != nil {
		return err
	}
	if status < 200 || status > 299 {
		return googleStatusError("revoke", status, body)
	}
	return nil
}
