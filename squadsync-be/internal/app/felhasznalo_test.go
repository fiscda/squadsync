package app_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"squadsync-be/internal/app"
	"squadsync-be/internal/apptest"
)

func TestFirstLoginCreatesFelhasznaloWithMainDiscordAccount(t *testing.T) {
	h := apptest.New(t)
	ctx := context.Background()

	f, err := h.App.FirstLogin(ctx, app.FirstLoginInput{
		FelhasznaloID: "ba-user-anna",
		DiscordID:     "111",
		Name:          "Anna",
	})
	if err != nil {
		t.Fatalf("FirstLogin: %v", err)
	}
	if f.ID != "ba-user-anna" || f.Name != "Anna" || !f.CreatedAt.Equal(apptest.Start) {
		t.Fatalf("FirstLogin returned %+v, want id ba-user-anna, name Anna, created at %v", f, apptest.Start)
	}

	got, err := h.App.Felhasznalo(ctx, "ba-user-anna")
	if err != nil {
		t.Fatalf("Felhasznalo: %v", err)
	}
	if got != f {
		t.Fatalf("Felhasznalo = %+v, want %+v", got, f)
	}

	byDiscord, err := h.App.FelhasznaloByDiscordID(ctx, "111")
	if err != nil {
		t.Fatalf("FelhasznaloByDiscordID: %v", err)
	}
	if byDiscord.ID != "ba-user-anna" {
		t.Fatalf("Discord account 111 resolves to %q, want ba-user-anna", byDiscord.ID)
	}

	accounts, err := h.App.DiscordAccounts(ctx, "ba-user-anna")
	if err != nil {
		t.Fatalf("DiscordAccounts: %v", err)
	}
	if len(accounts) != 1 || accounts[0].DiscordID != "111" || !accounts[0].IsMain {
		t.Fatalf("DiscordAccounts = %+v, want exactly the main account 111", accounts)
	}
}

func TestSecondLoginReusesFelhasznalo(t *testing.T) {
	h := apptest.New(t)
	ctx := context.Background()
	in := app.FirstLoginInput{FelhasznaloID: "ba-user-bela", DiscordID: "222", Name: "Béla"}

	first, err := h.App.FirstLogin(ctx, in)
	if err != nil {
		t.Fatalf("first login: %v", err)
	}

	h.Clock.Advance(48 * time.Hour)
	in.Name = "Béla új néven"
	second, err := h.App.FirstLogin(ctx, in)
	if err != nil {
		t.Fatalf("second login: %v", err)
	}

	if second != first {
		t.Fatalf("second login returned %+v, want the existing Felhasznalo %+v", second, first)
	}
	accounts, err := h.App.DiscordAccounts(ctx, "ba-user-bela")
	if err != nil {
		t.Fatalf("DiscordAccounts: %v", err)
	}
	if len(accounts) != 1 {
		t.Fatalf("Felhasznalo has %d Discord accounts after two logins, want 1", len(accounts))
	}
}

func TestFirstLoginRefusesDiscordAccountOfAnotherFelhasznalo(t *testing.T) {
	h := apptest.New(t)
	ctx := context.Background()

	if _, err := h.App.FirstLogin(ctx, app.FirstLoginInput{FelhasznaloID: "ba-user-anna", DiscordID: "111", Name: "Anna"}); err != nil {
		t.Fatalf("first login: %v", err)
	}

	_, err := h.App.FirstLogin(ctx, app.FirstLoginInput{FelhasznaloID: "ba-user-other", DiscordID: "111", Name: "Anna"})
	if !errors.Is(err, app.ErrDiscordAccountInUse) {
		t.Fatalf("FirstLogin with a taken Discord account: err = %v, want ErrDiscordAccountInUse", err)
	}
	if _, err := h.App.Felhasznalo(ctx, "ba-user-other"); !errors.Is(err, app.ErrNotFound) {
		t.Fatalf("refused Felhasznalo exists anyway: err = %v, want ErrNotFound", err)
	}
}

func TestFirstLoginRequiresFelhasznaloAndDiscordID(t *testing.T) {
	h := apptest.New(t)
	ctx := context.Background()

	for _, in := range []app.FirstLoginInput{
		{FelhasznaloID: "", DiscordID: "111", Name: "Anna"},
		{FelhasznaloID: "ba-user-anna", DiscordID: " ", Name: "Anna"},
	} {
		if _, err := h.App.FirstLogin(ctx, in); !errors.Is(err, app.ErrInvalidInput) {
			t.Errorf("FirstLogin(%+v): err = %v, want ErrInvalidInput", in, err)
		}
	}
}

func TestUnknownDiscordAccountHasNoFelhasznalo(t *testing.T) {
	h := apptest.New(t)

	_, err := h.App.FelhasznaloByDiscordID(context.Background(), "999")
	if !errors.Is(err, app.ErrNotFound) {
		t.Fatalf("FelhasznaloByDiscordID(unknown): err = %v, want ErrNotFound", err)
	}
}
