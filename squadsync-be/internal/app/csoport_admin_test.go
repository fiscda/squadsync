package app_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"squadsync-be/internal/app"
	"squadsync-be/internal/apptest"
	"squadsync-be/internal/domain"
)

// admins lists the Felhasznalo ids of the Admin Tags of the Csoport.
func admins(t *testing.T, h *apptest.Harness, csoportID, viewerID string) []string {
	t.Helper()
	d, err := h.App.Csoport(context.Background(), viewerID, csoportID)
	if err != nil {
		t.Fatalf("Csoport: %v", err)
	}
	var out []string
	for _, tag := range d.Tags {
		if tag.Admin {
			out = append(out, tag.FelhasznaloID)
		}
	}
	return out
}

func isTag(t *testing.T, h *apptest.Harness, felhasznaloID, csoportID string) bool {
	t.Helper()
	_, err := h.App.Csoport(context.Background(), felhasznaloID, csoportID)
	if err != nil && !errors.Is(err, app.ErrNotFound) {
		t.Fatalf("Csoport: %v", err)
	}
	return err == nil
}

// insertEsemeny stores an Esemeny of the Szervezo in the given state.
func insertEsemeny(t *testing.T, h *apptest.Harness, csoportID, szervezoID string, allapot domain.Allapot) {
	t.Helper()
	err := h.Store.InsertEsemeny(context.Background(), domain.Esemeny{
		ID: "esemeny-" + string(allapot) + "-" + szervezoID, CsoportID: csoportID, Title: "Játék",
		SzervezoID: szervezoID, Allapot: allapot, CreatedAt: apptest.Start,
	})
	if err != nil {
		t.Fatalf("InsertEsemeny: %v", err)
	}
}

func TestOnlyAdminsRenameTheCsoportUnderTheCreationRules(t *testing.T) {
	h := apptest.New(t)
	ctx := context.Background()
	c := csoportOf(t, h, "bela")
	addFelhasznalo(t, h, "kivul", "Kívül")

	if _, err := h.App.RenameCsoport(ctx, "bela", c.ID, "Új név"); !errors.Is(err, app.ErrNotAdmin) {
		t.Fatalf("Tag renaming: err = %v, want ErrNotAdmin", err)
	}
	if _, err := h.App.RenameCsoport(ctx, "kivul", c.ID, "Új név"); !errors.Is(err, app.ErrNotFound) {
		t.Fatalf("non-Tag renaming: err = %v, want ErrNotFound", err)
	}
	for name, input := range map[string]string{
		"empty":    "  ",
		"too long": strings.Repeat("á", app.MaxCsoportNameLength+1),
	} {
		if _, err := h.App.RenameCsoport(ctx, "anna", c.ID, input); !errors.Is(err, app.ErrInvalidInput) {
			t.Fatalf("%s name: err = %v, want ErrInvalidInput", name, err)
		}
	}

	got, err := h.App.RenameCsoport(ctx, "anna", c.ID, "  Szombat esték ")
	if err != nil || got.Name != "Szombat esték" {
		t.Fatalf("RenameCsoport = %+v, %v, want the trimmed name", got, err)
	}
	d, err := h.App.Csoport(ctx, "bela", c.ID)
	if err != nil || d.Name != "Szombat esték" {
		t.Fatalf("Csoport = %+v, %v, want the new name", d, err)
	}
}

func TestOnlyAdminsGrantAndRevokeAdmin(t *testing.T) {
	h := apptest.New(t)
	ctx := context.Background()
	c := csoportOf(t, h, "bela", "cecil")
	addFelhasznalo(t, h, "kivul", "Kívül")

	if err := h.App.SetAdmin(ctx, "bela", c.ID, "cecil", true); !errors.Is(err, app.ErrNotAdmin) {
		t.Fatalf("Tag granting: err = %v, want ErrNotAdmin", err)
	}
	if err := h.App.SetAdmin(ctx, "kivul", c.ID, "cecil", true); !errors.Is(err, app.ErrNotFound) {
		t.Fatalf("non-Tag granting: err = %v, want ErrNotFound", err)
	}
	if err := h.App.SetAdmin(ctx, "anna", c.ID, "kivul", true); !errors.Is(err, app.ErrNotFound) {
		t.Fatalf("granting a non-Tag: err = %v, want ErrNotFound", err)
	}

	if err := h.App.SetAdmin(ctx, "anna", c.ID, "bela", true); err != nil {
		t.Fatalf("grant: %v", err)
	}
	if got := admins(t, h, c.ID, "anna"); len(got) != 2 {
		t.Fatalf("admins = %v, want anna and bela", got)
	}
	// Bela is an Admin now, and can act as one.
	if err := h.App.SetAdmin(ctx, "bela", c.ID, "cecil", true); err != nil {
		t.Fatalf("new Admin granting: %v", err)
	}
	if err := h.App.SetAdmin(ctx, "anna", c.ID, "bela", false); err != nil {
		t.Fatalf("revoke: %v", err)
	}
	d, _ := h.App.Csoport(ctx, "bela", c.ID)
	if d.Admin {
		t.Fatal("Bela is still Admin after the revoke")
	}
	// Granting twice is fine.
	if err := h.App.SetAdmin(ctx, "anna", c.ID, "cecil", true); err != nil {
		t.Fatalf("granting twice: %v", err)
	}
}

func TestTheLastAdminCantRevokeTheirOwnRoleUntilAnotherTagIsAdmin(t *testing.T) {
	h := apptest.New(t)
	ctx := context.Background()
	c := csoportOf(t, h, "bela")

	if err := h.App.SetAdmin(ctx, "anna", c.ID, "anna", false); !errors.Is(err, app.ErrLastAdmin) {
		t.Fatalf("last Admin revoking: err = %v, want ErrLastAdmin", err)
	}
	if got := admins(t, h, c.ID, "anna"); len(got) != 1 {
		t.Fatalf("admins = %v, want anna still", got)
	}

	if err := h.App.SetAdmin(ctx, "anna", c.ID, "bela", true); err != nil {
		t.Fatalf("grant: %v", err)
	}
	if err := h.App.SetAdmin(ctx, "anna", c.ID, "anna", false); err != nil {
		t.Fatalf("revoking with another Admin: %v", err)
	}
	// Now Bela is the last Admin.
	if err := h.App.SetAdmin(ctx, "bela", c.ID, "bela", false); !errors.Is(err, app.ErrLastAdmin) {
		t.Fatalf("new last Admin revoking: err = %v, want ErrLastAdmin", err)
	}
}

func TestOnlyAdminsRemoveTags(t *testing.T) {
	h := apptest.New(t)
	ctx := context.Background()
	c := csoportOf(t, h, "bela", "cecil")
	addFelhasznalo(t, h, "kivul", "Kívül")

	if err := h.App.RemoveTag(ctx, "bela", c.ID, "cecil"); !errors.Is(err, app.ErrNotAdmin) {
		t.Fatalf("Tag removing: err = %v, want ErrNotAdmin", err)
	}
	if err := h.App.RemoveTag(ctx, "kivul", c.ID, "cecil"); !errors.Is(err, app.ErrNotFound) {
		t.Fatalf("non-Tag removing: err = %v, want ErrNotFound", err)
	}
	if err := h.App.RemoveTag(ctx, "anna", c.ID, "kivul"); !errors.Is(err, app.ErrNotFound) {
		t.Fatalf("removing a non-Tag: err = %v, want ErrNotFound", err)
	}

	if err := h.App.RemoveTag(ctx, "anna", c.ID, "cecil"); err != nil {
		t.Fatalf("RemoveTag: %v", err)
	}
	if isTag(t, h, "cecil", c.ID) {
		t.Fatal("Cecil is still a Tag")
	}
	if !isTag(t, h, "bela", c.ID) {
		t.Fatal("Bela lost their Tag")
	}
	// The last Admin can't remove themselves either.
	if err := h.App.RemoveTag(ctx, "anna", c.ID, "anna"); !errors.Is(err, app.ErrLastAdmin) {
		t.Fatalf("last Admin removing themselves: err = %v, want ErrLastAdmin", err)
	}
}

func TestAnyTagLeavesButTheLastAdminMustHandOverFirst(t *testing.T) {
	h := apptest.New(t)
	ctx := context.Background()
	c := csoportOf(t, h, "bela")
	addFelhasznalo(t, h, "kivul", "Kívül")

	if err := h.App.LeaveCsoport(ctx, "kivul", c.ID); !errors.Is(err, app.ErrNotFound) {
		t.Fatalf("non-Tag leaving: err = %v, want ErrNotFound", err)
	}
	if err := h.App.LeaveCsoport(ctx, "anna", c.ID); !errors.Is(err, app.ErrLastAdmin) {
		t.Fatalf("last Admin leaving: err = %v, want ErrLastAdmin", err)
	}
	if !isTag(t, h, "anna", c.ID) {
		t.Fatal("Anna lost their Tag despite the refusal")
	}

	if err := h.App.LeaveCsoport(ctx, "bela", c.ID); err != nil {
		t.Fatalf("Tag leaving: %v", err)
	}
	if isTag(t, h, "bela", c.ID) {
		t.Fatal("Bela is still a Tag")
	}
	if cs, _ := h.App.CsoportokOf(ctx, "bela"); len(cs) != 0 {
		t.Fatalf("Bela's Csoportok = %+v, want none", cs)
	}
}

func TestAfterAnotherTagIsAdminTheLastAdminCanLeave(t *testing.T) {
	h := apptest.New(t)
	ctx := context.Background()
	c := csoportOf(t, h, "bela")
	if err := h.App.SetAdmin(ctx, "anna", c.ID, "bela", true); err != nil {
		t.Fatalf("grant: %v", err)
	}
	if err := h.App.LeaveCsoport(ctx, "anna", c.ID); err != nil {
		t.Fatalf("leaving: %v", err)
	}
	if isTag(t, h, "anna", c.ID) {
		t.Fatal("Anna is still a Tag")
	}
	if got := admins(t, h, c.ID, "bela"); len(got) != 1 {
		t.Fatalf("admins = %v, want bela", got)
	}
}

func TestWhenTheLastTagLeavesTheCsoportJustHasNoTags(t *testing.T) {
	h := apptest.New(t)
	ctx := context.Background()
	c := csoportOf(t, h, "bela")
	if err := h.App.LeaveCsoport(ctx, "bela", c.ID); err != nil {
		t.Fatalf("bela leaving: %v", err)
	}
	// Anna is the only Tag left, so there is nobody to hand the role to.
	if err := h.App.LeaveCsoport(ctx, "anna", c.ID); err != nil {
		t.Fatalf("the only Tag leaving: %v", err)
	}
	tags, err := h.Store.ListTags(ctx, c.ID)
	if err != nil || len(tags) != 0 {
		t.Fatalf("ListTags = %+v, %v, want none", tags, err)
	}
	if _, err := h.Store.FindCsoport(ctx, c.ID); err != nil {
		t.Fatalf("the Csoport itself should remain: %v", err)
	}
}

func TestALeavingTagKeepsTheirVisszajelzesButItNoLongerCounts(t *testing.T) {
	h := apptest.New(t)
	ctx := context.Background()
	c := csoportOf(t, h, "bela")
	e := propose(t, h, "anna", c.ID, 1)
	d := detail(t, h, "anna", e.ID)
	answer(t, h, "bela", d, 0, domain.Igen)

	if err := h.App.LeaveCsoport(ctx, "bela", c.ID); err != nil {
		t.Fatalf("leaving: %v", err)
	}

	a := detail(t, h, "anna", e.ID).Idopontok[0]
	if a.Igen != 0 || len(a.Visszajelzesek) != 1 || a.Visszajelzesek[0].IsTag {
		t.Fatalf("idopont = %+v, want Bela's igen kept but not counted", a)
	}
}

func TestASzervezoOfAnActiveEsemenyCantLeaveOrBeRemoved(t *testing.T) {
	for _, allapot := range []domain.Allapot{domain.Egyeztetes, domain.Veglegesitve} {
		t.Run(string(allapot), func(t *testing.T) {
			h := apptest.New(t)
			ctx := context.Background()
			c := csoportOf(t, h, "bela", "cecil")
			insertEsemeny(t, h, c.ID, "bela", allapot)

			if err := h.App.LeaveCsoport(ctx, "bela", c.ID); !errors.Is(err, app.ErrSzervezoOfActiveEsemeny) {
				t.Fatalf("leaving: err = %v, want ErrSzervezoOfActiveEsemeny", err)
			}
			if err := h.App.RemoveTag(ctx, "anna", c.ID, "bela"); !errors.Is(err, app.ErrSzervezoOfActiveEsemeny) {
				t.Fatalf("removing: err = %v, want ErrSzervezoOfActiveEsemeny", err)
			}
			if !isTag(t, h, "bela", c.ID) {
				t.Fatal("Bela lost their Tag despite the refusal")
			}
			// Someone who isn't the Szervezo is free to go.
			if err := h.App.LeaveCsoport(ctx, "cecil", c.ID); err != nil {
				t.Fatalf("other Tag leaving: %v", err)
			}
		})
	}
}

func TestASzervezoOfAnEsemenyThatIsOverCanLeave(t *testing.T) {
	for _, allapot := range []domain.Allapot{domain.Elmaradt, domain.Lezajlott} {
		t.Run(string(allapot), func(t *testing.T) {
			h := apptest.New(t)
			ctx := context.Background()
			c := csoportOf(t, h, "bela", "cecil")
			insertEsemeny(t, h, c.ID, "bela", allapot)
			insertEsemeny(t, h, c.ID, "cecil", allapot)

			if err := h.App.LeaveCsoport(ctx, "bela", c.ID); err != nil {
				t.Fatalf("leaving: %v", err)
			}
			if err := h.App.RemoveTag(ctx, "anna", c.ID, "cecil"); err != nil {
				t.Fatalf("removing: %v", err)
			}
		})
	}
}

func TestAnEsemenyOfAnotherCsoportDoesntBlockLeaving(t *testing.T) {
	h := apptest.New(t)
	ctx := context.Background()
	c := csoportOf(t, h, "bela")
	other, err := h.App.CreateCsoport(ctx, "bela", "Másik")
	if err != nil {
		t.Fatalf("CreateCsoport: %v", err)
	}
	insertEsemeny(t, h, other.ID, "bela", domain.Egyeztetes)

	if err := h.App.LeaveCsoport(ctx, "bela", c.ID); err != nil {
		t.Fatalf("leaving: %v", err)
	}
}

func TestMutingIsPerTagAndPerCsoport(t *testing.T) {
	h := apptest.New(t)
	ctx := context.Background()
	c := csoportOf(t, h, "bela")
	addFelhasznalo(t, h, "kivul", "Kívül")

	if err := h.App.SetMuted(ctx, "kivul", c.ID, true); !errors.Is(err, app.ErrNotFound) {
		t.Fatalf("non-Tag muting: err = %v, want ErrNotFound", err)
	}
	if err := h.App.SetMuted(ctx, "bela", c.ID, true); err != nil {
		t.Fatalf("SetMuted: %v", err)
	}
	if d, _ := h.App.Csoport(ctx, "bela", c.ID); !d.Muted {
		t.Fatal("Bela doesn't see their mute")
	}
	if d, _ := h.App.Csoport(ctx, "anna", c.ID); d.Muted {
		t.Fatal("Anna is muted too")
	}
	if err := h.App.SetMuted(ctx, "bela", c.ID, false); err != nil {
		t.Fatalf("unmute: %v", err)
	}
	if d, _ := h.App.Csoport(ctx, "bela", c.ID); d.Muted {
		t.Fatal("Bela is still muted")
	}
}

func TestAMutedTagGetsNoDMsFromTheCsoportButTheOthersDo(t *testing.T) {
	h := apptest.New(t)
	ctx := context.Background()
	c := csoportOf(t, h, "bela", "cecil")
	// No Csatorna, so the announcement goes by DM.
	other, err := h.App.CreateCsoport(ctx, "anna", "Másik")
	if err != nil {
		t.Fatalf("CreateCsoport: %v", err)
	}
	if err := h.App.SetMuted(ctx, "bela", c.ID, true); err != nil {
		t.Fatalf("SetMuted: %v", err)
	}

	propose(t, h, "anna", c.ID, 1)

	if dms := h.Discord.DMsTo("discord-bela"); len(dms) != 0 {
		t.Fatalf("muted Bela got DMs: %+v", dms)
	}
	for _, id := range []string{"discord-anna", "discord-cecil"} {
		if dms := h.Discord.DMsTo(id); len(dms) != 1 {
			t.Fatalf("DMs to %s = %+v, want one", id, dms)
		}
	}

	// Muting is per Csoport: Bela is in "Másik" too and is still told there.
	m, err := h.App.CreateCsoportMeghivo(ctx, "anna", other.ID)
	if err != nil {
		t.Fatalf("CreateCsoportMeghivo: %v", err)
	}
	if _, err := h.App.JoinCsoport(ctx, "bela", m.Token); err != nil {
		t.Fatalf("JoinCsoport: %v", err)
	}
	propose(t, h, "anna", other.ID, 1)
	if dms := h.Discord.DMsTo("discord-bela"); len(dms) != 1 {
		t.Fatalf("DMs to Bela from the other Csoport = %+v, want one", dms)
	}

	// Unmuting brings the DMs back.
	if err := h.App.SetMuted(ctx, "bela", c.ID, false); err != nil {
		t.Fatalf("unmute: %v", err)
	}
	propose(t, h, "anna", c.ID, 1)
	if dms := h.Discord.DMsTo("discord-bela"); len(dms) != 2 {
		t.Fatalf("DMs to Bela after unmuting = %+v, want two in total", dms)
	}
}

func TestMutingDoesntSilenceTheChannelPost(t *testing.T) {
	h := apptest.New(t)
	c := csoportOf(t, h, "bela")
	linkCsatorna(t, h, c.ID)
	if err := h.App.SetMuted(context.Background(), "bela", c.ID, true); err != nil {
		t.Fatalf("SetMuted: %v", err)
	}

	propose(t, h, "anna", c.ID, 1)

	if posts := h.Discord.Posts(); len(posts) != 1 {
		t.Fatalf("posts = %+v, want the channel post", posts)
	}
	if dms := h.Discord.DMsTo("discord-bela"); len(dms) != 0 {
		t.Fatalf("muted Bela got DMs: %+v", dms)
	}
}
