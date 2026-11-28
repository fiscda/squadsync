package app_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"squadsync-be/internal/app"
	"squadsync-be/internal/apptest"
	"squadsync-be/internal/domain"
)

func edited(title string) app.EsemenyDetailsInput {
	return app.EsemenyDetailsInput{
		Title:       title,
		Description: "Új leírás",
		Helyszin:    domain.Helyszin{Name: "Új hely", URL: "https://maps.app.goo.gl/new"},
	}
}

func TestSzervezoEditsTheDetailsInEveryStateButElmaradt(t *testing.T) {
	h := apptest.New(t)
	ctx := context.Background()
	c := csoportOf(t, h, "bela")
	e := propose(t, h, "anna", c.ID, 2)

	if err := h.App.EditEsemeny(ctx, "anna", e.ID, edited("  Új cím  ")); err != nil {
		t.Fatalf("EditEsemeny in Egyeztetes: %v", err)
	}
	d := detail(t, h, "bela", e.ID)
	if d.Title != "Új cím" || d.Description != "Új leírás" ||
		d.Helyszin != (domain.Helyszin{Name: "Új hely", URL: "https://maps.app.goo.gl/new"}) {
		t.Fatalf("after editing: %+v", d.Esemeny)
	}

	answer(t, h, "bela", d, 0, domain.Igen)
	confirm(t, h, "anna", d, 0)
	if err := h.App.EditEsemeny(ctx, "anna", e.ID, edited("Véglegesítve is")); err != nil {
		t.Fatalf("EditEsemeny in Veglegesitve: %v", err)
	}
	// The confirmed time is untouched.
	after := detail(t, h, "anna", e.ID)
	if after.Title != "Véglegesítve is" || after.ConfirmedIdopont != d.Idopontok[0].ID ||
		!after.Idopontok[0].Start.Equal(d.Idopontok[0].Start) {
		t.Fatalf("after editing in Veglegesitve: %+v", after)
	}

	// Lezajlott is set by a timed job; set it directly here.
	if err := h.Store.UpdateEsemenyAllapot(ctx, e.ID, domain.Lezajlott, d.Idopontok[0].ID); err != nil {
		t.Fatal(err)
	}
	if err := h.App.EditEsemeny(ctx, "anna", e.ID, edited("Lezajlott is")); err != nil {
		t.Fatalf("EditEsemeny in Lezajlott: %v", err)
	}

	f := propose(t, h, "anna", c.ID, 1)
	if err := h.App.CancelEsemeny(ctx, "anna", f.ID); err != nil {
		t.Fatalf("CancelEsemeny: %v", err)
	}
	if err := h.App.EditEsemeny(ctx, "anna", f.ID, edited("Elmaradt")); !errors.Is(err, app.ErrEsemenyElmaradt) {
		t.Fatalf("EditEsemeny in Elmaradt: err = %v, want ErrEsemenyElmaradt", err)
	}
	if got := detail(t, h, "anna", f.ID).Title; got != "Társasjáték" {
		t.Fatalf("title = %q, want it unchanged", got)
	}
}

func TestOnlyTheSzervezoEditsAndTheSameValidationAppliesAsProposing(t *testing.T) {
	h := apptest.New(t)
	ctx := context.Background()
	c := csoportOf(t, h, "bela")
	addFelhasznalo(t, h, "dani", "Dani")
	e := propose(t, h, "anna", c.ID, 1)

	if err := h.App.EditEsemeny(ctx, "bela", e.ID, edited("Bélané")); !errors.Is(err, app.ErrNotSzervezo) {
		t.Fatalf("a Tag editing: err = %v, want ErrNotSzervezo", err)
	}
	if err := h.App.EditEsemeny(ctx, "dani", e.ID, edited("Dani")); !errors.Is(err, app.ErrNotFound) {
		t.Fatalf("a non-Tag editing: err = %v, want ErrNotFound", err)
	}
	for name, in := range map[string]app.EsemenyDetailsInput{
		"no title":         {Title: "  "},
		"long title":       {Title: strings.Repeat("a", app.MaxEsemenyTitleLength+1)},
		"long description": {Title: "X", Description: strings.Repeat("a", app.MaxEsemenyDescriptionLength+1)},
		"bad URL":          {Title: "X", Helyszin: domain.Helyszin{URL: "javascript:alert(1)"}},
	} {
		if err := h.App.EditEsemeny(ctx, "anna", e.ID, in); !errors.Is(err, app.ErrInvalidInput) {
			t.Errorf("%s: err = %v, want ErrInvalidInput", name, err)
		}
	}
	if got := detail(t, h, "anna", e.ID).Title; got != "Társasjáték" {
		t.Fatalf("title = %q, want it unchanged", got)
	}
}

func TestEditingReRendersTheChannelMessageAndEveryDMCopy(t *testing.T) {
	h := apptest.New(t)
	ctx := context.Background()
	c := csoportOf(t, h, "bela")
	e := propose(t, h, "anna", c.ID, 2) // no Csatorna: a DM copy for each Tag
	refs, err := h.Store.ListEsemenyDiscordMessages(ctx, e.ID)
	if err != nil || len(refs) != 2 {
		t.Fatalf("messages = %+v, err %v, want one DM copy each", refs, err)
	}

	if err := h.App.EditEsemeny(ctx, "anna", e.ID, edited("Új cím")); err != nil {
		t.Fatal(err)
	}
	edits := h.Discord.Edits()
	if len(edits) != 2 {
		t.Fatalf("edits = %+v, want both copies edited in place", edits)
	}
	for _, ed := range edits {
		m := ed.Message
		if m.Title != "Új cím" || m.Description != "Új leírás" || m.HelyszinName != "Új hely" || m.HelyszinURL != "https://maps.app.goo.gl/new" {
			t.Fatalf("edit = %+v, want the new details", m)
		}
	}
	if len(h.Discord.Posts()) != 0 || len(h.Discord.DMs()) != 2 {
		t.Fatal("editing must edit in place, not send new messages")
	}
}

func TestAnyTagAddsAnIdopontOnlyDuringEgyeztetes(t *testing.T) {
	h := apptest.New(t)
	ctx := context.Background()
	c := csoportOf(t, h, "bela")
	addFelhasznalo(t, h, "dani", "Dani")
	e := propose(t, h, "anna", c.ID, 2)
	linkCsatorna(t, h, c.ID)
	refs, _ := h.Store.ListEsemenyDiscordMessages(ctx, e.ID)
	editsBefore := len(h.Discord.Edits())

	i, err := h.App.AddIdopont(ctx, "bela", e.ID, app.IdopontInput{Start: "2026-12-20T18:00", End: "2026-12-20T21:00"})
	if err != nil {
		t.Fatalf("AddIdopont: %v", err)
	}
	if i.AddedBy != "bela" {
		t.Fatalf("added by %q, want bela", i.AddedBy)
	}
	d := detail(t, h, "anna", e.ID)
	if len(d.Idopontok) != 3 || d.Idopontok[2].ID != i.ID || d.Idopontok[2].AddedBy != "bela" {
		t.Fatalf("idopontok = %+v, want Béla's added last", d.Idopontok)
	}
	// 18:00 in winter Budapest is 17:00 UTC.
	if !d.Idopontok[2].Start.Equal(time.Date(2026, 12, 20, 17, 0, 0, 0, time.UTC)) {
		t.Fatalf("start = %v", d.Idopontok[2].Start)
	}
	if len(refs) > 0 && len(h.Discord.Edits()) == editsBefore {
		t.Fatal("the Discord message wasn't re-rendered")
	}

	if _, err := h.App.AddIdopont(ctx, "dani", e.ID, app.IdopontInput{Start: "2026-12-21T18:00"}); !errors.Is(err, app.ErrNotFound) {
		t.Fatalf("a non-Tag adding: err = %v, want ErrNotFound", err)
	}

	answer(t, h, "bela", d, 0, domain.Igen)
	confirm(t, h, "anna", d, 0)
	if _, err := h.App.AddIdopont(ctx, "bela", e.ID, app.IdopontInput{Start: "2026-12-22T18:00"}); !errors.Is(err, app.ErrNotEgyeztetes) {
		t.Fatalf("adding in Veglegesitve: err = %v, want ErrNotEgyeztetes", err)
	}
	reopen(t, h, "anna", e.ID)
	if _, err := h.App.AddIdopont(ctx, "bela", e.ID, app.IdopontInput{Start: "2026-12-22T18:00"}); err != nil {
		t.Fatalf("adding after reopening: %v", err)
	}
	if err := h.App.CancelEsemeny(ctx, "anna", e.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := h.App.AddIdopont(ctx, "bela", e.ID, app.IdopontInput{Start: "2026-12-23T18:00"}); !errors.Is(err, app.ErrNotEgyeztetes) {
		t.Fatalf("adding in Elmaradt: err = %v, want ErrNotEgyeztetes", err)
	}
}

func TestAddingAnIdopontIsValidatedLikeProposing(t *testing.T) {
	h := apptest.New(t)
	ctx := context.Background()
	c := csoportOf(t, h, "bela")
	e := propose(t, h, "anna", c.ID, 1)

	for name, tc := range map[string]struct {
		in   app.IdopontInput
		want error
	}{
		"malformed":          {app.IdopontInput{Start: "holnap"}, app.ErrInvalidInput},
		"end before start":   {app.IdopontInput{Start: "2026-12-20T18:00", End: "2026-12-20T17:00"}, app.ErrInvalidInput},
		"skipped DST hour":   {app.IdopontInput{Start: "2026-03-29T02:30"}, app.ErrNonexistentLocalTime},
		"last day too early": {app.IdopontInput{Start: "2026-12-20", End: "2026-12-19", EgeszNapos: true}, app.ErrInvalidInput},
	} {
		if _, err := h.App.AddIdopont(ctx, "bela", e.ID, tc.in); !errors.Is(err, tc.want) {
			t.Errorf("%s: err = %v, want %v", name, err, tc.want)
		}
	}

	i, err := h.App.AddIdopont(ctx, "bela", e.ID, app.IdopontInput{Start: "2026-12-20", End: "2026-12-22", EgeszNapos: true})
	if err != nil {
		t.Fatalf("whole-day AddIdopont: %v", err)
	}
	// Budapest midnight of the first day, and midnight after the last.
	if !i.EgeszNapos || !i.Start.Equal(time.Date(2026, 12, 19, 23, 0, 0, 0, time.UTC)) ||
		i.End == nil || !i.End.Equal(time.Date(2026, 12, 22, 23, 0, 0, 0, time.UTC)) {
		t.Fatalf("whole-day idopont = %+v", i)
	}
}

func TestTheNumberOfIdopontokIsCapped(t *testing.T) {
	h := apptest.New(t)
	ctx := context.Background()
	c := csoportOf(t, h, "bela")
	e, err := h.App.ProposeEsemeny(ctx, "anna", c.ID, app.EsemenyInput{Title: "X", Idopontok: idopontok(app.MaxIdopontok - 1)})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := h.App.AddIdopont(ctx, "bela", e.ID, app.IdopontInput{Start: "2027-01-01T18:00"}); err != nil {
		t.Fatalf("adding the last one that fits: %v", err)
	}
	if _, err := h.App.AddIdopont(ctx, "bela", e.ID, app.IdopontInput{Start: "2027-01-02T18:00"}); !errors.Is(err, app.ErrTooManyIdopontok) {
		t.Fatalf("adding past the cap: err = %v, want ErrTooManyIdopontok", err)
	}
	if got := len(detail(t, h, "anna", e.ID).Idopontok); got != app.MaxIdopontok {
		t.Fatalf("%d idopontok, want %d", got, app.MaxIdopontok)
	}
}

func TestSzervezoRemovesAnyIdopontWithItsVisszajelzesek(t *testing.T) {
	h := apptest.New(t)
	ctx := context.Background()
	c := csoportOf(t, h, "bela")
	e := propose(t, h, "anna", c.ID, 3)
	d := detail(t, h, "anna", e.ID)
	answer(t, h, "bela", d, 1, domain.Igen)
	answer(t, h, "anna", d, 1, domain.Talan)
	answer(t, h, "bela", d, 2, domain.Igen)

	if err := h.App.RemoveIdopont(ctx, "anna", e.ID, d.Idopontok[1].ID); err != nil {
		t.Fatalf("RemoveIdopont: %v", err)
	}
	after := detail(t, h, "anna", e.ID)
	if len(after.Idopontok) != 2 || after.Idopontok[0].ID != d.Idopontok[0].ID || after.Idopontok[1].ID != d.Idopontok[2].ID {
		t.Fatalf("idopontok = %+v, want the first and third", after.Idopontok)
	}
	// Its Visszajelzesek went with it; the others keep theirs.
	if after.Idopontok[1].Igen != 1 {
		t.Fatalf("third idopont = %+v, want Béla's igen kept", after.Idopontok[1])
	}
	vs, err := h.Store.ListVisszajelzesekOfEsemeny(ctx, e.ID)
	if err != nil || len(vs) != 1 {
		t.Fatalf("visszajelzesek = %+v, err %v, want only the one on the third idopont", vs, err)
	}
	expectHelyezesek(t, after, 0, 1)
}

func TestTheAdderRemovesTheirIdopontOnlyWhileNobodyHasAnsweredIt(t *testing.T) {
	h := apptest.New(t)
	ctx := context.Background()
	c := csoportOf(t, h, "bela", "cecil")
	e := propose(t, h, "anna", c.ID, 1)
	mine := func(start string) string {
		t.Helper()
		i, err := h.App.AddIdopont(ctx, "bela", e.ID, app.IdopontInput{Start: start})
		if err != nil {
			t.Fatal(err)
		}
		return i.ID
	}
	unanswered, answered := mine("2026-12-20T18:00"), mine("2026-12-21T18:00")
	d := detail(t, h, "anna", e.ID)
	answer(t, h, "cecil", d, 2, domain.Nem) // on the second added one

	if err := h.App.RemoveIdopont(ctx, "bela", e.ID, answered); !errors.Is(err, app.ErrIdopontHasAnswers) {
		t.Fatalf("removing an answered idopont: err = %v, want ErrIdopontHasAnswers", err)
	}
	if err := h.App.RemoveIdopont(ctx, "cecil", e.ID, unanswered); !errors.Is(err, app.ErrNotPermitted) {
		t.Fatalf("another Tag removing it: err = %v, want ErrNotPermitted", err)
	}
	if err := h.App.RemoveIdopont(ctx, "bela", e.ID, unanswered); err != nil {
		t.Fatalf("removing an unanswered idopont: %v", err)
	}
	if got := len(detail(t, h, "anna", e.ID).Idopontok); got != 2 {
		t.Fatalf("%d idopontok, want 2", got)
	}

	// The Removable flag follows the same rules for each viewer.
	removable := func(viewer string) []bool {
		var out []bool
		for _, i := range detail(t, h, viewer, e.ID).Idopontok {
			out = append(out, i.Removable)
		}
		return out
	}
	for viewer, want := range map[string][]bool{
		"anna":  {true, true},
		"bela":  {false, false},
		"cecil": {false, false},
	} {
		got := removable(viewer)
		if len(got) != 2 || got[0] != want[0] || got[1] != want[1] {
			t.Errorf("removable for %s = %v, want %v", viewer, got, want)
		}
	}
}

func TestTheLastIdopontAndOtherStatesRefuseRemoval(t *testing.T) {
	h := apptest.New(t)
	ctx := context.Background()
	c := csoportOf(t, h, "bela")
	addFelhasznalo(t, h, "dani", "Dani")
	e := propose(t, h, "anna", c.ID, 2)
	d := detail(t, h, "anna", e.ID)

	if err := h.App.RemoveIdopont(ctx, "dani", e.ID, d.Idopontok[0].ID); !errors.Is(err, app.ErrNotFound) {
		t.Fatalf("a non-Tag removing: err = %v, want ErrNotFound", err)
	}
	other := propose(t, h, "anna", c.ID, 1)
	if err := h.App.RemoveIdopont(ctx, "anna", e.ID, detail(t, h, "anna", other.ID).Idopontok[0].ID); !errors.Is(err, app.ErrNotFound) {
		t.Fatalf("an idopont of another esemeny: err = %v, want ErrNotFound", err)
	}

	if err := h.App.RemoveIdopont(ctx, "anna", e.ID, d.Idopontok[1].ID); err != nil {
		t.Fatal(err)
	}
	if err := h.App.RemoveIdopont(ctx, "anna", e.ID, d.Idopontok[0].ID); !errors.Is(err, app.ErrLastIdopont) {
		t.Fatalf("removing the last idopont: err = %v, want ErrLastIdopont", err)
	}
	if i := detail(t, h, "anna", e.ID).Idopontok[0]; i.Removable {
		t.Fatal("the last idopont is marked removable")
	}

	if _, err := h.App.AddIdopont(ctx, "anna", e.ID, app.IdopontInput{Start: "2026-12-30T18:00"}); err != nil {
		t.Fatal(err)
	}
	b, err := h.App.AddIdopont(ctx, "bela", e.ID, app.IdopontInput{Start: "2026-12-31T18:00"})
	if err != nil {
		t.Fatal(err)
	}
	belas := b.ID
	d = detail(t, h, "anna", e.ID)
	confirm(t, h, "anna", d, 0)
	if err := h.App.RemoveIdopont(ctx, "anna", e.ID, d.Idopontok[1].ID); !errors.Is(err, app.ErrNotEgyeztetes) {
		t.Fatalf("removing in Veglegesitve: err = %v, want ErrNotEgyeztetes", err)
	}
	// A Tag who may never remove it gets the permission error, whatever the state.
	if err := h.App.RemoveIdopont(ctx, "bela", e.ID, d.Idopontok[1].ID); !errors.Is(err, app.ErrNotPermitted) {
		t.Fatalf("another Tag removing in Veglegesitve: err = %v, want ErrNotPermitted", err)
	}
	// The adder is refused by the state.
	if err := h.App.RemoveIdopont(ctx, "bela", e.ID, belas); !errors.Is(err, app.ErrNotEgyeztetes) {
		t.Fatalf("the adder removing in Veglegesitve: err = %v, want ErrNotEgyeztetes", err)
	}
}

func TestRemovingAnIdopontReRendersTheDiscordMessage(t *testing.T) {
	h := apptest.New(t)
	ctx := context.Background()
	c := csoportOf(t, h, "bela")
	linkCsatorna(t, h, c.ID)
	e := propose(t, h, "anna", c.ID, 7) // too many for buttons
	d := detail(t, h, "anna", e.ID)

	for i := 0; i < 2; i++ {
		if err := h.App.RemoveIdopont(ctx, "anna", e.ID, d.Idopontok[i].ID); err != nil {
			t.Fatal(err)
		}
	}
	edits := h.Discord.Edits()
	if len(edits) != 2 {
		t.Fatalf("edits = %d, want one per removal", len(edits))
	}
	first, last := edits[0].Message, edits[1].Message
	// Still more than 5: a web link instead of buttons.
	if len(first.Idopontok) != 6 || !first.WebButton {
		t.Fatalf("after the first removal: %d idopontok, web button %v, want 6 with the web link", len(first.Idopontok), first.WebButton)
	}
	if len(last.Idopontok) != 5 || last.WebButton || !last.Idopontok[0].Answerable {
		t.Fatalf("after the second removal: %d idopontok, web button %v, want 5 with buttons", len(last.Idopontok), last.WebButton)
	}
}

func TestCopyingShiftsEveryIdopontByOneBudapestWeekAcrossDST(t *testing.T) {
	h := apptest.New(t)
	ctx := context.Background()
	c := csoportOf(t, h, "bela")
	e, err := h.App.ProposeEsemeny(ctx, "anna", c.ID, app.EsemenyInput{
		Title:       "Társasjáték",
		Description: "Catan",
		Helyszin:    domain.Helyszin{Name: "Nálam", URL: "https://maps.app.goo.gl/abc"},
		Idopontok: []app.IdopontInput{
			// Spring forward is on 2026-03-29, autumn back on 2026-10-25.
			{Start: "2026-03-28T20:00", End: "2026-03-28T23:00"},
			{Start: "2026-10-24T20:00"},
			{Start: "2026-03-25", End: "2026-03-28", EgeszNapos: true},
			{Start: "2026-10-22", EgeszNapos: true},
			{Start: "2026-06-10T09:30", End: "2026-06-12T18:00"},
		},
		Hatarido: "2026-03-20T12:00",
	})
	if err != nil {
		t.Fatal(err)
	}

	cp, err := h.App.CopyEsemeny(ctx, "bela", e.ID)
	if err != nil {
		t.Fatalf("CopyEsemeny: %v", err)
	}
	if cp.ID == e.ID || cp.CsoportID != c.ID || cp.SzervezoID != "bela" || cp.Allapot != domain.Egyeztetes {
		t.Fatalf("copy = %+v, want a new Esemeny of the Csoport in Egyeztetes with Béla as Szervezo", cp)
	}
	d := detail(t, h, "anna", cp.ID)
	if d.Title != "Társasjáték" || d.Description != "Catan" ||
		d.Helyszin != (domain.Helyszin{Name: "Nálam", URL: "https://maps.app.goo.gl/abc"}) || d.SzervezoName != "bela" {
		t.Fatalf("copied details = %+v", d)
	}
	if d.Hatarido != nil {
		t.Fatalf("hatarido = %v, want it not copied", d.Hatarido)
	}

	type span struct{ start, end string }
	show := func(t time.Time) string { return t.In(app.Budapest).Format(app.LocalTimeLayout) }
	var got []span
	for _, i := range d.Idopontok {
		s := span{start: show(i.Start)}
		if i.End != nil {
			s.end = show(*i.End)
		}
		if i.AddedBy != "bela" {
			t.Errorf("idopont added by %q, want the copier", i.AddedBy)
		}
		got = append(got, s)
	}
	want := []span{
		// Same Budapest wall clock readings, a week later.
		{"2026-04-01T00:00", "2026-04-05T00:00"}, // whole days 03-25..03-28 -> 04-01..04-04
		{"2026-04-04T20:00", "2026-04-04T23:00"}, // 03-28 (CET) -> 04-04 (CEST)
		{"2026-06-17T09:30", "2026-06-19T18:00"},
		{"2026-10-29T00:00", "2026-10-30T00:00"}, // whole day 10-22 -> 10-29
		{"2026-10-31T20:00", ""},                 // 10-24 (CEST) -> 10-31 (CET)
	}
	if len(got) != len(want) {
		t.Fatalf("copied idopontok = %v, want %v", got, want)
	}
	for n := range want {
		if got[n] != want[n] {
			t.Errorf("idopont %d = %v, want %v", n+1, got[n], want[n])
		}
	}

	// Not simply 7*24 hours: the clocks changed in between (the evening of
	// 03-28 is 19:00 UTC, a week later 20:00 is 18:00 UTC).
	if want := time.Date(2026, 4, 4, 18, 0, 0, 0, time.UTC); !d.Idopontok[1].Start.Equal(want) {
		t.Fatalf("spring copy starts at %v, want %v", d.Idopontok[1].Start, want)
	}
	for _, i := range d.Idopontok {
		if i.EgeszNapos && (i.End == nil || !strings.HasSuffix(show(i.Start), "T00:00")) {
			t.Errorf("whole-day idopont %+v lost its midnights", i)
		}
	}
}

func TestAnyTagCopiesInAnyStateAndNonTagsGetNotFound(t *testing.T) {
	h := apptest.New(t)
	ctx := context.Background()
	c := csoportOf(t, h, "bela")
	addFelhasznalo(t, h, "dani", "Dani")
	e := propose(t, h, "anna", c.ID, 2)
	d := detail(t, h, "anna", e.ID)
	answer(t, h, "bela", d, 0, domain.Igen)

	if _, err := h.App.CopyEsemeny(ctx, "dani", e.ID); !errors.Is(err, app.ErrNotFound) {
		t.Fatalf("a non-Tag copying: err = %v, want ErrNotFound", err)
	}
	if _, err := h.App.CopyEsemeny(ctx, "anna", "no-such-esemeny"); !errors.Is(err, app.ErrNotFound) {
		t.Fatalf("copying a missing esemeny: err = %v, want ErrNotFound", err)
	}

	confirm(t, h, "anna", d, 0)
	if _, err := h.App.CopyEsemeny(ctx, "bela", e.ID); err != nil {
		t.Fatalf("copying a Veglegesitve esemeny: %v", err)
	}
	if err := h.App.CancelEsemeny(ctx, "anna", e.ID); err != nil {
		t.Fatal(err)
	}
	cp, err := h.App.CopyEsemeny(ctx, "bela", e.ID)
	if err != nil {
		t.Fatalf("copying an Elmaradt esemeny: %v", err)
	}
	// The source is untouched and the copy has no answers or confirmed time.
	cd := detail(t, h, "bela", cp.ID)
	if cd.Allapot != domain.Egyeztetes || cd.ConfirmedIdopont != "" || cd.Idopontok[0].Igen != 0 || len(cd.Resztvevok) != 0 {
		t.Fatalf("copy = %+v", cd)
	}
	if got := detail(t, h, "anna", e.ID); got.Allapot != domain.Elmaradt || len(got.Idopontok) != 2 {
		t.Fatalf("source = %+v, want it unchanged", got)
	}
}

func TestCopyingKeepsTheEnabledKiegeszitokButNotTheirData(t *testing.T) {
	h := apptest.New(t)
	ctx := context.Background()
	c := csoportOf(t, h, "bela")
	e := propose(t, h, "anna", c.ID, 1)
	switchKiegeszito(t, h, "anna", e.ID, domain.Felszereles, true)
	addTetel(t, h, "anna", e.ID, "sátor", 3)

	cp, err := h.App.CopyEsemeny(ctx, "bela", e.ID)
	if err != nil {
		t.Fatal(err)
	}
	on := func(d app.EsemenyDetail, kind domain.KiegeszitoKind) bool {
		for _, k := range d.Kiegeszitok {
			if k.Kind == kind {
				return k.Enabled
			}
		}
		return false
	}
	d := detail(t, h, "bela", cp.ID)
	if !on(d, domain.Felszereles) {
		t.Fatalf("kiegeszitok = %+v, want Felszereles on in the copy", d.Kiegeszitok)
	}
	if f := felszereles(t, h, "bela", cp.ID); len(f.Tetelek) != 0 {
		t.Fatalf("tetelek = %+v, want none copied", f.Tetelek)
	}

	// Switched off in the source: not copied on.
	switchKiegeszito(t, h, "anna", e.ID, domain.Felszereles, false)
	cp2, err := h.App.CopyEsemeny(ctx, "bela", e.ID)
	if err != nil {
		t.Fatal(err)
	}
	if on(detail(t, h, "bela", cp2.ID), domain.Felszereles) {
		t.Fatal("a switched-off Kiegeszito was switched on in the copy")
	}
}

func TestACopyIsAnnouncedLikeAProposedEsemeny(t *testing.T) {
	h := apptest.New(t)
	ctx := context.Background()
	c := csoportOf(t, h, "bela", "cecil")
	linkCsatorna(t, h, c.ID)
	e := propose(t, h, "anna", c.ID, 2)
	posts, dms := len(h.Discord.Posts()), len(h.Discord.DMs())

	cp, err := h.App.CopyEsemeny(ctx, "bela", e.ID)
	if err != nil {
		t.Fatal(err)
	}
	p := h.Discord.Posts()
	if len(p) != posts+1 {
		t.Fatalf("posts = %d, want one more", len(p))
	}
	post := p[len(p)-1]
	if post.Ref.ChannelID != "chan-1" || post.Message.EsemenyID != cp.ID || post.Message.SzervezoName != "bela" || len(post.Message.Idopontok) != 2 {
		t.Fatalf("post = %+v", post)
	}
	// Everyone but the copier is asked to answer, by DM.
	if got := len(h.Discord.DMs()) - dms; got != 2 {
		t.Fatalf("%d DMs sent, want 2 (Anna and Cecil)", got)
	}
	for _, dm := range h.Discord.DMs()[dms:] {
		if dm.DiscordID == "discord-bela" {
			t.Fatalf("the copier got a DM: %+v", dm)
		}
	}
}

func TestACopyWithoutACsatornaIsSentToEveryTagByDM(t *testing.T) {
	h := apptest.New(t)
	ctx := context.Background()
	c := csoportOf(t, h, "bela")
	e := propose(t, h, "anna", c.ID, 1)
	dms := len(h.Discord.DMs())

	cp, err := h.App.CopyEsemeny(ctx, "bela", e.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got := len(h.Discord.DMs()) - dms; got != 2 {
		t.Fatalf("%d DMs sent, want one for each Tag", got)
	}
	for _, dm := range h.Discord.DMs()[dms:] {
		if dm.Esemeny == nil || dm.Esemeny.EsemenyID != cp.ID {
			t.Fatalf("DM = %+v, want the copy's message", dm)
		}
	}
}
