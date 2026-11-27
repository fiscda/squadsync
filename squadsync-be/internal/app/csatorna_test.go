package app_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"squadsync-be/internal/app"
	"squadsync-be/internal/apptest"
	"squadsync-be/internal/domain"
)

// csoportWithTag has Anna create "Péntek esték" as Admin and Bela join it as
// a plain Tag.
func csoportWithTag(t *testing.T, h *apptest.Harness) domain.Csoport {
	t.Helper()
	c, m := csoportWithMeghivo(t, h)
	addFelhasznalo(t, h, "bela", "Béla")
	if _, err := h.App.JoinCsoport(context.Background(), "bela", m.Token); err != nil {
		t.Fatalf("JoinCsoport: %v", err)
	}
	return c
}

func csatornaOf(t *testing.T, h *apptest.Harness, felhasznaloID, csoportID string) *domain.Csatorna {
	t.Helper()
	d, err := h.App.Csoport(context.Background(), felhasznaloID, csoportID)
	if err != nil {
		t.Fatalf("Csoport: %v", err)
	}
	return d.Csatorna
}

func TestKnownDiscordAccountResolvesToItsFelhasznalo(t *testing.T) {
	h := apptest.New(t)
	addFelhasznalo(t, h, "anna", "Anna")

	f, err := h.App.FelhasznaloByDiscordID(context.Background(), "discord-anna")
	if err != nil {
		t.Fatalf("FelhasznaloByDiscordID: %v", err)
	}
	if f.ID != "anna" || f.Name != "Anna" {
		t.Fatalf("resolved %+v, want Anna", f)
	}
	if _, err := h.App.FelhasznaloByDiscordID(context.Background(), "discord-stranger"); !errors.Is(err, app.ErrNotFound) {
		t.Fatalf("unknown account: err = %v, want ErrNotFound", err)
	}
}

func TestAdminLinksAChannelToTheCsoport(t *testing.T) {
	h := apptest.New(t)
	ctx := context.Background()
	c := csoportWithTag(t, h)

	if got := csatornaOf(t, h, "anna", c.ID); got != nil {
		t.Fatalf("new Csoport has Csatorna %+v, want none", got)
	}

	linked, err := h.App.LinkCsatorna(ctx, "anna", app.LinkCsatornaInput{
		CsoportID: c.ID, GuildID: "guild-1", ChannelID: "chan-1", ChannelName: " altalanos ",
	})
	if err != nil {
		t.Fatalf("LinkCsatorna: %v", err)
	}
	want := domain.Csatorna{
		CsoportID: c.ID, GuildID: "guild-1", ChannelID: "chan-1", ChannelName: "altalanos",
		LinkedBy: "anna", LinkedAt: apptest.Start,
	}
	if linked != want {
		t.Fatalf("LinkCsatorna = %+v, want %+v", linked, want)
	}
	// Every Tag sees the linked channel, not only Admins.
	if got := csatornaOf(t, h, "bela", c.ID); got == nil || *got != want {
		t.Fatalf("Csatorna seen by a Tag = %+v, want %+v", got, want)
	}
}

func TestLinkingAgainReplacesTheChannel(t *testing.T) {
	h := apptest.New(t)
	ctx := context.Background()
	c := csoportWithTag(t, h)

	if _, err := h.App.LinkCsatorna(ctx, "anna", app.LinkCsatornaInput{CsoportID: c.ID, GuildID: "guild-1", ChannelID: "chan-1"}); err != nil {
		t.Fatalf("first LinkCsatorna: %v", err)
	}
	h.Clock.Advance(time.Hour)
	if _, err := h.App.LinkCsatorna(ctx, "anna", app.LinkCsatornaInput{CsoportID: c.ID, GuildID: "guild-2", ChannelID: "chan-2", ChannelName: "jatek"}); err != nil {
		t.Fatalf("second LinkCsatorna: %v", err)
	}

	got := csatornaOf(t, h, "anna", c.ID)
	if got == nil || got.GuildID != "guild-2" || got.ChannelID != "chan-2" || got.ChannelName != "jatek" || !got.LinkedAt.Equal(apptest.Start.Add(time.Hour)) {
		t.Fatalf("Csatorna = %+v, want only the second channel", got)
	}
}

func TestOnlyAdminsLinkAndUnlinkChannels(t *testing.T) {
	h := apptest.New(t)
	ctx := context.Background()
	c := csoportWithTag(t, h)
	addFelhasznalo(t, h, "cecil", "Cecil")
	in := app.LinkCsatornaInput{CsoportID: c.ID, GuildID: "guild-1", ChannelID: "chan-1"}

	if _, err := h.App.LinkCsatorna(ctx, "bela", in); !errors.Is(err, app.ErrNotAdmin) {
		t.Fatalf("Tag links: err = %v, want ErrNotAdmin", err)
	}
	if _, err := h.App.LinkCsatorna(ctx, "cecil", in); !errors.Is(err, app.ErrNotFound) {
		t.Fatalf("non-Tag links: err = %v, want ErrNotFound", err)
	}
	if got := csatornaOf(t, h, "anna", c.ID); got != nil {
		t.Fatalf("refused links left Csatorna %+v", got)
	}

	if _, err := h.App.LinkCsatorna(ctx, "anna", in); err != nil {
		t.Fatalf("Admin links: %v", err)
	}
	if err := h.App.UnlinkCsatorna(ctx, "bela", c.ID); !errors.Is(err, app.ErrNotAdmin) {
		t.Fatalf("Tag unlinks: err = %v, want ErrNotAdmin", err)
	}
	if err := h.App.UnlinkCsatorna(ctx, "cecil", c.ID); !errors.Is(err, app.ErrNotFound) {
		t.Fatalf("non-Tag unlinks: err = %v, want ErrNotFound", err)
	}
	if got := csatornaOf(t, h, "anna", c.ID); got == nil {
		t.Fatal("refused unlinks removed the Csatorna")
	}
}

func TestAdminUnlinksTheChannel(t *testing.T) {
	h := apptest.New(t)
	ctx := context.Background()
	c := csoportWithTag(t, h)

	if _, err := h.App.LinkCsatorna(ctx, "anna", app.LinkCsatornaInput{CsoportID: c.ID, GuildID: "guild-1", ChannelID: "chan-1"}); err != nil {
		t.Fatalf("LinkCsatorna: %v", err)
	}
	if err := h.App.UnlinkCsatorna(ctx, "anna", c.ID); err != nil {
		t.Fatalf("UnlinkCsatorna: %v", err)
	}
	if got := csatornaOf(t, h, "anna", c.ID); got != nil {
		t.Fatalf("after unlinking Csatorna = %+v, want none", got)
	}
	if err := h.App.UnlinkCsatorna(ctx, "anna", c.ID); err != nil {
		t.Fatalf("unlinking again: %v", err)
	}
}

func TestLinkingNeedsAGuildAndAChannel(t *testing.T) {
	h := apptest.New(t)
	c := csoportWithTag(t, h)

	for name, in := range map[string]app.LinkCsatornaInput{
		"no guild":   {CsoportID: c.ID, ChannelID: "chan-1"},
		"no channel": {CsoportID: c.ID, GuildID: "guild-1", ChannelID: "  "},
	} {
		if _, err := h.App.LinkCsatorna(context.Background(), "anna", in); !errors.Is(err, app.ErrInvalidInput) {
			t.Errorf("%s: err = %v, want ErrInvalidInput", name, err)
		}
	}
}

func TestAdminCsoportokListsOnlyCsoportsWhereTheCallerIsAdmin(t *testing.T) {
	h := apptest.New(t)
	ctx := context.Background()
	tagOnly := csoportWithTag(t, h) // Anna is Admin, Bela a plain Tag
	own, err := h.App.CreateCsoport(ctx, "bela", "Béla csoportja")
	if err != nil {
		t.Fatalf("CreateCsoport: %v", err)
	}

	got, err := h.App.AdminCsoportokOf(ctx, "bela")
	if err != nil {
		t.Fatalf("AdminCsoportokOf: %v", err)
	}
	if len(got) != 1 || got[0].ID != own.ID {
		t.Fatalf("AdminCsoportokOf(bela) = %+v, want only %q (not %q)", got, own.ID, tagOnly.ID)
	}

	got, err = h.App.AdminCsoportokOf(ctx, "anna")
	if err != nil {
		t.Fatalf("AdminCsoportokOf: %v", err)
	}
	if len(got) != 1 || got[0].ID != tagOnly.ID {
		t.Fatalf("AdminCsoportokOf(anna) = %+v, want only %q", got, tagOnly.ID)
	}
}
