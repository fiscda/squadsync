package gateway

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// DiscordAPIBase is Discord's REST API root.
const DiscordAPIBase = "https://discord.com/api/v10"

// DiscordOAuthClient is the real DiscordOAuth. It uses the same Discord
// application as the web login (DISCORD_CLIENT_ID, DISCORD_CLIENT_SECRET).
type DiscordOAuthClient struct {
	ClientID     string
	ClientSecret string
	// BaseURL defaults to DiscordAPIBase.
	BaseURL string
	// HTTPClient defaults to a client with a 10-second timeout.
	HTTPClient *http.Client
}

var _ DiscordOAuth = (*DiscordOAuthClient)(nil)

// ExchangeCode trades the code for an access token at Discord's token
// endpoint, then reads the account with GET /users/@me. The token is used
// once and never stored.
func (c *DiscordOAuthClient) ExchangeCode(ctx context.Context, code, redirectURI string) (DiscordIdentity, error) {
	base := c.BaseURL
	if base == "" {
		base = DiscordAPIBase
	}
	client := c.HTTPClient
	if client == nil {
		client = &http.Client{Timeout: 10 * time.Second}
	}

	form := url.Values{
		"grant_type":   {"authorization_code"},
		"code":         {code},
		"redirect_uri": {redirectURI},
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, base+"/oauth2/token", strings.NewReader(form.Encode()))
	if err != nil {
		return DiscordIdentity{}, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.SetBasicAuth(url.QueryEscape(c.ClientID), url.QueryEscape(c.ClientSecret))
	var token struct {
		AccessToken string `json:"access_token"`
		TokenType   string `json:"token_type"`
	}
	if err := doJSON(client, req, &token); err != nil {
		return DiscordIdentity{}, fmt.Errorf("discord token exchange: %w", err)
	}
	if token.AccessToken == "" {
		return DiscordIdentity{}, fmt.Errorf("discord token exchange: %w", ErrDiscordCodeRejected)
	}

	req, err = http.NewRequestWithContext(ctx, http.MethodGet, base+"/users/@me", nil)
	if err != nil {
		return DiscordIdentity{}, err
	}
	req.Header.Set("Authorization", "Bearer "+token.AccessToken)
	var user struct {
		ID         string  `json:"id"`
		Username   string  `json:"username"`
		GlobalName *string `json:"global_name"`
	}
	if err := doJSON(client, req, &user); err != nil {
		return DiscordIdentity{}, fmt.Errorf("discord /users/@me: %w", err)
	}
	if user.ID == "" {
		return DiscordIdentity{}, fmt.Errorf("discord /users/@me returned no user id")
	}
	name := user.Username
	if user.GlobalName != nil && *user.GlobalName != "" {
		name = *user.GlobalName
	}
	return DiscordIdentity{ID: user.ID, Name: name}, nil
}

// doJSON sends req and decodes a 2xx JSON answer into v. A 400 or 401 means
// Discord refused the code or token: ErrDiscordCodeRejected.
func doJSON(client *http.Client, req *http.Request, v any) error {
	res, err := client.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	body, err := io.ReadAll(io.LimitReader(res.Body, 1<<20))
	if err != nil {
		return err
	}
	switch {
	case res.StatusCode == http.StatusBadRequest || res.StatusCode == http.StatusUnauthorized:
		return fmt.Errorf("%w (status %d: %s)", ErrDiscordCodeRejected, res.StatusCode, body)
	case res.StatusCode < 200 || res.StatusCode > 299:
		return fmt.Errorf("unexpected status %d: %s", res.StatusCode, body)
	}
	return json.Unmarshal(body, v)
}
