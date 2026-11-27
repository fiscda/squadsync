package app_test

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"squadsync-be/internal/app"
	"squadsync-be/internal/apptest"
	"squadsync-be/internal/domain"
)

// addFelhasznalo registers a Felhasznalo through the first login, with a
// Discord id derived from the id.
func addFelhasznalo(t *testing.T, h *apptest.Harness, id, name string) {
	t.Helper()
	if _, err := h.App.FirstLogin(context.Background(), app.FirstLoginInput{
		FelhasznaloID: id,
		DiscordID:     "discord-" + id,
		Name:          name,
	}); err != nil {
		t.Fatalf("FirstLogin(%s): %v", id, err)
	}
}

// csoportWithMeghivo has Anna create the Csoport "Péntek esték" and a
// Csoport-meghivo for it.
func csoportWithMeghivo(t *testing.T, h *apptest.Harness) (domain.Csoport, domain.CsoportMeghivo) {
	t.Helper()
	ctx := context.Background()
	addFelhasznalo(t, h, "anna", "Anna")
	c, err := h.App.CreateCsoport(ctx, "anna", "Péntek esték")
	if err != nil {
		t.Fatalf("CreateCsoport: %v", err)
	}
	m, err := h.App.CreateCsoportMeghivo(ctx, "anna", c.ID)
	if err != nil {
		t.Fatalf("CreateCsoportMeghivo: %v", err)
	}
	return c, m
}

func TestCreatorOfCsoportBecomesAdminTag(t *testing.T) {
	h := apptest.New(t)
	ctx := context.Background()
	addFelhasznalo(t, h, "anna", "Anna")

	c, err := h.App.CreateCsoport(ctx, "anna", "  Péntek esték  ")
	if err != nil {
		t.Fatalf("CreateCsoport: %v", err)
	}
	if c.Name != "Péntek esték" || !c.CreatedAt.Equal(apptest.Start) {
		t.Fatalf("CreateCsoport returned %+v, want the trimmed name created at %v", c, apptest.Start)
	}

	d, err := h.App.Csoport(ctx, "anna", c.ID)
	if err != nil {
		t.Fatalf("Csoport: %v", err)
	}
	if !d.Admin {
		t.Fatalf("creator is not Admin: %+v", d)
	}
	if len(d.Tags) != 1 || d.Tags[0].FelhasznaloID != "anna" || d.Tags[0].FelhasznaloName != "Anna" || !d.Tags[0].Admin {
		t.Fatalf("Tags = %+v, want only Anna as Admin", d.Tags)
	}
}

func TestCsoportNeedsAReasonableName(t *testing.T) {
	h := apptest.New(t)
	addFelhasznalo(t, h, "anna", "Anna")

	for name, input := range map[string]string{
		"empty":    "",
		"blank":    "   ",
		"too long": strings.Repeat("á", app.MaxCsoportNameLength+1),
	} {
		if _, err := h.App.CreateCsoport(context.Background(), "anna", input); !errors.Is(err, app.ErrInvalidInput) {
			t.Errorf("CreateCsoport(%s): err = %v, want ErrInvalidInput", name, err)
		}
	}
	if _, err := h.App.CreateCsoport(context.Background(), "anna", strings.Repeat("á", app.MaxCsoportNameLength)); err != nil {
		t.Errorf("CreateCsoport with the longest allowed name: %v", err)
	}
}

func TestUnknownFelhasznaloCannotCreateCsoport(t *testing.T) {
	h := apptest.New(t)
	if _, err := h.App.CreateCsoport(context.Background(), "nobody", "Csoport"); !errors.Is(err, app.ErrNotFound) {
		t.Fatalf("CreateCsoport by an unknown Felhasznalo: err = %v, want ErrNotFound", err)
	}
}

func TestJoiningThroughCsoportMeghivoMakesATag(t *testing.T) {
	h := apptest.New(t)
	ctx := context.Background()
	c, m := csoportWithMeghivo(t, h)
	addFelhasznalo(t, h, "bela", "Béla")
	h.Clock.Advance(time.Hour)

	res, err := h.App.JoinCsoport(ctx, "bela", m.Token)
	if err != nil {
		t.Fatalf("JoinCsoport: %v", err)
	}
	if !res.Joined || res.Csoport.ID != c.ID {
		t.Fatalf("JoinCsoport = %+v, want Joined in %s", res, c.ID)
	}

	d, err := h.App.Csoport(ctx, "bela", c.ID)
	if err != nil {
		t.Fatalf("Csoport as the new Tag: %v", err)
	}
	if d.Admin {
		t.Fatalf("a Tag who joined by link is Admin: %+v", d)
	}
	if len(d.Tags) != 2 || d.Tags[0].FelhasznaloID != "anna" || d.Tags[1].FelhasznaloID != "bela" || d.Tags[1].Admin {
		t.Fatalf("Tags = %+v, want Anna (Admin) then Béla", d.Tags)
	}
	if !d.Tags[1].CreatedAt.Equal(apptest.Start.Add(time.Hour)) {
		t.Fatalf("Béla's Tag created at %v, want one hour after the start", d.Tags[1].CreatedAt)
	}
}

func TestCsoportMeghivoIsReusableAndNeverExpires(t *testing.T) {
	h := apptest.New(t)
	ctx := context.Background()
	c, m := csoportWithMeghivo(t, h)
	addFelhasznalo(t, h, "bela", "Béla")
	addFelhasznalo(t, h, "cili", "Cili")

	if _, err := h.App.JoinCsoport(ctx, "bela", m.Token); err != nil {
		t.Fatalf("Béla joins: %v", err)
	}
	h.Clock.Advance(10 * 365 * 24 * time.Hour)
	if _, err := h.App.JoinCsoport(ctx, "cili", m.Token); err != nil {
		t.Fatalf("Cili joins ten years later with the same link: %v", err)
	}
	d, err := h.App.Csoport(ctx, "anna", c.ID)
	if err != nil {
		t.Fatalf("Csoport: %v", err)
	}
	if len(d.Tags) != 3 {
		t.Fatalf("Csoport has %d Tags, want 3", len(d.Tags))
	}
}

func TestJoiningAsExistingTagChangesNothing(t *testing.T) {
	h := apptest.New(t)
	ctx := context.Background()
	c, m := csoportWithMeghivo(t, h)
	addFelhasznalo(t, h, "bela", "Béla")
	if _, err := h.App.JoinCsoport(ctx, "bela", m.Token); err != nil {
		t.Fatalf("Béla joins: %v", err)
	}
	before, err := h.App.Csoport(ctx, "anna", c.ID)
	if err != nil {
		t.Fatalf("Csoport: %v", err)
	}

	h.Clock.Advance(time.Hour)
	// A plain Tag and the Admin both open the link again.
	for _, id := range []string{"bela", "anna"} {
		res, err := h.App.JoinCsoport(ctx, id, m.Token)
		if err != nil {
			t.Fatalf("%s opens the link again: %v", id, err)
		}
		if res.Joined || res.Csoport.ID != c.ID {
			t.Fatalf("%s re-joining returned %+v, want not Joined, in %s", id, res, c.ID)
		}
	}

	after, err := h.App.Csoport(ctx, "anna", c.ID)
	if err != nil {
		t.Fatalf("Csoport: %v", err)
	}
	if len(after.Tags) != len(before.Tags) {
		t.Fatalf("Tags changed: before %+v, after %+v", before.Tags, after.Tags)
	}
	for i := range before.Tags {
		if after.Tags[i] != before.Tags[i] {
			t.Fatalf("Tag %d changed: before %+v, after %+v", i, before.Tags[i], after.Tags[i])
		}
	}
	if !after.Tags[0].Admin || after.Tags[0].FelhasznaloID != "anna" {
		t.Fatalf("Anna lost her Admin role: %+v", after.Tags)
	}
}

func TestConcurrentJoinsCreateOneTag(t *testing.T) {
	h := apptest.New(t)
	ctx := context.Background()
	c, m := csoportWithMeghivo(t, h)
	addFelhasznalo(t, h, "bela", "Béla")

	const joins = 8
	var wg sync.WaitGroup
	errs := make([]error, joins)
	results := make([]app.JoinResult, joins)
	for i := range joins {
		wg.Add(1)
		go func() {
			defer wg.Done()
			results[i], errs[i] = h.App.JoinCsoport(ctx, "bela", m.Token)
		}()
	}
	wg.Wait()

	joined := 0
	for i := range joins {
		if errs[i] != nil {
			t.Fatalf("join %d: %v", i, errs[i])
		}
		if results[i].Joined {
			joined++
		}
	}
	if joined != 1 {
		t.Fatalf("%d of %d concurrent joins reported Joined, want exactly 1", joined, joins)
	}
	d, err := h.App.Csoport(ctx, "anna", c.ID)
	if err != nil {
		t.Fatalf("Csoport: %v", err)
	}
	if len(d.Tags) != 2 {
		t.Fatalf("Csoport has %d Tags, want 2", len(d.Tags))
	}
}

func TestRevokedCsoportMeghivoIsRefused(t *testing.T) {
	h := apptest.New(t)
	ctx := context.Background()
	c, m := csoportWithMeghivo(t, h)
	addFelhasznalo(t, h, "bela", "Béla")

	if err := h.App.RevokeCsoportMeghivo(ctx, "anna", c.ID, m.ID); err != nil {
		t.Fatalf("RevokeCsoportMeghivo: %v", err)
	}

	if _, err := h.App.JoinCsoport(ctx, "bela", m.Token); !errors.Is(err, app.ErrCsoportMeghivoRevoked) {
		t.Fatalf("JoinCsoport with a revoked link: err = %v, want ErrCsoportMeghivoRevoked", err)
	}
	if _, err := h.App.CsoportMeghivoPreview(ctx, m.Token); !errors.Is(err, app.ErrCsoportMeghivoRevoked) {
		t.Fatalf("CsoportMeghivoPreview of a revoked link: err = %v, want ErrCsoportMeghivoRevoked", err)
	}
	if _, err := h.App.Csoport(ctx, "bela", c.ID); !errors.Is(err, app.ErrNotFound) {
		t.Fatalf("Béla became a Tag through a revoked link: err = %v, want ErrNotFound", err)
	}

	// Revoking twice is harmless, and a new link still works.
	if err := h.App.RevokeCsoportMeghivo(ctx, "anna", c.ID, m.ID); err != nil {
		t.Fatalf("revoking again: %v", err)
	}
	fresh, err := h.App.CreateCsoportMeghivo(ctx, "anna", c.ID)
	if err != nil {
		t.Fatalf("CreateCsoportMeghivo: %v", err)
	}
	if _, err := h.App.JoinCsoport(ctx, "bela", fresh.Token); err != nil {
		t.Fatalf("JoinCsoport with a fresh link: %v", err)
	}
}

func TestRevokedLinkKeepsExistingTags(t *testing.T) {
	h := apptest.New(t)
	ctx := context.Background()
	c, m := csoportWithMeghivo(t, h)
	addFelhasznalo(t, h, "bela", "Béla")
	if _, err := h.App.JoinCsoport(ctx, "bela", m.Token); err != nil {
		t.Fatalf("Béla joins: %v", err)
	}

	if err := h.App.RevokeCsoportMeghivo(ctx, "bela", c.ID, m.ID); err != nil {
		t.Fatalf("a plain Tag revokes the link: %v", err)
	}
	if _, err := h.App.Csoport(ctx, "bela", c.ID); err != nil {
		t.Fatalf("Béla lost his Tag when the link was revoked: %v", err)
	}
}

func TestUnknownTokenIsNotFound(t *testing.T) {
	h := apptest.New(t)
	ctx := context.Background()
	addFelhasznalo(t, h, "bela", "Béla")

	for _, token := range []string{"", "no-such-token"} {
		if _, err := h.App.JoinCsoport(ctx, "bela", token); !errors.Is(err, app.ErrNotFound) {
			t.Errorf("JoinCsoport(%q): err = %v, want ErrNotFound", token, err)
		}
		if _, err := h.App.CsoportMeghivoPreview(ctx, token); !errors.Is(err, app.ErrNotFound) {
			t.Errorf("CsoportMeghivoPreview(%q): err = %v, want ErrNotFound", token, err)
		}
	}
}

func TestPreviewShowsTheCsoportBehindTheLink(t *testing.T) {
	h := apptest.New(t)
	c, m := csoportWithMeghivo(t, h)

	got, err := h.App.CsoportMeghivoPreview(context.Background(), m.Token)
	if err != nil {
		t.Fatalf("CsoportMeghivoPreview: %v", err)
	}
	if got.ID != c.ID || got.Name != "Péntek esték" {
		t.Fatalf("CsoportMeghivoPreview = %+v, want %+v", got, c)
	}
}

func TestCsoportMeghivoTokensAreLongAndDistinct(t *testing.T) {
	h := apptest.New(t)
	ctx := context.Background()
	c, first := csoportWithMeghivo(t, h)
	second, err := h.App.CreateCsoportMeghivo(ctx, "anna", c.ID)
	if err != nil {
		t.Fatalf("CreateCsoportMeghivo: %v", err)
	}
	// 32 random bytes are 43 characters of unpadded base64.
	if len(first.Token) < 43 || len(second.Token) < 43 || first.Token == second.Token || first.ID == second.ID {
		t.Fatalf("tokens %q and %q are too short or not distinct", first.Token, second.Token)
	}
}

func TestNonTagsCannotSeeOrManageACsoport(t *testing.T) {
	h := apptest.New(t)
	ctx := context.Background()
	c, m := csoportWithMeghivo(t, h)
	addFelhasznalo(t, h, "dani", "Dani")

	if _, err := h.App.Csoport(ctx, "dani", c.ID); !errors.Is(err, app.ErrNotFound) {
		t.Errorf("Csoport as a non-Tag: err = %v, want ErrNotFound", err)
	}
	if _, err := h.App.CsoportMeghivok(ctx, "dani", c.ID); !errors.Is(err, app.ErrNotFound) {
		t.Errorf("CsoportMeghivok as a non-Tag: err = %v, want ErrNotFound", err)
	}
	if _, err := h.App.CreateCsoportMeghivo(ctx, "dani", c.ID); !errors.Is(err, app.ErrNotFound) {
		t.Errorf("CreateCsoportMeghivo as a non-Tag: err = %v, want ErrNotFound", err)
	}
	if err := h.App.RevokeCsoportMeghivo(ctx, "dani", c.ID, m.ID); !errors.Is(err, app.ErrNotFound) {
		t.Errorf("RevokeCsoportMeghivo as a non-Tag: err = %v, want ErrNotFound", err)
	}

	// The refusal is the same whether the Csoport exists or not.
	if _, err := h.App.Csoport(ctx, "dani", "no-such-csoport"); !errors.Is(err, app.ErrNotFound) {
		t.Errorf("Csoport that doesn't exist: err = %v, want ErrNotFound", err)
	}

	// The link must still work: the failed revoke changed nothing.
	if _, err := h.App.JoinCsoport(ctx, "dani", m.Token); err != nil {
		t.Errorf("link no longer works after a refused revoke: %v", err)
	}
}

func TestMeghivoOfAnotherCsoportCannotBeRevokedThroughMine(t *testing.T) {
	h := apptest.New(t)
	ctx := context.Background()
	_, annasMeghivo := csoportWithMeghivo(t, h)
	addFelhasznalo(t, h, "dani", "Dani")
	other, err := h.App.CreateCsoport(ctx, "dani", "Danié")
	if err != nil {
		t.Fatalf("CreateCsoport: %v", err)
	}

	if err := h.App.RevokeCsoportMeghivo(ctx, "dani", other.ID, annasMeghivo.ID); !errors.Is(err, app.ErrNotFound) {
		t.Fatalf("revoking Anna's link through Dani's Csoport: err = %v, want ErrNotFound", err)
	}
	if _, err := h.App.CsoportMeghivoPreview(ctx, annasMeghivo.Token); err != nil {
		t.Fatalf("Anna's link stopped working: %v", err)
	}
}

func TestCsoportMeghivokListsOnlyWorkingLinks(t *testing.T) {
	h := apptest.New(t)
	ctx := context.Background()
	c, first := csoportWithMeghivo(t, h)
	h.Clock.Advance(time.Minute)
	second, err := h.App.CreateCsoportMeghivo(ctx, "anna", c.ID)
	if err != nil {
		t.Fatalf("CreateCsoportMeghivo: %v", err)
	}
	third, err := h.App.CreateCsoportMeghivo(ctx, "anna", c.ID)
	if err != nil {
		t.Fatalf("CreateCsoportMeghivo: %v", err)
	}
	if err := h.App.RevokeCsoportMeghivo(ctx, "anna", c.ID, third.ID); err != nil {
		t.Fatalf("RevokeCsoportMeghivo: %v", err)
	}

	list, err := h.App.CsoportMeghivok(ctx, "anna", c.ID)
	if err != nil {
		t.Fatalf("CsoportMeghivok: %v", err)
	}
	if len(list) != 2 || list[0].ID != second.ID || list[1].ID != first.ID {
		t.Fatalf("CsoportMeghivok = %+v, want the second then the first link", list)
	}
	if list[0].Token != second.Token {
		t.Fatalf("listed token %q, want %q", list[0].Token, second.Token)
	}
}

func TestAnyTagCanCreateCsoportMeghivo(t *testing.T) {
	h := apptest.New(t)
	ctx := context.Background()
	c, m := csoportWithMeghivo(t, h)
	addFelhasznalo(t, h, "bela", "Béla")
	if _, err := h.App.JoinCsoport(ctx, "bela", m.Token); err != nil {
		t.Fatalf("Béla joins: %v", err)
	}

	belas, err := h.App.CreateCsoportMeghivo(ctx, "bela", c.ID)
	if err != nil {
		t.Fatalf("a plain Tag creates a link: %v", err)
	}
	addFelhasznalo(t, h, "cili", "Cili")
	if res, err := h.App.JoinCsoport(ctx, "cili", belas.Token); err != nil || !res.Joined {
		t.Fatalf("Cili joins through Béla's link: %+v, %v", res, err)
	}
}

func TestMyCsoportokListsEveryCsoportOfATag(t *testing.T) {
	h := apptest.New(t)
	ctx := context.Background()
	addFelhasznalo(t, h, "anna", "Anna")
	addFelhasznalo(t, h, "bela", "Béla")

	zold, err := h.App.CreateCsoport(ctx, "anna", "Zöld")
	if err != nil {
		t.Fatalf("CreateCsoport: %v", err)
	}
	h.Clock.Advance(time.Minute)
	abc, err := h.App.CreateCsoport(ctx, "bela", "Ábécé")
	if err != nil {
		t.Fatalf("CreateCsoport: %v", err)
	}
	if _, err := h.App.CreateCsoport(ctx, "bela", "Csak Béláé"); err != nil {
		t.Fatalf("CreateCsoport: %v", err)
	}
	m, err := h.App.CreateCsoportMeghivo(ctx, "bela", abc.ID)
	if err != nil {
		t.Fatalf("CreateCsoportMeghivo: %v", err)
	}
	h.Clock.Advance(time.Minute)
	if _, err := h.App.JoinCsoport(ctx, "anna", m.Token); err != nil {
		t.Fatalf("Anna joins: %v", err)
	}

	list, err := h.App.CsoportokOf(ctx, "anna")
	if err != nil {
		t.Fatalf("CsoportokOf: %v", err)
	}
	if len(list) != 2 {
		t.Fatalf("Anna sees %d Csoportok, want 2: %+v", len(list), list)
	}
	if list[0].ID != zold.ID || !list[0].Admin || list[0].TagCount != 1 {
		t.Errorf("first = %+v, want Zöld, Admin, 1 Tag", list[0])
	}
	if list[1].ID != abc.ID || list[1].Admin || list[1].TagCount != 2 {
		t.Errorf("second = %+v, want Ábécé, not Admin, 2 Tags", list[1])
	}

	none, err := h.App.CsoportokOf(ctx, "nobody")
	if err != nil || len(none) != 0 {
		t.Fatalf("CsoportokOf(nobody) = %+v, %v, want an empty list", none, err)
	}
}

func TestJoinByUnknownFelhasznaloIsNotFound(t *testing.T) {
	h := apptest.New(t)
	_, m := csoportWithMeghivo(t, h)
	if _, err := h.App.JoinCsoport(context.Background(), "nobody", m.Token); !errors.Is(err, app.ErrNotFound) {
		t.Fatalf("JoinCsoport by an unknown Felhasznalo: err = %v, want ErrNotFound", err)
	}
}
