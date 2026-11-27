package httpapi_test

import (
	"io"
	"log/slog"
	"net/http"
	"testing"

	"squadsync-be/internal/apptest"
	"squadsync-be/internal/gateway"
	"squadsync-be/internal/httpapi"
)

type discordAccountResp struct {
	DiscordID string `json:"discordId"`
	Name      string `json:"name"`
	Main      bool   `json:"main"`
}

type linkResp struct {
	Account              discordAccountResp `json:"account"`
	AlreadyLinked        bool               `json:"alreadyLinked"`
	RemovedFelhasznaloID string             `json:"removedFelhasznaloId"`
}

type webLoginResp struct {
	Allowed bool   `json:"allowed"`
	Message string `json:"message"`
}

func linkRequest(code string) string {
	return `{"code":"` + code + `"}`
}

func TestLinkListAndUnlinkDiscordAccountsOverHTTP(t *testing.T) {
	h := apptest.New(t)
	api := httpapi.New(h.App, secret, slog.New(slog.NewTextHandler(io.Discard, nil)))
	login(t, api, "anna", "d-anna", "Anna")
	login(t, api, "bela", "d-bela", "Béla")
	expectStatus(t, call(t, api, http.MethodPost, "/csoport", `{"name":"Béla csoportja"}`, as("bela")), http.StatusCreated)
	login(t, api, "empty", "d-empty", "Üres")

	h.DiscordOAuth.Authorize("phone", gateway.DiscordIdentity{ID: "d-anna-phone", Name: "Anna telefon"})
	h.DiscordOAuth.Authorize("bela", gateway.DiscordIdentity{ID: "d-bela", Name: "Béla"})
	h.DiscordOAuth.Authorize("empty", gateway.DiscordIdentity{ID: "d-empty", Name: "Üres"})

	// Linking needs the caller.
	expectStatus(t, call(t, api, http.MethodPost, "/felhasznalo/discord-account", linkRequest("phone"), map[string]string{httpapi.HeaderSecret: secret}), http.StatusBadRequest)

	got := decode[linkResp](t, call(t, api, http.MethodPost, "/felhasznalo/discord-account", linkRequest("phone"), as("anna")))
	// Go used its own redirect URI, not one from the request.
	if ex := h.DiscordOAuth.Exchanges(); len(ex) != 1 || ex[0].RedirectURI != apptest.WebURL+"/fiok/discord/callback" {
		t.Fatalf("exchanges = %+v, want one with Go's redirect URI", ex)
	}
	if got.Account.DiscordID != "d-anna-phone" || got.Account.Main || got.AlreadyLinked || got.RemovedFelhasznaloID != "" {
		t.Fatalf("link = %+v, want a new linked account d-anna-phone", got)
	}
	again := decode[linkResp](t, call(t, api, http.MethodPost, "/felhasznalo/discord-account", linkRequest("phone"), as("anna")))
	if !again.AlreadyLinked {
		t.Fatalf("linking again = %+v, want alreadyLinked", again)
	}
	// Béla has a Tag: refused. A code Discord refuses: 400.
	expectStatus(t, call(t, api, http.MethodPost, "/felhasznalo/discord-account", linkRequest("bela"), as("anna")), http.StatusConflict)
	expectStatus(t, call(t, api, http.MethodPost, "/felhasznalo/discord-account", linkRequest("nope"), as("anna")), http.StatusBadRequest)
	expectStatus(t, call(t, api, http.MethodPost, "/felhasznalo/discord-account", `{`, as("anna")), http.StatusBadRequest)
	// The empty Felhasznalo is taken over, and Next.js learns whom to log out.
	taken := decode[linkResp](t, call(t, api, http.MethodPost, "/felhasznalo/discord-account", linkRequest("empty"), as("anna")))
	if taken.RemovedFelhasznaloID != "empty" {
		t.Fatalf("takeover = %+v, want removedFelhasznaloId empty", taken)
	}

	list := decode[[]discordAccountResp](t, call(t, api, http.MethodGet, "/felhasznalo/discord-account", "", as("anna")))
	if len(list) != 3 || !list[0].Main || list[0].DiscordID != "d-anna" || list[0].Name != "Anna" ||
		list[1].DiscordID != "d-anna-phone" || list[1].Main || list[2].DiscordID != "d-empty" {
		t.Fatalf("list = %+v, want main d-anna, then d-anna-phone and d-empty", list)
	}

	expectStatus(t, call(t, api, http.MethodDelete, "/felhasznalo/discord-account/d-anna", "", as("anna")), http.StatusConflict)
	expectStatus(t, call(t, api, http.MethodDelete, "/felhasznalo/discord-account/d-bela", "", as("anna")), http.StatusNotFound)
	expectStatus(t, call(t, api, http.MethodDelete, "/felhasznalo/discord-account/d-anna-phone", "", as("anna")), http.StatusNoContent)
	if list := decode[[]discordAccountResp](t, call(t, api, http.MethodGet, "/felhasznalo/discord-account", "", as("anna"))); len(list) != 2 {
		t.Fatalf("after unlinking: %+v, want 2 accounts", list)
	}
}

func TestLinkingWithoutDiscordConfiguredAnswers503(t *testing.T) {
	h := apptest.New(t)
	api := httpapi.New(h.App, secret, slog.New(slog.NewTextHandler(io.Discard, nil)))
	login(t, api, "anna", "d-anna", "Anna")
	h.DiscordOAuth.Fail(gateway.ErrNotConfigured)

	expectStatus(t, call(t, api, http.MethodPost, "/felhasznalo/discord-account", linkRequest("x"), as("anna")), http.StatusServiceUnavailable)
}

func TestWebLoginCheckOverHTTP(t *testing.T) {
	h := apptest.New(t)
	api := httpapi.New(h.App, secret, slog.New(slog.NewTextHandler(io.Discard, nil)))
	login(t, api, "anna", "d-anna", "Anna")
	h.DiscordOAuth.Authorize("phone", gateway.DiscordIdentity{ID: "d-anna-phone", Name: "Anna telefon"})
	expectStatus(t, call(t, api, http.MethodPost, "/felhasznalo/discord-account", linkRequest("phone"), as("anna")), http.StatusOK)

	// Only the secret is needed: no Felhasznalo is known during login.
	onlySecret := map[string]string{httpapi.HeaderSecret: secret}
	expectStatus(t, call(t, api, http.MethodGet, "/discord-account/d-anna-phone/web-login", "", map[string]string{}), http.StatusUnauthorized)

	refused := decode[webLoginResp](t, call(t, api, http.MethodGet, "/discord-account/d-anna-phone/web-login", "", onlySecret))
	if refused.Allowed || refused.Message != "Ez a fiók Anna-hez van kapcsolva, lépj be azzal." {
		t.Fatalf("linked account = %+v, want refused with the message naming Anna", refused)
	}
	for _, id := range []string{"d-anna", "d-new"} {
		if ok := decode[webLoginResp](t, call(t, api, http.MethodGet, "/discord-account/"+id+"/web-login", "", onlySecret)); !ok.Allowed || ok.Message != "" {
			t.Fatalf("%s = %+v, want allowed", id, ok)
		}
	}
}
