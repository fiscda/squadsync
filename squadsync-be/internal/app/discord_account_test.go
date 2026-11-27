package app_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"squadsync-be/internal/app"
	"squadsync-be/internal/apptest"
	"squadsync-be/internal/gateway"
)

// redirectURI is what Go builds from the harness's web URL.
const redirectURI = apptest.WebURL + "/fiok/discord/callback"

// firstLogin logs a new Felhasznalo in to the web with their main account.
func firstLogin(t *testing.T, h *apptest.Harness, id, discordID, name string) {
	t.Helper()
	if _, err := h.App.FirstLogin(context.Background(), app.FirstLoginInput{FelhasznaloID: id, DiscordID: discordID, Name: name}); err != nil {
		t.Fatalf("FirstLogin(%s): %v", id, err)
	}
}

// link runs the linking flow for felhasznaloID, as if the Discord account
// discordID had authorized the app.
func link(h *apptest.Harness, felhasznaloID, discordID, name string) (app.LinkResult, error) {
	code := h.DiscordOAuth.Authorize("code-for-"+discordID, gateway.DiscordIdentity{ID: discordID, Name: name})
	return h.App.LinkDiscordAccount(context.Background(), felhasznaloID, code)
}

func discordIDs(t *testing.T, h *apptest.Harness, felhasznaloID string) []string {
	t.Helper()
	accounts, err := h.App.DiscordAccounts(context.Background(), felhasznaloID)
	if err != nil {
		t.Fatalf("DiscordAccounts: %v", err)
	}
	var ids []string
	for _, a := range accounts {
		ids = append(ids, a.DiscordID)
	}
	return ids
}

func expectIDs(t *testing.T, what string, got []string, want ...string) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("%s = %v, want %v", what, got, want)
	}
	for i := range got {
		if got[i] != want[i] {
			t.Fatalf("%s = %v, want %v", what, got, want)
		}
	}
}

func TestLinkingASecondDiscordAccount(t *testing.T) {
	h := apptest.New(t)
	ctx := context.Background()
	firstLogin(t, h, "anna", "d-anna-pc", "Anna")
	h.Clock.Advance(time.Hour)

	res, err := link(h, "anna", "d-anna-phone", "Anna telefon")
	if err != nil {
		t.Fatalf("LinkDiscordAccount: %v", err)
	}
	if res.AlreadyLinked || res.RemovedFelhasznaloID != "" {
		t.Fatalf("result = %+v, want a plain new link", res)
	}
	want := app.LinkResult{}
	want.Account.DiscordID, want.Account.FelhasznaloID, want.Account.Name = "d-anna-phone", "anna", "Anna telefon"
	want.Account.CreatedAt = apptest.Start.Add(time.Hour)
	if res != want {
		t.Fatalf("result = %+v, want %+v", res, want)
	}

	// Go exchanged the code itself, with the redirect URI of the flow.
	ex := h.DiscordOAuth.Exchanges()
	if len(ex) != 1 || ex[0].Code != "code-for-d-anna-phone" || ex[0].RedirectURI != redirectURI {
		t.Fatalf("code exchanges = %+v, want one for the code with %s", ex, redirectURI)
	}

	f, err := h.App.FelhasznaloByDiscordID(ctx, "d-anna-phone")
	if err != nil || f.ID != "anna" {
		t.Fatalf("linked account resolves to %+v (err %v), want anna", f, err)
	}
	accounts, err := h.App.DiscordAccounts(ctx, "anna")
	if err != nil {
		t.Fatalf("DiscordAccounts: %v", err)
	}
	if len(accounts) != 2 || !accounts[0].IsMain || accounts[0].DiscordID != "d-anna-pc" || accounts[0].Name != "Anna" ||
		accounts[1].IsMain || accounts[1].DiscordID != "d-anna-phone" || accounts[1].Name != "Anna telefon" {
		t.Fatalf("DiscordAccounts = %+v, want main d-anna-pc (Anna), then linked d-anna-phone (Anna telefon)", accounts)
	}
}

func TestLinkingAnAccountOfAFelhasznaloWithATagIsRefused(t *testing.T) {
	h := apptest.New(t)
	ctx := context.Background()
	firstLogin(t, h, "anna", "d-anna", "Anna")
	firstLogin(t, h, "bela", "d-bela", "Béla")
	if _, err := h.App.CreateCsoport(ctx, "bela", "Béla csoportja"); err != nil {
		t.Fatalf("CreateCsoport: %v", err)
	}

	_, err := link(h, "anna", "d-bela", "Béla")
	if !errors.Is(err, app.ErrDiscordAccountInUse) {
		t.Fatalf("linking Béla's account: err = %v, want ErrDiscordAccountInUse", err)
	}
	expectIDs(t, "Anna's accounts", discordIDs(t, h, "anna"), "d-anna")
	expectIDs(t, "Béla's accounts", discordIDs(t, h, "bela"), "d-bela")
	if _, err := h.App.Felhasznalo(ctx, "bela"); err != nil {
		t.Fatalf("Béla after the refusal: %v, want him untouched", err)
	}
}

func TestLinkingAnotherFelhasznalosLinkedAccountIsRefused(t *testing.T) {
	h := apptest.New(t)
	firstLogin(t, h, "anna", "d-anna", "Anna")
	firstLogin(t, h, "cili", "d-cili", "Cili")
	if _, err := link(h, "cili", "d-cili-phone", "Cili telefon"); err != nil {
		t.Fatalf("Cili links her phone: %v", err)
	}

	// Cili has no Tag, but two accounts: she is not an empty Felhasznalo.
	if _, err := link(h, "anna", "d-cili-phone", "Cili telefon"); !errors.Is(err, app.ErrDiscordAccountInUse) {
		t.Fatalf("linking Cili's linked account: err = %v, want ErrDiscordAccountInUse", err)
	}
	if _, err := link(h, "anna", "d-cili", "Cili"); !errors.Is(err, app.ErrDiscordAccountInUse) {
		t.Fatalf("linking Cili's main account: err = %v, want ErrDiscordAccountInUse", err)
	}
	expectIDs(t, "Cili's accounts", discordIDs(t, h, "cili"), "d-cili", "d-cili-phone")
}

func TestLinkingTheAccountOfAnEmptyFelhasznaloTakesItOver(t *testing.T) {
	h := apptest.New(t)
	ctx := context.Background()
	firstLogin(t, h, "anna", "d-anna", "Anna")
	// Anna once logged in to the web with her phone account by mistake,
	// which made an empty Felhasznalo: no Tag, nothing else.
	firstLogin(t, h, "anna-phone-user", "d-anna-phone", "Anna telefon")

	res, err := link(h, "anna", "d-anna-phone", "Anna telefon")
	if err != nil {
		t.Fatalf("LinkDiscordAccount: %v", err)
	}
	if res.RemovedFelhasznaloID != "anna-phone-user" || res.Account.FelhasznaloID != "anna" || res.Account.IsMain {
		t.Fatalf("result = %+v, want a linked account of anna, taken from anna-phone-user", res)
	}
	if _, err := h.App.Felhasznalo(ctx, "anna-phone-user"); !errors.Is(err, app.ErrNotFound) {
		t.Fatalf("the empty Felhasznalo still exists: err = %v, want ErrNotFound", err)
	}
	expectIDs(t, "Anna's accounts", discordIDs(t, h, "anna"), "d-anna", "d-anna-phone")
	f, err := h.App.FelhasznaloByDiscordID(ctx, "d-anna-phone")
	if err != nil || f.ID != "anna" {
		t.Fatalf("phone account resolves to %+v (err %v), want anna", f, err)
	}
}

func TestLinkingAnAccountThatIsAlreadyMineChangesNothing(t *testing.T) {
	h := apptest.New(t)
	ctx := context.Background()
	firstLogin(t, h, "anna", "d-anna", "Anna")
	first, err := link(h, "anna", "d-anna-phone", "Anna telefon")
	if err != nil {
		t.Fatalf("first link: %v", err)
	}
	h.Clock.Advance(time.Hour)

	again, err := link(h, "anna", "d-anna-phone", "Új név")
	if err != nil {
		t.Fatalf("linking again: %v", err)
	}
	if !again.AlreadyLinked || again.Account != first.Account {
		t.Fatalf("linking again = %+v, want AlreadyLinked with the unchanged %+v", again, first.Account)
	}

	main, err := link(h, "anna", "d-anna", "Anna")
	if err != nil {
		t.Fatalf("linking the main account: %v", err)
	}
	if !main.AlreadyLinked || !main.Account.IsMain {
		t.Fatalf("linking the main account = %+v, want AlreadyLinked, still main", main)
	}
	accounts, err := h.App.DiscordAccounts(ctx, "anna")
	if err != nil {
		t.Fatalf("DiscordAccounts: %v", err)
	}
	if len(accounts) != 2 || !accounts[0].IsMain || accounts[1].IsMain || accounts[1].Name != "Anna telefon" {
		t.Fatalf("DiscordAccounts = %+v, want main d-anna and the unchanged d-anna-phone", accounts)
	}
}

func TestLinkingWithACodeDiscordRefuses(t *testing.T) {
	h := apptest.New(t)
	firstLogin(t, h, "anna", "d-anna", "Anna")

	_, err := h.App.LinkDiscordAccount(context.Background(), "anna", "stale-code")
	if !errors.Is(err, app.ErrDiscordCodeRejected) || !errors.Is(err, app.ErrInvalidInput) {
		t.Fatalf("err = %v, want ErrDiscordCodeRejected (an ErrInvalidInput)", err)
	}
	expectIDs(t, "Anna's accounts", discordIDs(t, h, "anna"), "d-anna")
}

func TestLinkingNeedsAKnownFelhasznaloAndInput(t *testing.T) {
	h := apptest.New(t)
	ctx := context.Background()
	code := h.DiscordOAuth.Authorize("c", gateway.DiscordIdentity{ID: "d-x", Name: "X"})

	if _, err := h.App.LinkDiscordAccount(ctx, "nobody", code); !errors.Is(err, app.ErrNotFound) {
		t.Fatalf("unknown Felhasznalo: err = %v, want ErrNotFound", err)
	}
	firstLogin(t, h, "anna", "d-anna", "Anna")
	if _, err := h.App.LinkDiscordAccount(ctx, "anna", " "); !errors.Is(err, app.ErrInvalidInput) {
		t.Fatalf("empty code: err = %v, want ErrInvalidInput", err)
	}
	if len(h.DiscordOAuth.Exchanges()) != 0 {
		t.Fatalf("Discord was asked anyway: %+v", h.DiscordOAuth.Exchanges())
	}
}

func TestUnlinkingADiscordAccount(t *testing.T) {
	h := apptest.New(t)
	ctx := context.Background()
	firstLogin(t, h, "anna", "d-anna", "Anna")
	firstLogin(t, h, "bela", "d-bela", "Béla")
	if _, err := link(h, "anna", "d-anna-phone", "Anna telefon"); err != nil {
		t.Fatalf("link: %v", err)
	}
	if _, err := link(h, "bela", "d-bela-phone", "Béla telefon"); err != nil {
		t.Fatalf("link: %v", err)
	}

	if err := h.App.UnlinkDiscordAccount(ctx, "anna", "d-anna"); !errors.Is(err, app.ErrMainDiscordAccount) {
		t.Fatalf("unlinking the main account: err = %v, want ErrMainDiscordAccount", err)
	}
	if err := h.App.UnlinkDiscordAccount(ctx, "anna", "d-bela-phone"); !errors.Is(err, app.ErrNotFound) {
		t.Fatalf("unlinking Béla's account: err = %v, want ErrNotFound", err)
	}
	if err := h.App.UnlinkDiscordAccount(ctx, "anna", "d-unknown"); !errors.Is(err, app.ErrNotFound) {
		t.Fatalf("unlinking an unknown account: err = %v, want ErrNotFound", err)
	}

	if err := h.App.UnlinkDiscordAccount(ctx, "anna", "d-anna-phone"); err != nil {
		t.Fatalf("UnlinkDiscordAccount: %v", err)
	}
	expectIDs(t, "Anna's accounts", discordIDs(t, h, "anna"), "d-anna")
	expectIDs(t, "Béla's accounts", discordIDs(t, h, "bela"), "d-bela", "d-bela-phone")
	// The account is unknown again: the bot will send it a login link.
	if _, err := h.App.FelhasznaloByDiscordID(ctx, "d-anna-phone"); !errors.Is(err, app.ErrNotFound) {
		t.Fatalf("unlinked account still resolves: err = %v, want ErrNotFound", err)
	}
}

func TestWebLoginIsRefusedForALinkedAccount(t *testing.T) {
	h := apptest.New(t)
	ctx := context.Background()
	firstLogin(t, h, "anna", "d-anna", "Anna")
	if _, err := link(h, "anna", "d-anna-phone", "Anna telefon"); err != nil {
		t.Fatalf("link: %v", err)
	}

	for _, discordID := range []string{"d-anna", "d-stranger"} {
		d, err := h.App.WebLogin(ctx, discordID)
		if err != nil || !d.Allowed || d.RefusalMessage() != "" {
			t.Fatalf("WebLogin(%s) = %+v, %v; want allowed", discordID, d, err)
		}
	}

	d, err := h.App.WebLogin(ctx, "d-anna-phone")
	if err != nil {
		t.Fatalf("WebLogin: %v", err)
	}
	if d.Allowed || d.MainAccountName != "Anna" {
		t.Fatalf("WebLogin(linked) = %+v, want refused, naming Anna", d)
	}
	if got, want := d.RefusalMessage(), "Ez a fiók Anna-hez van kapcsolva, lépj be azzal."; got != want {
		t.Fatalf("message = %q, want %q", got, want)
	}

	// Once unlinked, it may log in again (and becomes its own Felhasznalo).
	if err := h.App.UnlinkDiscordAccount(ctx, "anna", "d-anna-phone"); err != nil {
		t.Fatalf("UnlinkDiscordAccount: %v", err)
	}
	if d, err := h.App.WebLogin(ctx, "d-anna-phone"); err != nil || !d.Allowed {
		t.Fatalf("WebLogin after unlinking = %+v, %v; want allowed", d, err)
	}
}

func TestLinkingIsUnavailableWithoutAWebURL(t *testing.T) {
	h := apptest.New(t)
	firstLogin(t, h, "anna", "d-anna", "Anna")
	a := app.New(app.Deps{Store: h.Store, Clock: h.Clock, Discord: h.Discord, DiscordOAuth: h.DiscordOAuth})
	code := h.DiscordOAuth.Authorize("c", gateway.DiscordIdentity{ID: "d-x", Name: "X"})

	if _, err := a.LinkDiscordAccount(context.Background(), "anna", code); !errors.Is(err, gateway.ErrNotConfigured) {
		t.Fatalf("err = %v, want gateway.ErrNotConfigured", err)
	}
	if len(h.DiscordOAuth.Exchanges()) != 0 {
		t.Fatalf("Discord was asked anyway: %+v", h.DiscordOAuth.Exchanges())
	}
}

func TestDMsReachEveryLinkedDiscordAccount(t *testing.T) {
	h := apptest.New(t)
	c := csoportOf(t, h, "bela") // no Csatorna: the announcement goes by DM
	if _, err := link(h, "bela", "discord-bela-phone", "Béla telefon"); err != nil {
		t.Fatalf("Béla links his phone: %v", err)
	}

	e := propose(t, h, "anna", c.ID, 2)

	for _, discordID := range []string{"discord-bela", "discord-bela-phone"} {
		dms := h.Discord.DMsTo(discordID)
		if len(dms) != 1 || dms[0].Esemeny == nil || dms[0].Esemeny.EsemenyID != e.ID {
			t.Fatalf("DMs to %s = %+v, want the one announcement of %s", discordID, dms, e.ID)
		}
	}
}
