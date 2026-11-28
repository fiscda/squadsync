package app_test

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"squadsync-be/internal/app"
	"squadsync-be/internal/apptest"
	"squadsync-be/internal/domain"
)

// vendegWorld is a Csoport of Anna (the Szervezo), Bela and Cecil with an
// Esemeny of two Idopontok, still in Egyeztetes. Vera, a Felhasznalo, has
// joined it as a Vendeg through Bela's Esemeny-meghivo, and Bela has added
// Nagyi, a name-only Vendeg. Nobody has answered yet.
type vendegWorld struct {
	h       *apptest.Harness
	c       domain.Csoport
	d       app.EsemenyDetail
	meghivo domain.EsemenyMeghivo
	nagyi   domain.Vendeg
}

func newVendegWorld(t *testing.T) vendegWorld {
	t.Helper()
	ctx := context.Background()
	h := apptest.New(t)
	c := csoportOf(t, h, "bela", "cecil")
	e := propose(t, h, "anna", c.ID, 2)
	m, err := h.App.CreateEsemenyMeghivo(ctx, "bela", e.ID)
	if err != nil {
		t.Fatalf("CreateEsemenyMeghivo: %v", err)
	}
	joinAsVendeg(t, h, "vera", "Vera", m.Token)
	nagyi, err := h.App.AddVendeg(ctx, "bela", e.ID, "  Nagyi ")
	if err != nil {
		t.Fatalf("AddVendeg: %v", err)
	}
	return vendegWorld{h: h, c: c, d: detail(t, h, "anna", e.ID), meghivo: m, nagyi: nagyi}
}

// joinAsVendeg registers a Felhasznalo and has them open the link.
func joinAsVendeg(t *testing.T, h *apptest.Harness, id, name, token string) {
	t.Helper()
	addFelhasznalo(t, h, id, name)
	res, err := h.App.JoinEsemeny(context.Background(), id, token)
	if err != nil || !res.Joined {
		t.Fatalf("JoinEsemeny(%s) = %+v, %v; want joined", id, res, err)
	}
}

// answerFor is a Tag answering for their name-only Vendeg.
func answerFor(t *testing.T, w vendegWorld, tagID string, v domain.Vendeg, idopont int, valasz domain.Valasz) {
	t.Helper()
	if err := w.h.App.SetVendegVisszajelzes(context.Background(), tagID, w.d.ID, w.d.Idopontok[idopont].ID, v.ID, valasz); err != nil {
		t.Fatalf("SetVendegVisszajelzes(%s for %s): %v", tagID, v.Name, err)
	}
}

// confirmed has Anna, Bela, Vera and (by Bela) Nagyi say igen and Cecil
// nem on the first Idopont, which Anna confirms.
func (w vendegWorld) confirmed(t *testing.T) vendegWorld {
	t.Helper()
	for _, who := range []string{"anna", "bela", "vera"} {
		answer(t, w.h, who, w.d, 0, domain.Igen)
	}
	answer(t, w.h, "cecil", w.d, 0, domain.Nem)
	answerFor(t, w, "bela", w.nagyi, 0, domain.Igen)
	confirm(t, w.h, "anna", w.d, 0)
	w.d = detail(t, w.h, "anna", w.d.ID)
	return w
}

// resztvevoNames shows the Resztvevok as "name name ...", a Vendeg marked
// with a star.
func resztvevoNames(d app.EsemenyDetail) string {
	parts := []string{}
	for _, r := range d.Resztvevok {
		name := r.Name
		if r.IsVendeg {
			name += "*"
		}
		parts = append(parts, name)
	}
	return strings.Join(parts, " ")
}

func expectErr(t *testing.T, what string, err, want error) {
	t.Helper()
	if !errors.Is(err, want) {
		t.Fatalf("%s: err = %v, want %v", what, err, want)
	}
}

func TestAnEsemenyMeghivoMakesAFelhasznaloAVendegOfThatEsemenyOnly(t *testing.T) {
	w := newVendegWorld(t)
	ctx := context.Background()

	p, err := w.h.App.EsemenyMeghivoPreview(ctx, w.meghivo.Token)
	if err != nil || p.EsemenyID != w.d.ID || p.Title != "Társasjáték" || p.SzervezoName != "Anna" {
		t.Fatalf("EsemenyMeghivoPreview = %+v, %v", p, err)
	}
	got := detail(t, w.h, "vera", w.d.ID)
	if !got.ViewerIsVendeg || got.CsoportName != "" || got.Title != "Társasjáték" {
		t.Fatalf("Vera's view: ViewerIsVendeg=%v CsoportName=%q Title=%q", got.ViewerIsVendeg, got.CsoportName, got.Title)
	}
	if id, err := w.h.App.MemberEsemenyOfMeghivo(ctx, "vera", w.meghivo.Token); err != nil || id != w.d.ID {
		t.Fatalf("MemberEsemenyOfMeghivo = %q, %v", id, err)
	}
	// Vera isn't a Tag: the Csoport and its other Esemenyek stay hidden.
	other := propose(t, w.h, "anna", w.c.ID, 1)
	_, err = w.h.App.Esemeny(ctx, "vera", other.ID)
	expectErr(t, "another Esemeny of the Csoport", err, app.ErrNotFound)
	_, err = w.h.App.Csoport(ctx, "vera", w.c.ID)
	expectErr(t, "the Csoport", err, app.ErrNotFound)
	if list, err := w.h.App.CsoportokOf(ctx, "vera"); err != nil || len(list) != 0 {
		t.Fatalf("Vera's Csoportok = %+v, %v; want none", list, err)
	}

	// The Vendegek as a Tag sees them.
	vendegek := detail(t, w.h, "bela", w.d.ID).Vendegek
	if len(vendegek) != 2 {
		t.Fatalf("vendegek = %+v", vendegek)
	}
	nagyi, vera := vendegek[0], vendegek[1]
	if nagyi.Name != "Nagyi" || nagyi.Kind != domain.VendegNev || nagyi.AddedBy != "bela" || !nagyi.CanAnswer || !nagyi.CanRemove {
		t.Fatalf("Nagyi as Bela sees her = %+v", nagyi)
	}
	if vera.Name != "Vera" || vera.Kind != domain.VendegFelhasznalo || vera.FelhasznaloID != "vera" || vera.CanAnswer || vera.CanRemove {
		t.Fatalf("Vera as Bela sees her = %+v", vera)
	}
}

func TestATagOrAVendegOpeningAnEsemenyMeghivoJustLandsOnTheEsemeny(t *testing.T) {
	w := newVendegWorld(t)
	ctx := context.Background()

	// A Tag doesn't become a Vendeg.
	res, err := w.h.App.JoinEsemeny(ctx, "cecil", w.meghivo.Token)
	if err != nil || res.Joined || res.EsemenyID != w.d.ID {
		t.Fatalf("a Tag opening it = %+v, %v; want the Esemeny, not joined", res, err)
	}
	if d := detail(t, w.h, "cecil", w.d.ID); d.ViewerIsVendeg || len(d.Vendegek) != 2 {
		t.Fatalf("Cecil after opening: ViewerIsVendeg=%v, %d vendegek", d.ViewerIsVendeg, len(d.Vendegek))
	}
	if id, err := w.h.App.MemberEsemenyOfMeghivo(ctx, "cecil", w.meghivo.Token); err != nil || id != w.d.ID {
		t.Fatalf("MemberEsemenyOfMeghivo(Tag) = %q, %v", id, err)
	}

	// A Vendeg rejoining changes nothing, even once the link is revoked.
	if err := w.h.App.RevokeEsemenyMeghivo(ctx, "anna", w.d.ID, w.meghivo.ID); err != nil {
		t.Fatalf("RevokeEsemenyMeghivo: %v", err)
	}
	res, err = w.h.App.JoinEsemeny(ctx, "vera", w.meghivo.Token)
	if err != nil || res.Joined || res.EsemenyID != w.d.ID {
		t.Fatalf("Vera opening it again = %+v, %v", res, err)
	}
	if n := len(detail(t, w.h, "anna", w.d.ID).Vendegek); n != 2 {
		t.Fatalf("vendegek after rejoining = %d, want 2", n)
	}

	// A stranger is no member.
	addFelhasznalo(t, w.h, "zoli", "Zoli")
	_, err = w.h.App.MemberEsemenyOfMeghivo(ctx, "zoli", w.meghivo.Token)
	expectErr(t, "a stranger's MemberEsemenyOfMeghivo", err, app.ErrNotFound)
	_, err = w.h.App.JoinEsemeny(ctx, "ghost", w.meghivo.Token)
	expectErr(t, "an unknown Felhasznalo", err, app.ErrFelhasznaloUnknown)
}

func TestRevokedAndExpiredEsemenyMeghivokAreRefused(t *testing.T) {
	ctx := context.Background()

	t.Run("unknown", func(t *testing.T) {
		w := newVendegWorld(t)
		_, err := w.h.App.EsemenyMeghivoPreview(ctx, "no-such-token")
		expectErr(t, "preview", err, app.ErrNotFound)
		_, err = w.h.App.JoinEsemeny(ctx, "anna", "no-such-token")
		expectErr(t, "join", err, app.ErrNotFound)
	})

	t.Run("revoked", func(t *testing.T) {
		w := newVendegWorld(t)
		if err := w.h.App.RevokeEsemenyMeghivo(ctx, "cecil", w.d.ID, w.meghivo.ID); err != nil {
			t.Fatalf("RevokeEsemenyMeghivo: %v", err)
		}
		if err := w.h.App.RevokeEsemenyMeghivo(ctx, "cecil", w.d.ID, w.meghivo.ID); err != nil {
			t.Fatalf("revoking twice: %v", err)
		}
		_, err := w.h.App.EsemenyMeghivoPreview(ctx, w.meghivo.Token)
		expectErr(t, "preview", err, app.ErrEsemenyMeghivoRevoked)
		addFelhasznalo(t, w.h, "zoli", "Zoli")
		_, err = w.h.App.JoinEsemeny(ctx, "zoli", w.meghivo.Token)
		expectErr(t, "join", err, app.ErrEsemenyMeghivoRevoked)
		if list, err := w.h.App.EsemenyMeghivok(ctx, "anna", w.d.ID); err != nil || len(list) != 0 {
			t.Fatalf("working meghivok = %+v, %v; want none", list, err)
		}
		// Another link still works.
		m, err := w.h.App.CreateEsemenyMeghivo(ctx, "cecil", w.d.ID)
		if err != nil {
			t.Fatalf("CreateEsemenyMeghivo: %v", err)
		}
		if m.Token == w.meghivo.Token || len(m.Token) < 40 {
			t.Fatalf("a new token %q: want a fresh unguessable one", m.Token)
		}
		joinAsVendeg(t, w.h, "zsofi", "Zsófi", m.Token)
	})

	for _, end := range []struct {
		name string
		do   func(w vendegWorld)
	}{
		{"lezajlott", func(w vendegWorld) { setLezajlott(t, w.h, w.d) }},
		{"elmaradt", func(w vendegWorld) { cancelEsemeny(t, w.h, "anna", w.d.ID) }},
	} {
		t.Run(end.name, func(t *testing.T) {
			w := newVendegWorld(t)
			end.do(w)
			_, err := w.h.App.EsemenyMeghivoPreview(ctx, w.meghivo.Token)
			expectErr(t, "preview", err, app.ErrEsemenyMeghivoExpired)
			addFelhasznalo(t, w.h, "zoli", "Zoli")
			_, err = w.h.App.JoinEsemeny(ctx, "zoli", w.meghivo.Token)
			expectErr(t, "join", err, app.ErrEsemenyMeghivoExpired)
			_, err = w.h.App.CreateEsemenyMeghivo(ctx, "anna", w.d.ID)
			expectErr(t, "a new meghivo", err, app.ErrVendegClosed)
			_, err = w.h.App.AddVendeg(ctx, "anna", w.d.ID, "Kuzin")
			expectErr(t, "a new name-only Vendeg", err, app.ErrVendegClosed)
			if list, err := w.h.App.EsemenyMeghivok(ctx, "anna", w.d.ID); err != nil || len(list) != 0 {
				t.Fatalf("working meghivok = %+v, %v; want none", list, err)
			}
			// Vera, already a Vendeg, still gets in.
			if res, err := w.h.App.JoinEsemeny(ctx, "vera", w.meghivo.Token); err != nil || res.Joined {
				t.Fatalf("Vera opening it = %+v, %v", res, err)
			}
		})
	}
}

func TestOnlyTagsCreateAndRevokeEsemenyMeghivok(t *testing.T) {
	w := newVendegWorld(t)
	ctx := context.Background()
	_, err := w.h.App.CreateEsemenyMeghivo(ctx, "vera", w.d.ID)
	expectErr(t, "a Vendeg creating one", err, app.ErrNotFound)
	_, err = w.h.App.EsemenyMeghivok(ctx, "vera", w.d.ID)
	expectErr(t, "a Vendeg listing them", err, app.ErrNotFound)
	expectErr(t, "a Vendeg revoking one", w.h.App.RevokeEsemenyMeghivo(ctx, "vera", w.d.ID, w.meghivo.ID), app.ErrNotFound)

	other := propose(t, w.h, "anna", w.c.ID, 1)
	expectErr(t, "revoking through another Esemeny", w.h.App.RevokeEsemenyMeghivo(ctx, "anna", other.ID, w.meghivo.ID), app.ErrNotFound)
	list, err := w.h.App.EsemenyMeghivok(ctx, "cecil", w.d.ID)
	if err != nil || len(list) != 1 || list[0].ID != w.meghivo.ID {
		t.Fatalf("EsemenyMeghivok = %+v, %v", list, err)
	}
}

func TestAVendegCantSeeTheCsoportsPlansNorRunTheEsemeny(t *testing.T) {
	w := newVendegWorld(t)
	ctx := context.Background()
	h := w.h

	_, err := h.App.EsemenyekOfCsoport(ctx, "vera", w.c.ID)
	expectErr(t, "the Csoport's Esemenyek", err, app.ErrNotFound)
	_, err = h.App.EsemenyRaeres(ctx, "vera", w.d.ID)
	expectErr(t, "the Esemeny's Raeres", err, app.ErrNotFound)
	_, err = h.App.CsoportHeatmap(ctx, "vera", w.c.ID, "2026-12-01", "2026-12-31")
	expectErr(t, "the Csoport's heatmap", err, app.ErrNotFound)
	_, err = h.App.AddIdopont(ctx, "vera", w.d.ID, app.IdopontInput{Start: "2026-12-10T20:00"})
	expectErr(t, "adding an Idopont", err, app.ErrNotFound)
	_, err = h.App.CopyEsemeny(ctx, "vera", w.d.ID)
	expectErr(t, "copying", err, app.ErrNotFound)
	expectErr(t, "confirming", h.App.ConfirmIdopont(ctx, "vera", w.d.ID, w.d.Idopontok[0].ID), app.ErrNotFound)
	expectErr(t, "switching a Kiegeszito", h.App.SetKiegeszito(ctx, "vera", w.d.ID, domain.Felszereles, true), app.ErrNotFound)
	_, err = h.App.AddVendeg(ctx, "vera", w.d.ID, "Barátom")
	expectErr(t, "adding a Vendeg", err, app.ErrNotFound)
	expectErr(t, "removing a Vendeg", h.App.RemoveVendeg(ctx, "vera", w.d.ID, w.nagyi.ID), app.ErrNotFound)

	// But Vera answers, and sees a Kiegeszito that is on.
	answer(t, h, "vera", w.d, 0, domain.Igen)
	switchKiegeszito(t, h, "anna", w.d.ID, domain.Felszereles, true)
	f, err := h.App.Felszereles(ctx, "vera", w.d.ID)
	if err != nil {
		t.Fatalf("Felszereles(vera): %v", err)
	}
	// In Egyeztetes only Tags add Tetelek.
	if f.CanAdd {
		t.Fatalf("a Vendeg may add a Tetel in Egyeztetes")
	}
	_, err = h.App.AddTetel(ctx, "vera", w.d.ID, app.TetelInput{Name: "Sátor"})
	expectErr(t, "a Vendeg adding a Tetel in Egyeztetes", err, app.ErrNotPermitted)
}

func TestAVendegsRaeresIsntShownToTheCsoport(t *testing.T) {
	w := newVendegWorld(t)
	ctx := context.Background()
	if err := w.h.App.SetRaeres(ctx, "vera", "2026-12-01", "2026-12-31", domain.Szabad); err != nil {
		t.Fatalf("SetRaeres(vera): %v", err)
	}
	if err := w.h.App.SetRaeres(ctx, "cecil", "2026-12-01", "2026-12-31", domain.Szabad); err != nil {
		t.Fatalf("SetRaeres(cecil): %v", err)
	}
	r, err := w.h.App.EsemenyRaeres(ctx, "anna", w.d.ID)
	if err != nil {
		t.Fatalf("EsemenyRaeres: %v", err)
	}
	if r[0].Szabad != 1 || r[0].Osszes != 3 {
		t.Fatalf("szabad aznap = %d/%d, want 1/3 (Tags only)", r[0].Szabad, r[0].Osszes)
	}
	days, err := w.h.App.CsoportHeatmap(ctx, "anna", w.c.ID, "2026-12-01", "2026-12-01")
	if err != nil || len(days) != 1 || days[0].Szabad != 1 || days[0].Osszes != 3 {
		t.Fatalf("heatmap = %+v, %v; want 1/3", days, err)
	}
}

func TestOnlyTheAddingTagAnswersForANameOnlyVendeg(t *testing.T) {
	w := newVendegWorld(t)
	ctx := context.Background()
	i := w.d.Idopontok[0].ID

	answerFor(t, w, "bela", w.nagyi, 0, domain.Talan)
	answerFor(t, w, "bela", w.nagyi, 0, domain.Igen) // and changes it
	expectErr(t, "another Tag", w.h.App.SetVendegVisszajelzes(ctx, "cecil", w.d.ID, i, w.nagyi.ID, domain.Nem), app.ErrNotPermitted)
	expectErr(t, "the Szervezo", w.h.App.SetVendegVisszajelzes(ctx, "anna", w.d.ID, i, w.nagyi.ID, domain.Nem), app.ErrNotPermitted)
	expectErr(t, "a Vendeg", w.h.App.SetVendegVisszajelzes(ctx, "vera", w.d.ID, i, w.nagyi.ID, domain.Nem), app.ErrNotFound)
	expectErr(t, "an unknown Vendeg", w.h.App.SetVendegVisszajelzes(ctx, "bela", w.d.ID, i, "no-such-vendeg", domain.Nem), app.ErrNotFound)
	vera := detail(t, w.h, "anna", w.d.ID).Vendegek[1]
	expectErr(t, "for a Felhasznalo Vendeg", w.h.App.SetVendegVisszajelzes(ctx, "bela", w.d.ID, i, vera.ID, domain.Nem), app.ErrNotFound)

	got := detail(t, w.h, "cecil", w.d.ID).Idopontok[0]
	if len(got.Visszajelzesek) != 1 {
		t.Fatalf("visszajelzesek = %+v", got.Visszajelzesek)
	}
	v := got.Visszajelzesek[0]
	if v.VendegID != w.nagyi.ID || v.FelhasznaloID != "" || v.FelhasznaloName != "Nagyi" || v.Valasz != domain.Igen || v.IsTag || !v.IsVendeg || v.VendegAddedBy != "bela" {
		t.Fatalf("Nagyi's visszajelzes = %+v", v)
	}
	// Bela answering for Nagyi isn't Bela's own answer.
	if s := detail(t, w.h, "bela", w.d.ID).Idopontok[0].Sajat; s != "" {
		t.Fatalf("Bela's own answer = %q, want none", s)
	}
}

func TestANameOnlyVendegIsRemovedByTheirTagOrTheSzervezo(t *testing.T) {
	w := newVendegWorld(t)
	ctx := context.Background()
	answerFor(t, w, "bela", w.nagyi, 0, domain.Igen)

	expectErr(t, "another Tag removing Nagyi", w.h.App.RemoveVendeg(ctx, "cecil", w.d.ID, w.nagyi.ID), app.ErrNotPermitted)
	if err := w.h.App.RemoveVendeg(ctx, "bela", w.d.ID, w.nagyi.ID); err != nil {
		t.Fatalf("Bela removing Nagyi: %v", err)
	}
	d := detail(t, w.h, "anna", w.d.ID)
	if len(d.Vendegek) != 1 || len(d.Idopontok[0].Visszajelzesek) != 0 {
		t.Fatalf("after removal: vendegek %+v, visszajelzesek %+v", d.Vendegek, d.Idopontok[0].Visszajelzesek)
	}
	expectErr(t, "removing again", w.h.App.RemoveVendeg(ctx, "bela", w.d.ID, w.nagyi.ID), app.ErrNotFound)

	// The Szervezo removes anyone's, and a Felhasznalo Vendeg too.
	kuzin, err := w.h.App.AddVendeg(ctx, "cecil", w.d.ID, "Kuzin")
	if err != nil {
		t.Fatalf("AddVendeg: %v", err)
	}
	if err := w.h.App.RemoveVendeg(ctx, "anna", w.d.ID, kuzin.ID); err != nil {
		t.Fatalf("the Szervezo removing Kuzin: %v", err)
	}
	answer(t, w.h, "vera", w.d, 0, domain.Igen)
	vera := detail(t, w.h, "anna", w.d.ID).Vendegek[0]
	expectErr(t, "a Tag removing Vera", w.h.App.RemoveVendeg(ctx, "bela", w.d.ID, vera.ID), app.ErrNotPermitted)
	if err := w.h.App.RemoveVendeg(ctx, "anna", w.d.ID, vera.ID); err != nil {
		t.Fatalf("the Szervezo removing Vera: %v", err)
	}
	_, err = w.h.App.Esemeny(ctx, "vera", w.d.ID)
	expectErr(t, "Vera's view after removal", err, app.ErrNotFound)
	d = detail(t, w.h, "anna", w.d.ID)
	if len(d.Vendegek) != 0 || len(d.Idopontok[0].Visszajelzesek) != 0 {
		t.Fatalf("after removing Vera: vendegek %+v, visszajelzesek %+v", d.Vendegek, d.Idopontok[0].Visszajelzesek)
	}
	_, err = w.h.App.AddVendeg(ctx, "bela", w.d.ID, "   ")
	expectErr(t, "an empty name", err, app.ErrInvalidInput)
}

func TestHelyezesIgnoresVendegekButTheHeadcountIncludesThem(t *testing.T) {
	w := newVendegWorld(t)
	joinAsVendeg(t, w.h, "zoli", "Zoli", w.meghivo.Token)
	// Two Tags on the first Idopont, one Tag and three Vendegek on the
	// second: the Csoport prefers the first.
	answer(t, w.h, "anna", w.d, 0, domain.Igen)
	answer(t, w.h, "bela", w.d, 0, domain.Igen)
	answer(t, w.h, "cecil", w.d, 1, domain.Igen)
	answer(t, w.h, "vera", w.d, 1, domain.Igen)
	answer(t, w.h, "zoli", w.d, 1, domain.Talan)
	answerFor(t, w, "bela", w.nagyi, 1, domain.Igen)

	d := detail(t, w.h, "anna", w.d.ID)
	expectHelyezesek(t, d, 1, 2)
	second := d.Idopontok[1]
	if second.Igen != 1 || second.Talan != 0 || second.VendegIgen != 2 || second.VendegTalan != 1 {
		t.Fatalf("second Idopont: Tags %d/%d, Vendegek %d/%d; want 1/0 and 2/1", second.Igen, second.Talan, second.VendegIgen, second.VendegTalan)
	}
	if got := detail(t, w.h, "vera", w.d.ID).Idopontok[1].Sajat; got != domain.Igen {
		t.Fatalf("Vera's own answer = %q", got)
	}

	// Confirming the second: both kinds of Vendeg with igen are Resztvevok.
	confirm(t, w.h, "anna", d, 1)
	if got := resztvevoNames(detail(t, w.h, "vera", w.d.ID)); got != "Nagyi* Vera* cecil" {
		t.Fatalf("resztvevok = %q", got)
	}
	// ... and the Discord message names them as Vendegek.
	edits := w.h.Discord.DMs()
	var last []string
	for _, dm := range edits {
		if dm.Esemeny != nil {
			last = dm.Esemeny.Resztvevok
		}
	}
	for _, e := range w.h.Discord.Edits() {
		last = e.Message.Resztvevok
	}
	if strings.Join(last, ", ") != "Nagyi (vendég), Vera (vendég), cecil" {
		t.Fatalf("Discord resztvevok = %q", last)
	}
}

func TestVendegekClaimInFelszereles(t *testing.T) {
	w := newVendegWorld(t)
	ctx := context.Background()
	h := w.h
	switchKiegeszito(t, h, "anna", w.d.ID, domain.Felszereles, true)
	w = w.confirmed(t)
	kuzin, err := h.App.AddVendeg(ctx, "bela", w.d.ID, "Kuzin") // no answer: no Resztvevo
	if err != nil {
		t.Fatalf("AddVendeg: %v", err)
	}

	// A Felhasznalo Vendeg who is a Resztvevo adds and claims for themselves.
	sator, err := h.App.AddTetel(ctx, "vera", w.d.ID, app.TetelInput{Name: "Sátor", Quantity: 4})
	if err != nil {
		t.Fatalf("Vera adding a Tetel: %v", err)
	}
	if err := h.App.SetVallalas(ctx, "vera", w.d.ID, sator.ID, app.VallalasInput{Quantity: 1}); err != nil {
		t.Fatalf("Vera claiming: %v", err)
	}
	// Bela claims for Nagyi; nobody else may.
	if err := h.App.SetVallalas(ctx, "bela", w.d.ID, sator.ID, app.VallalasInput{Quantity: 2, VendegID: w.nagyi.ID}); err != nil {
		t.Fatalf("Bela claiming for Nagyi: %v", err)
	}
	expectErr(t, "Cecil claiming for Nagyi", h.App.SetVallalas(ctx, "cecil", w.d.ID, sator.ID, app.VallalasInput{Quantity: 1, VendegID: w.nagyi.ID}), app.ErrNotPermitted)
	expectErr(t, "Vera claiming for Nagyi", h.App.SetVallalas(ctx, "vera", w.d.ID, sator.ID, app.VallalasInput{Quantity: 1, VendegID: w.nagyi.ID}), app.ErrNotPermitted)
	expectErr(t, "Bela claiming for a Vendeg who isn't a Resztvevo", h.App.SetVallalas(ctx, "bela", w.d.ID, sator.ID, app.VallalasInput{Quantity: 1, VendegID: kuzin.ID}), app.ErrNotResztvevo)

	f, err := h.App.Felszereles(ctx, "vera", w.d.ID)
	if err != nil {
		t.Fatalf("Felszereles: %v", err)
	}
	var got []string
	for _, v := range f.Tetelek[0].Vallalasok {
		got = append(got, fmt.Sprintf("%s=%d", v.Name, v.Quantity))
	}
	if strings.Join(got, " ") != "Nagyi=2 Vera=1" || f.Tetelek[0].Sajat != 1 || f.Hianyzik != 1 || !f.CanClaim {
		t.Fatalf("vallalasok = %v, sajat %d, hianyzik %d, canClaim %v", got, f.Tetelek[0].Sajat, f.Hianyzik, f.CanClaim)
	}

	// Nagyi dropping out (Bela answers nem) releases her claim and tells the
	// Szervezo.
	answerFor(t, w, "bela", w.nagyi, 0, domain.Nem)
	f, _ = h.App.Felszereles(ctx, "anna", w.d.ID)
	if len(f.Tetelek[0].Vallalasok) != 1 || f.Hianyzik != 3 {
		t.Fatalf("after Nagyi dropped out: %+v", f.Tetelek[0].Vallalasok)
	}
	drops := dropOutDMs(h)
	if len(drops) != 1 || drops[0].DiscordID != "discord-anna" || drops[0].DropOut.Name != "Nagyi" || drops[0].DropOut.Felszabadult != "2 db Sátor" {
		t.Fatalf("drop-out DMs = %+v", drops)
	}
}

func TestVendegekClaimSeatsAndBeds(t *testing.T) {
	w := newVendegWorld(t)
	ctx := context.Background()
	h := w.h
	switchKiegeszito(t, h, "anna", w.d.ID, domain.Autok, true)
	switchKiegeszito(t, h, "anna", w.d.ID, domain.Szallas, true)
	w = w.confirmed(t)

	// Autok: Vera drives, Bela seats Nagyi in her Auto and Cecil can't.
	auto, err := h.App.OfferAuto(ctx, "vera", w.d.ID, app.AutoInput{Seats: 2})
	if err != nil {
		t.Fatalf("Vera offering an Auto: %v", err)
	}
	if err := h.App.ClaimUles(ctx, "bela", w.d.ID, auto.ID, app.UtasInput{VendegID: w.nagyi.ID}); err != nil {
		t.Fatalf("Bela seating Nagyi: %v", err)
	}
	expectErr(t, "Cecil unseating Nagyi", h.App.ReleaseUles(ctx, "cecil", w.d.ID, app.UtasInput{VendegID: w.nagyi.ID}), app.ErrNotPermitted)
	autok := autokOf(t, h, "anna", w.d.ID)
	if len(autok.Autok) != 1 || autok.Autok[0].SoforName != "Vera" || len(autok.Autok[0].Utasok) != 1 || autok.Autok[0].Utasok[0].Name != "Nagyi" {
		t.Fatalf("autok = %+v", autok.Autok)
	}
	var helyNelkul []string
	for _, r := range autok.HelyNelkul {
		helyNelkul = append(helyNelkul, r.Name)
	}
	if strings.Join(helyNelkul, " ") != "Anna bela" {
		t.Fatalf("hely nelkul = %v", helyNelkul)
	}

	// Szallas: Bela gives Nagyi a bed, Vera takes one herself.
	szoba := addSzoba(t, h, w.d.ID, "Nappali")
	agy := addAgy(t, h, w.d.ID, szoba.ID, 2)
	if err := h.App.ClaimAgy(ctx, "bela", w.d.ID, agy.ID, app.FoglalasInput{VendegID: w.nagyi.ID}); err != nil {
		t.Fatalf("Bela giving Nagyi a bed: %v", err)
	}
	if err := h.App.ClaimAgy(ctx, "vera", w.d.ID, agy.ID, app.FoglalasInput{}); err != nil {
		t.Fatalf("Vera taking a bed: %v", err)
	}
	expectErr(t, "Cecil moving Nagyi", h.App.ClaimAgy(ctx, "cecil", w.d.ID, agy.ID, app.FoglalasInput{VendegID: w.nagyi.ID}), app.ErrNotPermitted)
	z := szallasOf(t, h, "anna", w.d.ID)
	var alvok []string
	for _, g := range z.Szobak[0].Agyak[0].Alvok {
		alvok = append(alvok, g.Name)
	}
	if strings.Join(alvok, " ") != "Nagyi Vera" {
		t.Fatalf("alvok = %v", alvok)
	}

	// Vera dropping out removes her Auto: Nagyi has no Discord, so nobody
	// gets the passengers' DM, and her bed is freed.
	answer(t, h, "vera", w.d, 0, domain.Nem)
	if a := autokOf(t, h, "anna", w.d.ID); len(a.Autok) != 0 {
		t.Fatalf("autok after Vera dropped out = %+v", a.Autok)
	}
	if dms := autoDMs(h); len(dms) != 0 {
		t.Fatalf("auto DMs = %+v, want none for a name-only Vendeg", dms)
	}
	if n := len(szallasOf(t, h, "anna", w.d.ID).Szobak[0].Agyak[0].Alvok); n != 1 {
		t.Fatalf("alvok after Vera dropped out = %d, want 1", n)
	}
}

func TestAFelhasznaloVendegPassengerIsToldTheirAutoIsGone(t *testing.T) {
	w := newVendegWorld(t)
	ctx := context.Background()
	switchKiegeszito(t, w.h, "anna", w.d.ID, domain.Autok, true)
	w = w.confirmed(t)
	auto, err := w.h.App.OfferAuto(ctx, "bela", w.d.ID, app.AutoInput{Seats: 3})
	if err != nil {
		t.Fatalf("OfferAuto: %v", err)
	}
	autoClaim(t, w.h, "vera", w.d.ID, auto.ID)
	if err := w.h.App.DeleteAuto(ctx, "bela", w.d.ID, auto.ID); err != nil {
		t.Fatalf("DeleteAuto: %v", err)
	}
	dms := autoDMs(w.h)
	if len(dms) != 1 || dms[0].DiscordID != "discord-vera" {
		t.Fatalf("auto DMs = %+v, want one to Vera", dms)
	}
}

func TestANameOnlyVendegsShareIsChargedToTheirTag(t *testing.T) {
	w := newVendegWorld(t)
	ctx := context.Background()
	switchKiegeszito(t, w.h, "anna", w.d.ID, domain.Koltsegek, true)
	w = w.confirmed(t)

	// Split among every Resztvevo by default: Anna, Bela, Vera and Nagyi.
	k := addKiadas(t, w.h, "anna", w.d.ID, app.KiadasInput{Description: "Szállás", Amount: 4000})
	// Vera, a Vendeg, records one too; Bela splits one with Nagyi only.
	addKiadas(t, w.h, "vera", w.d.ID, app.KiadasInput{Description: "Bor", Amount: 600, Split: &app.SplitInput{Holders: holders("anna", "vera")}})
	nagyi := []domain.ClaimHolder{{VendegID: w.nagyi.ID}}
	addKiadas(t, w.h, "anna", w.d.ID, app.KiadasInput{Description: "Fagyi", Amount: 200, PayerID: "bela", Split: &app.SplitInput{Holders: nagyi}})
	expectErr(t, "Cecil, no Resztvevo, adding one", func() error {
		_, err := w.h.App.AddKiadas(ctx, "cecil", w.d.ID, app.KiadasInput{Description: "x", Amount: 1})
		return err
	}(), app.ErrNotResztvevo)

	got := koltsegek(t, w.h, "vera", w.d.ID)
	var parts []string
	for _, r := range kiadasAllas(t, got, k.ID).Reszek {
		parts = append(parts, fmt.Sprintf("%s=%d", r.Name, r.Amount))
	}
	if strings.Join(parts, " ") != "Anna=1000 bela=1000 Vera=1000 Nagyi=1000" {
		t.Fatalf("the split = %v", parts)
	}
	// Bela pays Nagyi's 1000 and her 200 of the Fagyi he paid himself.
	if e := egyenlegek(got); e != "anna:4000/1300/2700 vera:600/1300/-700 bela:200/2200/-2000" {
		t.Fatalf("egyenlegek = %s", e)
	}
	expectErr(t, "removing Nagyi while she has a share", w.h.App.RemoveVendeg(ctx, "bela", w.d.ID, w.nagyi.ID), app.ErrVendegHasKiadas)
	vera := detail(t, w.h, "anna", w.d.ID).Vendegek[1]
	expectErr(t, "removing Vera while she paid one", w.h.App.RemoveVendeg(ctx, "anna", w.d.ID, vera.ID), app.ErrVendegHasKiadas)
}

func TestAnElszamolasWithVendegek(t *testing.T) {
	w := newVendegWorld(t)
	ctx := context.Background()
	switchKiegeszito(t, w.h, "anna", w.d.ID, domain.Koltsegek, true)
	w = w.confirmed(t)
	addKiadas(t, w.h, "anna", w.d.ID, app.KiadasInput{Description: "Szállás", Amount: 4000})
	setLezajlott(t, w.h, w.d)

	e := elszamolasOf(t, w.h, "bela", w.d.ID)
	if names(e.Hianyzok) != "Anna Nagyi Vera bela" || !e.CanConfirm {
		t.Fatalf("before: hianyzok %q, Bela canConfirm %v", names(e.Hianyzok), e.CanConfirm)
	}
	// Cecil is no Resztvevo and added nobody.
	expectErr(t, "Cecil confirming", w.h.App.ConfirmKiadasokRecorded(ctx, "cecil", w.d.ID), app.ErrNotResztvevo)

	// Bela's press covers Nagyi; taking it back takes hers back too.
	confirmKiadasok(t, w.h, w.d.ID, "bela")
	if e := elszamolasOf(t, w.h, "bela", w.d.ID); names(e.Megerositettek) != "Nagyi bela" || !e.Megerositette || !e.CanUnconfirm {
		t.Fatalf("after Bela: megerositettek %q, %+v", names(e.Megerositettek), e)
	}
	if err := w.h.App.UnconfirmKiadasokRecorded(ctx, "bela", w.d.ID); err != nil {
		t.Fatalf("UnconfirmKiadasokRecorded: %v", err)
	}
	if e := elszamolasOf(t, w.h, "bela", w.d.ID); names(e.Megerositettek) != "" {
		t.Fatalf("after Bela took it back: megerositettek %q", names(e.Megerositettek))
	}

	// Vera confirms for herself; the last one calculates it.
	confirmKiadasok(t, w.h, w.d.ID, "bela", "vera", "anna")
	k := koltsegek(t, w.h, "vera", w.d.ID)
	if k.Elszamolas.Allapot != app.ElszamolasKiszamolva {
		t.Fatalf("allapot = %s", k.Elszamolas.Allapot)
	}
	if got := utalasok(k.Elszamolas); got != "bela>anna:2000 vera>anna:1000" {
		t.Fatalf("utalasok = %s", got)
	}
	expectBalanced(t, k)
	// Vera, a Vendeg, gets her result DM; Nagyi has none to get.
	dms := elszamolasDMs(w.h, "discord-vera")
	if len(dms) != 1 || len(dms[0].Fizetendo) != 1 || dms[0].Fizetendo[0].Name != "Anna" || dms[0].Fizetendo[0].Amount != 1000 {
		t.Fatalf("Vera's elszamolas DMs = %+v", dms)
	}
	if dms := elszamolasDMs(w.h, "discord-bela"); len(dms) != 1 || dms[0].Fizetendo[0].Amount != 2000 {
		t.Fatalf("Bela's elszamolas DMs = %+v", dms)
	}
	// Vera, a party of her Utalas, marks it megvolt.
	if err := w.h.App.SetUtalasMegvolt(ctx, "vera", w.d.ID, k.Elszamolas.Utalasok[1].ID, true); err != nil {
		t.Fatalf("SetUtalasMegvolt(vera): %v", err)
	}
}

func TestATagConfirmsForTheirNameOnlyVendegEvenIfTheyStayHome(t *testing.T) {
	w := newVendegWorld(t)
	switchKiegeszito(t, w.h, "anna", w.d.ID, domain.Koltsegek, true)
	// Cecil says nem himself but brings his Kuzin.
	kuzin, err := w.h.App.AddVendeg(context.Background(), "cecil", w.d.ID, "Kuzin")
	if err != nil {
		t.Fatalf("AddVendeg: %v", err)
	}
	answer(t, w.h, "anna", w.d, 0, domain.Igen)
	answer(t, w.h, "cecil", w.d, 0, domain.Nem)
	answerFor(t, w, "cecil", kuzin, 0, domain.Igen)
	confirm(t, w.h, "anna", w.d, 0)
	addKiadas(t, w.h, "anna", w.d.ID, app.KiadasInput{Description: "Pizza", Amount: 3000})
	setLezajlott(t, w.h, w.d)

	if e := elszamolasOf(t, w.h, "cecil", w.d.ID); !e.CanConfirm {
		t.Fatalf("Cecil can't confirm for Kuzin: %+v", e)
	}
	confirmKiadasok(t, w.h, w.d.ID, "cecil", "anna")
	k := koltsegek(t, w.h, "cecil", w.d.ID)
	if got := utalasok(k.Elszamolas); got != "cecil>anna:1500" {
		t.Fatalf("utalasok = %s", got)
	}
}

func TestTheElszamolasReminderGoesToTheTagOfAnUnconfirmedNameOnlyVendeg(t *testing.T) {
	w := newVendegWorld(t)
	switchKiegeszito(t, w.h, "anna", w.d.ID, domain.Koltsegek, true)
	w = w.confirmed(t)
	runAt(t, w.h, firstEnd) // Lezajlott
	addKiadas(t, w.h, "anna", w.d.ID, app.KiadasInput{Description: "Szállás", Amount: 4000})
	confirmKiadasok(t, w.h, w.d.ID, "anna")

	runAt(t, w.h, firstEnd.Add(72*time.Hour))
	// Nagyi (Bela's) and Vera, by name; Bela once for himself and Nagyi.
	expectLines(t, "reminders", elszamolasEmlekeztetok(w.h), "discord-bela", "discord-vera")
}

func TestEmlekeztetokGoToVendegekWithIgenOrTalan(t *testing.T) {
	w := newVendegWorld(t)
	joinAsVendeg(t, w.h, "zoli", "Zoli", w.meghivo.Token)
	joinAsVendeg(t, w.h, "xena", "Xena", w.meghivo.Token)
	answer(t, w.h, "anna", w.d, 0, domain.Igen)
	answer(t, w.h, "vera", w.d, 0, domain.Igen)
	answer(t, w.h, "zoli", w.d, 0, domain.Talan)
	answer(t, w.h, "xena", w.d, 0, domain.Nem)
	answerFor(t, w, "bela", w.nagyi, 0, domain.Igen)
	confirm(t, w.h, "anna", w.d, 0)

	runAt(t, w.h, firstStart.Add(-24*time.Hour))
	label := "dec. 1. (kedd) 20:00–23:00"
	expectLines(t, "emlekeztetok", emlekeztetoDMs(w.h),
		"discord-anna: "+label, "discord-vera: "+label, "discord-zoli: "+label)
}

func TestStateChangesReachVendegekByDM(t *testing.T) {
	w := newVendegWorld(t)
	before := len(w.h.Discord.DMsTo("discord-vera"))
	confirm(t, w.h, "anna", w.d, 0)
	cancelEsemeny(t, w.h, "anna", w.d.ID)

	var headlines []string
	for _, dm := range w.h.Discord.DMsTo("discord-vera")[before:] {
		if dm.Announcement == nil {
			t.Fatalf("Vera got a DM that isn't an announcement: %+v", dm)
		}
		headlines = append(headlines, dm.Announcement.Headline)
	}
	expectLines(t, "Vera's announcements", headlines, "✅ Véglegesítve", "❌ Elmaradt")
	// No vote request ever reaches a Vendeg.
	for _, dm := range w.h.Discord.DMsTo("discord-vera") {
		if dm.VoteRequest != nil {
			t.Fatalf("Vera got a vote request: %+v", dm)
		}
	}
}

func TestAVendegCantBecomeTheSzervezo(t *testing.T) {
	w := newVendegWorld(t)
	answer(t, w.h, "vera", w.d, 0, domain.Igen)
	answerFor(t, w, "bela", w.nagyi, 0, domain.Igen)
	expectErr(t, "handing over to Vera", w.h.App.HandOverSzervezo(context.Background(), "anna", w.d.ID, "vera"), app.ErrNotEligibleSzervezo)
}
