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

// csoportOf has Anna create a Csoport that the other Felhasznalok join as
// Tags. Every id is also the Felhasznalo's name.
func csoportOf(t *testing.T, h *apptest.Harness, others ...string) domain.Csoport {
	t.Helper()
	ctx := context.Background()
	c, m := csoportWithMeghivo(t, h)
	for _, id := range others {
		addFelhasznalo(t, h, id, id)
		if _, err := h.App.JoinCsoport(ctx, id, m.Token); err != nil {
			t.Fatalf("JoinCsoport(%s): %v", id, err)
		}
	}
	return c
}

// idopontok builds n one-evening Idopontok on consecutive days of December.
func idopontok(n int) []app.IdopontInput {
	out := make([]app.IdopontInput, n)
	for i := range out {
		day := time.Date(2026, time.December, 1+i, 0, 0, 0, 0, time.UTC).Format("2006-01-02")
		out[i] = app.IdopontInput{Start: day + "T20:00", End: day + "T23:00"}
	}
	return out
}

func propose(t *testing.T, h *apptest.Harness, felhasznaloID, csoportID string, n int) domain.Esemeny {
	t.Helper()
	e, err := h.App.ProposeEsemeny(context.Background(), felhasznaloID, csoportID, app.EsemenyInput{
		Title:     "Társasjáték",
		Idopontok: idopontok(n),
	})
	if err != nil {
		t.Fatalf("ProposeEsemeny: %v", err)
	}
	return e
}

func detail(t *testing.T, h *apptest.Harness, felhasznaloID, esemenyID string) app.EsemenyDetail {
	t.Helper()
	d, err := h.App.Esemeny(context.Background(), felhasznaloID, esemenyID)
	if err != nil {
		t.Fatalf("Esemeny: %v", err)
	}
	return d
}

func answer(t *testing.T, h *apptest.Harness, felhasznaloID string, d app.EsemenyDetail, idopont int, v domain.Valasz) {
	t.Helper()
	if err := h.App.SetVisszajelzes(context.Background(), felhasznaloID, d.ID, d.Idopontok[idopont].ID, v); err != nil {
		t.Fatalf("SetVisszajelzes(%s, %d, %s): %v", felhasznaloID, idopont, v, err)
	}
}

func helyezesek(d app.EsemenyDetail) []app.Helyezes {
	out := make([]app.Helyezes, len(d.Idopontok))
	for i, a := range d.Idopontok {
		out[i] = a.Helyezes
	}
	return out
}

func expectHelyezesek(t *testing.T, d app.EsemenyDetail, want ...app.Helyezes) {
	t.Helper()
	got := helyezesek(d)
	if len(got) != len(want) {
		t.Fatalf("helyezesek = %v, want %v", got, want)
	}
	for i := range got {
		if got[i] != want[i] {
			t.Fatalf("helyezesek = %v, want %v", got, want)
		}
	}
}

func TestProposedEsemenyStartsInEgyeztetesWithTheProposerAsSzervezo(t *testing.T) {
	h := apptest.New(t)
	ctx := context.Background()
	c := csoportOf(t, h, "bela")

	e, err := h.App.ProposeEsemeny(ctx, "bela", c.ID, app.EsemenyInput{
		Title:       "  Kocsmázás  ",
		Description: "  Sör és csocsó  ",
		Helyszin:    domain.Helyszin{Name: " Kisüzem ", URL: " https://maps.app.goo.gl/abc "},
		Idopontok: []app.IdopontInput{
			// Winter (UTC+1): 20:00 in Budapest is 19:00 UTC.
			{Start: "2026-12-05T20:00", End: "2026-12-05T23:30"},
			// Summer (UTC+2), no end.
			{Start: "2026-07-10T18:00"},
			// A multi-day trip.
			{Start: "2026-08-14T18:00", End: "2026-08-16T14:00"},
		},
	})
	if err != nil {
		t.Fatalf("ProposeEsemeny: %v", err)
	}
	if e.CsoportID != c.ID || e.SzervezoID != "bela" || e.Allapot != domain.Egyeztetes || !e.CreatedAt.Equal(apptest.Start) {
		t.Fatalf("esemeny = %+v", e)
	}

	d := detail(t, h, "anna", e.ID)
	if d.Title != "Kocsmázás" || d.Description != "Sör és csocsó" ||
		d.Helyszin != (domain.Helyszin{Name: "Kisüzem", URL: "https://maps.app.goo.gl/abc"}) {
		t.Fatalf("detail = %+v, want trimmed fields", d.Esemeny)
	}
	if d.CsoportName != "Péntek esték" || d.SzervezoName != "bela" {
		t.Fatalf("csoport %q, szervezo %q", d.CsoportName, d.SzervezoName)
	}

	// Chronological order, stored in UTC.
	utc := func(y int, m time.Month, day, hh, mm int) time.Time {
		return time.Date(y, m, day, hh, mm, 0, 0, time.UTC)
	}
	want := []struct {
		start time.Time
		end   *time.Time
	}{
		{utc(2026, time.July, 10, 16, 0), nil},
		{utc(2026, time.August, 14, 16, 0), ptr(utc(2026, time.August, 16, 12, 0))},
		{utc(2026, time.December, 5, 19, 0), ptr(utc(2026, time.December, 5, 22, 30))},
	}
	if len(d.Idopontok) != len(want) {
		t.Fatalf("got %d idopontok, want %d", len(d.Idopontok), len(want))
	}
	for n, w := range want {
		i := d.Idopontok[n]
		if !i.Start.Equal(w.start) || i.Start.Location() != time.UTC {
			t.Errorf("idopont %d starts at %v, want %v", n, i.Start, w.start)
		}
		if (i.End == nil) != (w.end == nil) || (i.End != nil && !i.End.Equal(*w.end)) {
			t.Errorf("idopont %d ends at %v, want %v", n, i.End, w.end)
		}
		if i.AddedBy != "bela" {
			t.Errorf("idopont %d added by %q, want bela", n, i.AddedBy)
		}
		if i.Helyezes != 0 || i.Igen+i.Talan+i.Nem != 0 || i.Sajat != "" {
			t.Errorf("idopont %d = %+v, want no answers and no place", n, i)
		}
	}
}

func ptr[T any](v T) *T { return &v }

func TestProposeEsemenyRejectsBadInput(t *testing.T) {
	h := apptest.New(t)
	c := csoportOf(t, h)
	ok := idopontok(1)
	for name, in := range map[string]app.EsemenyInput{
		"blank title":          {Title: "   ", Idopontok: ok},
		"title too long":       {Title: strings.Repeat("é", app.MaxEsemenyTitleLength+1), Idopontok: ok},
		"description too long": {Title: "X", Description: strings.Repeat("a", app.MaxEsemenyDescriptionLength+1), Idopontok: ok},
		"no idopont":           {Title: "X"},
		"too many idopontok":   {Title: "X", Idopontok: idopontok(app.MaxIdopontok + 1)},
		"missing start":        {Title: "X", Idopontok: []app.IdopontInput{{End: "2026-12-05T20:00"}}},
		"malformed start":      {Title: "X", Idopontok: []app.IdopontInput{{Start: "12.05 20:00"}}},
		"malformed end":        {Title: "X", Idopontok: []app.IdopontInput{{Start: "2026-12-05T20:00", End: "23:00"}}},
		"end before start":     {Title: "X", Idopontok: []app.IdopontInput{{Start: "2026-12-05T20:00", End: "2026-12-05T19:00"}}},
		"end equals start":     {Title: "X", Idopontok: []app.IdopontInput{{Start: "2026-12-05T20:00", End: "2026-12-05T20:00"}}},
		"end days before":      {Title: "X", Idopontok: []app.IdopontInput{{Start: "2026-12-07T10:00", End: "2026-12-05T12:00"}}},
		"relative URL":         {Title: "X", Helyszin: domain.Helyszin{URL: "/maps/abc"}, Idopontok: ok},
		"URL without scheme":   {Title: "X", Helyszin: domain.Helyszin{URL: "maps.google.com/abc"}, Idopontok: ok},
		"non-http URL":         {Title: "X", Helyszin: domain.Helyszin{URL: "javascript:alert(1)"}, Idopontok: ok},
		"URL without host":     {Title: "X", Helyszin: domain.Helyszin{URL: "https://"}, Idopontok: ok},
		"helyszin too long":    {Title: "X", Helyszin: domain.Helyszin{Name: strings.Repeat("a", app.MaxHelyszinNameLength+1)}, Idopontok: ok},
	} {
		t.Run(name, func(t *testing.T) {
			_, err := h.App.ProposeEsemeny(context.Background(), "anna", c.ID, in)
			if !errors.Is(err, app.ErrInvalidInput) {
				t.Fatalf("err = %v, want ErrInvalidInput", err)
			}
		})
	}

	list, err := h.App.EsemenyekOfCsoport(context.Background(), "anna", c.ID)
	if err != nil || len(list) != 0 {
		t.Fatalf("after rejected proposals: list = %+v, err = %v; want none", list, err)
	}
}

func TestHelyszinIsOptionalAndItsURLMayStandAlone(t *testing.T) {
	h := apptest.New(t)
	ctx := context.Background()
	c := csoportOf(t, h)
	for _, hs := range []domain.Helyszin{{}, {Name: "Nálam"}, {URL: "http://maps.google.com/?q=Budapest"}} {
		e, err := h.App.ProposeEsemeny(ctx, "anna", c.ID, app.EsemenyInput{Title: "X", Helyszin: hs, Idopontok: idopontok(1)})
		if err != nil {
			t.Fatalf("ProposeEsemeny with %+v: %v", hs, err)
		}
		if e.Helyszin != hs {
			t.Fatalf("helyszin = %+v, want %+v", e.Helyszin, hs)
		}
	}
}

func TestEsemenyekOfCsoportListsNewestFirst(t *testing.T) {
	h := apptest.New(t)
	ctx := context.Background()
	c := csoportOf(t, h, "bela")
	first := propose(t, h, "anna", c.ID, 2)
	h.Clock.Advance(time.Hour)
	second := propose(t, h, "bela", c.ID, 1)

	list, err := h.App.EsemenyekOfCsoport(ctx, "bela", c.ID)
	if err != nil {
		t.Fatalf("EsemenyekOfCsoport: %v", err)
	}
	if len(list) != 2 || list[0].ID != second.ID || list[1].ID != first.ID {
		t.Fatalf("list = %+v, want newest first", list)
	}
	want := time.Date(2026, time.December, 1, 19, 0, 0, 0, time.UTC)
	if list[1].IdopontCount != 2 || list[1].FirstStart == nil || !list[1].FirstStart.Equal(want) {
		t.Fatalf("summary = %+v, want 2 idopontok starting %v", list[1], want)
	}
}

func TestOnlyTagsMayProposeViewOrAnswer(t *testing.T) {
	h := apptest.New(t)
	ctx := context.Background()
	c := csoportOf(t, h)
	e := propose(t, h, "anna", c.ID, 1)
	d := detail(t, h, "anna", e.ID)
	addFelhasznalo(t, h, "dani", "Dani")

	if _, err := h.App.ProposeEsemeny(ctx, "dani", c.ID, app.EsemenyInput{Title: "X", Idopontok: idopontok(1)}); !errors.Is(err, app.ErrNotFound) {
		t.Errorf("propose as non-Tag: err = %v, want ErrNotFound", err)
	}
	if _, err := h.App.ProposeEsemeny(ctx, "anna", "no-such-csoport", app.EsemenyInput{Title: "X", Idopontok: idopontok(1)}); !errors.Is(err, app.ErrNotFound) {
		t.Errorf("propose in unknown Csoport: err = %v, want ErrNotFound", err)
	}
	if _, err := h.App.EsemenyekOfCsoport(ctx, "dani", c.ID); !errors.Is(err, app.ErrNotFound) {
		t.Errorf("list as non-Tag: err = %v, want ErrNotFound", err)
	}
	if _, err := h.App.Esemeny(ctx, "dani", e.ID); !errors.Is(err, app.ErrNotFound) {
		t.Errorf("view as non-Tag: err = %v, want ErrNotFound", err)
	}
	if _, err := h.App.Esemeny(ctx, "anna", "no-such-esemeny"); !errors.Is(err, app.ErrNotFound) {
		t.Errorf("view unknown: err = %v, want ErrNotFound", err)
	}
	if err := h.App.SetVisszajelzes(ctx, "dani", e.ID, d.Idopontok[0].ID, domain.Igen); !errors.Is(err, app.ErrNotFound) {
		t.Errorf("answer as non-Tag: err = %v, want ErrNotFound", err)
	}
}

func TestATagAnswersAndChangesTheirVisszajelzes(t *testing.T) {
	h := apptest.New(t)
	c := csoportOf(t, h, "bela")
	e := propose(t, h, "anna", c.ID, 2)
	d := detail(t, h, "bela", e.ID)

	answer(t, h, "bela", d, 0, domain.Igen)
	answer(t, h, "bela", d, 1, domain.Talan)
	h.Clock.Advance(time.Minute)
	answer(t, h, "bela", d, 0, domain.Nem)

	d = detail(t, h, "bela", e.ID)
	first, second := d.Idopontok[0], d.Idopontok[1]
	if first.Sajat != domain.Nem || first.Igen != 0 || first.Nem != 1 || len(first.Visszajelzesek) != 1 {
		t.Fatalf("first = %+v, want Béla's single answer changed to nem", first)
	}
	if v := first.Visszajelzesek[0]; v.FelhasznaloID != "bela" || v.FelhasznaloName != "bela" || !v.IsTag ||
		!v.UpdatedAt.Equal(apptest.Start.Add(time.Minute)) {
		t.Fatalf("visszajelzes = %+v", v)
	}
	if second.Sajat != domain.Talan || second.Talan != 1 {
		t.Fatalf("second = %+v, want talan", second)
	}

	// Anna sees the counts but has no answer of her own.
	d = detail(t, h, "anna", e.ID)
	if d.Idopontok[0].Sajat != "" || d.Idopontok[0].Nem != 1 {
		t.Fatalf("as Anna: %+v", d.Idopontok[0])
	}
}

func TestSetVisszajelzesRejectsBadInput(t *testing.T) {
	h := apptest.New(t)
	ctx := context.Background()
	c := csoportOf(t, h)
	e := propose(t, h, "anna", c.ID, 1)
	other := propose(t, h, "anna", c.ID, 1)
	d := detail(t, h, "anna", e.ID)
	otherIdopont := detail(t, h, "anna", other.ID).Idopontok[0].ID

	if err := h.App.SetVisszajelzes(ctx, "anna", e.ID, d.Idopontok[0].ID, "talán"); !errors.Is(err, app.ErrInvalidInput) {
		t.Errorf("unknown valasz: err = %v, want ErrInvalidInput", err)
	}
	if err := h.App.SetVisszajelzes(ctx, "anna", e.ID, otherIdopont, domain.Igen); !errors.Is(err, app.ErrNotFound) {
		t.Errorf("idopont of another esemeny: err = %v, want ErrNotFound", err)
	}
	if err := h.App.SetVisszajelzes(ctx, "anna", e.ID, "no-such-idopont", domain.Igen); !errors.Is(err, app.ErrNotFound) {
		t.Errorf("unknown idopont: err = %v, want ErrNotFound", err)
	}
}

func TestHelyezesRanksByIgenThenTalanThenFewerNem(t *testing.T) {
	h := apptest.New(t)
	c := csoportOf(t, h, "b", "c", "d")
	e := propose(t, h, "anna", c.ID, 5)
	d := detail(t, h, "anna", e.ID)

	// 0: 2 igen.
	answer(t, h, "anna", d, 0, domain.Igen)
	answer(t, h, "b", d, 0, domain.Igen)
	// 1: 1 igen, 2 talan.
	answer(t, h, "anna", d, 1, domain.Igen)
	answer(t, h, "b", d, 1, domain.Talan)
	answer(t, h, "c", d, 1, domain.Talan)
	// 2: 1 igen, 1 talan, 2 nem: loses to 1 on talan.
	answer(t, h, "anna", d, 2, domain.Igen)
	answer(t, h, "b", d, 2, domain.Talan)
	answer(t, h, "c", d, 2, domain.Nem)
	answer(t, h, "d", d, 2, domain.Nem)
	// 3: 1 igen, 1 talan, 1 nem: beats 2 on fewer nem.
	answer(t, h, "anna", d, 3, domain.Igen)
	answer(t, h, "b", d, 3, domain.Talan)
	answer(t, h, "c", d, 3, domain.Nem)
	// 4: 3 igen and 1 nem: igen decides before nem.
	answer(t, h, "anna", d, 4, domain.Igen)
	answer(t, h, "b", d, 4, domain.Igen)
	answer(t, h, "c", d, 4, domain.Igen)
	answer(t, h, "d", d, 4, domain.Nem)

	d = detail(t, h, "anna", e.ID)
	expectHelyezesek(t, d, 2, 3, 5, 4, 1)
	if a := d.Idopontok[2]; a.Igen != 1 || a.Talan != 1 || a.Nem != 2 {
		t.Fatalf("counts of idopont 2 = %d/%d/%d, want 1/1/2", a.Igen, a.Talan, a.Nem)
	}
}

func TestTiedIdopontokShareAPlaceAndUnansweredOnesHaveNone(t *testing.T) {
	h := apptest.New(t)
	c := csoportOf(t, h, "b")
	e := propose(t, h, "anna", c.ID, 5)
	d := detail(t, h, "anna", e.ID)

	answer(t, h, "anna", d, 0, domain.Igen)
	answer(t, h, "anna", d, 1, domain.Igen)
	answer(t, h, "anna", d, 2, domain.Talan)
	answer(t, h, "b", d, 4, domain.Nem)
	// 3 is unanswered. 4 has only a nem: it still has a place, the last.

	expectHelyezesek(t, detail(t, h, "anna", e.ID), 1, 1, 3, 0, 4)
}

func TestNobodyAnsweredMeansNoPlaceAtAll(t *testing.T) {
	h := apptest.New(t)
	c := csoportOf(t, h)
	e := propose(t, h, "anna", c.ID, 3)
	expectHelyezesek(t, detail(t, h, "anna", e.ID), 0, 0, 0)
}

func TestOnlyTagsVisszajelzesekCount(t *testing.T) {
	h := apptest.New(t)
	c := csoportOf(t, h, "b", "volt")
	e := propose(t, h, "anna", c.ID, 2)
	d := detail(t, h, "anna", e.ID)

	answer(t, h, "anna", d, 0, domain.Igen)
	answer(t, h, "volt", d, 1, domain.Igen)
	answer(t, h, "b", d, 1, domain.Talan)
	answer(t, h, "volt", d, 0, domain.Nem)
	// "volt" stops being a Tag. Leaving isn't a use case yet, so go through
	// the store.
	if err := h.Store.DeleteTag(context.Background(), c.ID, "volt"); err != nil {
		t.Fatalf("remove tag: %v", err)
	}

	d = detail(t, h, "anna", e.ID)
	// Without "volt": 0 has 1 igen, 1 has 1 talan.
	expectHelyezesek(t, d, 1, 2)
	if a := d.Idopontok[1]; a.Igen != 0 || a.Talan != 1 || len(a.Visszajelzesek) != 2 {
		t.Fatalf("idopont 1 = %+v, want only b's talan counted, both listed", a)
	}
	for _, v := range d.Idopontok[1].Visszajelzesek {
		if v.FelhasznaloID == "volt" && v.IsTag {
			t.Fatalf("former Tag listed as Tag")
		}
	}
}

func TestHelyezesekIsAGenericRanking(t *testing.T) {
	// Javaslatok (#19) are ranked by their number of backers alone.
	got := app.Helyezesek([]app.Pontszam{{2}, {5}, nil, {2}, {0}, {5}})
	want := []app.Helyezes{3, 1, 0, 3, 5, 1}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("Helyezesek = %v, want %v", got, want)
		}
	}
	if len(app.Helyezesek(nil)) != 0 {
		t.Fatalf("empty input should give an empty ranking")
	}
}

func TestAnIdopontAnsweredOnlyByAFormerTagHasNoPlace(t *testing.T) {
	h := apptest.New(t)
	c := csoportOf(t, h, "volt")
	e := propose(t, h, "anna", c.ID, 2)
	d := detail(t, h, "anna", e.ID)

	answer(t, h, "anna", d, 0, domain.Talan)
	answer(t, h, "volt", d, 1, domain.Igen)
	if err := h.Store.DeleteTag(context.Background(), c.ID, "volt"); err != nil {
		t.Fatalf("remove tag: %v", err)
	}

	d = detail(t, h, "anna", e.ID)
	expectHelyezesek(t, d, 1, 0)
	a := d.Idopontok[1]
	if a.Igen != 0 || a.Talan != 0 || a.Nem != 0 {
		t.Fatalf("counts = %d/%d/%d, want none", a.Igen, a.Talan, a.Nem)
	}
	if len(a.Visszajelzesek) != 1 || a.Visszajelzesek[0].FelhasznaloID != "volt" ||
		a.Visszajelzesek[0].IsTag || a.Visszajelzesek[0].Valasz != domain.Igen {
		t.Fatalf("visszajelzesek = %+v, want volt's igen listed as a non-Tag", a.Visszajelzesek)
	}
}

func TestWholeDayIdopontRunsFromBudapestMidnightToMidnightAfterTheLastDay(t *testing.T) {
	h := apptest.New(t)
	ctx := context.Background()
	c := csoportOf(t, h)
	e, err := h.App.ProposeEsemeny(ctx, "anna", c.ID, app.EsemenyInput{
		Title: "Kirándulás",
		Idopontok: []app.IdopontInput{
			// One winter day.
			{Start: "2026-12-05", EgeszNapos: true},
			// Three days across the spring DST change (29 March).
			{Start: "2026-03-28", End: "2026-03-30", EgeszNapos: true},
		},
	})
	if err != nil {
		t.Fatalf("ProposeEsemeny: %v", err)
	}
	d := detail(t, h, "anna", e.ID)
	want := []struct{ start, end time.Time }{
		{time.Date(2026, 3, 27, 23, 0, 0, 0, time.UTC), time.Date(2026, 3, 30, 22, 0, 0, 0, time.UTC)},
		{time.Date(2026, 12, 4, 23, 0, 0, 0, time.UTC), time.Date(2026, 12, 5, 23, 0, 0, 0, time.UTC)},
	}
	for n, w := range want {
		i := d.Idopontok[n]
		if !i.EgeszNapos || !i.Start.Equal(w.start) || i.End == nil || !i.End.Equal(w.end) {
			t.Errorf("idopont %d = %v – %v (egesz napos %v), want %v – %v", n, i.Start, i.End, i.EgeszNapos, w.start, w.end)
		}
	}

	for name, in := range map[string]app.IdopontInput{
		"last day before first": {Start: "2026-12-07", End: "2026-12-05", EgeszNapos: true},
		"a time, not a day":     {Start: "2026-12-05T20:00", EgeszNapos: true},
	} {
		_, err := h.App.ProposeEsemeny(ctx, "anna", c.ID, app.EsemenyInput{Title: "X", Idopontok: []app.IdopontInput{in}})
		if !errors.Is(err, app.ErrInvalidInput) {
			t.Errorf("%s: err = %v, want ErrInvalidInput", name, err)
		}
	}
}

func TestLocalTimesAroundDaylightSavingChanges(t *testing.T) {
	h := apptest.New(t)
	ctx := context.Background()
	c := csoportOf(t, h)

	// Clocks jump from 02:00 to 03:00 on 29 March 2026: 02:30 never happens.
	for _, in := range []app.IdopontInput{
		{Start: "2026-03-29T02:30"},
		{Start: "2026-03-28T20:00", End: "2026-03-29T02:15"},
	} {
		_, err := h.App.ProposeEsemeny(ctx, "anna", c.ID, app.EsemenyInput{Title: "X", Idopontok: []app.IdopontInput{in}})
		if !errors.Is(err, app.ErrNonexistentLocalTime) || !errors.Is(err, app.ErrInvalidInput) {
			t.Errorf("%+v: err = %v, want ErrNonexistentLocalTime", in, err)
		}
	}

	// Clocks fall back from 03:00 to 02:00 on 25 October 2026: 02:30 happens
	// twice and means the first, still in summer time (UTC+2).
	e, err := h.App.ProposeEsemeny(ctx, "anna", c.ID, app.EsemenyInput{Title: "X", Idopontok: []app.IdopontInput{{Start: "2026-10-25T02:30"}}})
	if err != nil {
		t.Fatalf("ProposeEsemeny in the autumn overlap: %v", err)
	}
	got := detail(t, h, "anna", e.ID).Idopontok[0].Start
	if want := time.Date(2026, 10, 25, 0, 30, 0, 0, time.UTC); !got.Equal(want) {
		t.Fatalf("start = %v, want the first occurrence %v", got, want)
	}
}
