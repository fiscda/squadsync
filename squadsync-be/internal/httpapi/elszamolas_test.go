package httpapi_test

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"testing"
	"time"

	"squadsync-be/internal/apptest"
	"squadsync-be/internal/httpapi"
)

type elszamolasResp struct {
	Elszamolas *struct {
		Allapot        string `json:"allapot"`
		Megerositettek []struct {
			FelhasznaloID string `json:"felhasznaloId"`
			Name          string `json:"name"`
		} `json:"megerositettek"`
		Hianyzok []struct {
			FelhasznaloID string `json:"felhasznaloId"`
			Name          string `json:"name"`
		} `json:"hianyzok"`
		Megerositette   bool       `json:"megerositette"`
		CanConfirm      bool       `json:"canConfirm"`
		TervezettFizeto []string   `json:"tervezettFizeto"`
		CanUnconfirm    bool       `json:"canUnconfirm"`
		CanClose        bool       `json:"canClose"`
		CanReopen       bool       `json:"canReopen"`
		KiszamolvaAt    *time.Time `json:"kiszamolvaAt"`
		LezartaName     string     `json:"lezartaName"`
		Utalasok        []struct {
			ID            string     `json:"id"`
			FromID        string     `json:"fromId"`
			FromName      string     `json:"fromName"`
			ToID          string     `json:"toId"`
			ToName        string     `json:"toName"`
			Amount        int        `json:"amount"`
			MegvoltAt     *time.Time `json:"megvoltAt"`
			MegvoltByName string     `json:"megvoltByName"`
			CanMark       bool       `json:"canMark"`
		} `json:"utalasok"`
	} `json:"elszamolas"`
}

func TestElszamolasOverHTTP(t *testing.T) {
	h := apptest.New(t)
	api := httpapi.New(h.App, secret, slog.New(slog.NewTextHandler(io.Discard, nil)))
	login(t, api, "anna", "111", "Anna")
	login(t, api, "bela", "222", "Béla")
	login(t, api, "dani", "444", "Dani")
	c := decode[csoportResp](t, call(t, api, http.MethodPost, "/csoport", `{"name":"Péntek esték"}`, as("anna")))
	m := decode[meghivoResp](t, call(t, api, http.MethodPost, "/csoport/"+c.ID+"/meghivo", "", as("anna")))
	login(t, api, "eve", "555", "Éva")
	for _, who := range []string{"bela", "dani"} {
		expectStatus(t, call(t, api, http.MethodPost, "/csoport-meghivo/"+m.Token+"/join", "", as(who)), http.StatusOK)
	}
	e := decode[esemenyResp](t, call(t, api, http.MethodPost, "/csoport/"+c.ID+"/esemeny", proposal, as("anna")))
	d := decode[allapotResp](t, call(t, api, http.MethodGet, "/esemeny/"+e.ID, "", as("anna")))
	esemeny := "/esemeny/" + e.ID
	base := esemeny + "/koltsegek"
	expectCoded := func(method, path, body, who string, status int, code string) {
		t.Helper()
		rec := call(t, api, method, path, body, as(who))
		expectStatus(t, rec, status)
		if got := decode[errorResp](t, rec); got.Code != code {
			t.Fatalf("%s %s as %s: code = %q, want %q", method, path, who, got.Code, code)
		}
	}
	get := func(who string) elszamolasResp {
		t.Helper()
		return decode[elszamolasResp](t, call(t, api, http.MethodGet, base, "", as(who)))
	}

	expectStatus(t, call(t, api, http.MethodPut, esemeny+"/kiegeszito/koltsegek", `{"enabled":true}`, as("anna")), http.StatusNoContent)
	for _, who := range []string{"anna", "bela"} {
		expectStatus(t, call(t, api, http.MethodPut, esemeny+"/idopont/"+d.Idopontok[0].ID+"/visszajelzes", `{"valasz":"igen"}`, as(who)), http.StatusNoContent)
	}
	expectStatus(t, call(t, api, http.MethodPost, esemeny+"/veglegesites", `{"idopontId":"`+d.Idopontok[0].ID+`"}`, as("anna")), http.StatusNoContent)
	if r := get("anna"); r.Elszamolas != nil {
		t.Fatalf("elszamolas without a Kiadas = %+v, want null", r.Elszamolas)
	}
	expectStatus(t, call(t, api, http.MethodPost, base+"/kiadas", `{"description":"Pizza","amount":8000}`, as("anna")), http.StatusCreated)
	expectStatus(t, call(t, api, http.MethodPost, base+"/kiadas", `{"description":"Szállás","amount":60000,"allapot":"tervezett"}`, as("bela")), http.StatusCreated)
	if r := get("anna"); r.Elszamolas == nil || r.Elszamolas.Allapot != "lezajlas_elott" {
		t.Fatalf("elszamolas before Lezajlott = %+v", r.Elszamolas)
	}
	expectCoded(http.MethodPost, base+"/elszamolas/megerosites", "", "anna", http.StatusConflict, "koltsegek_elszamolas_not_open")

	// The Idopont ends on 2026-12-05 at 23:00 in Budapest.
	h.Clock.Set(time.Date(2026, time.December, 5, 22, 0, 0, 0, time.UTC))
	if err := h.App.RunDueJobs(context.Background()); err != nil {
		t.Fatalf("RunDueJobs: %v", err)
	}

	expectCoded(http.MethodPost, base+"/elszamolas/megerosites", "", "dani", http.StatusForbidden, "koltsegek_elszamolas_not_resztvevo")
	expectCoded(http.MethodPost, base+"/elszamolas/megerosites", "", "bela", http.StatusConflict, "koltsegek_elszamolas_tervezett")
	if r := get("bela"); len(r.Elszamolas.TervezettFizeto) != 1 || r.Elszamolas.TervezettFizeto[0] != "Szállás" || !r.Elszamolas.CanConfirm {
		t.Fatalf("Bela's view = %+v", r.Elszamolas)
	}
	expectStatus(t, call(t, api, http.MethodPost, base+"/elszamolas/megerosites", "", as("anna")), http.StatusNoContent)
	expectStatus(t, call(t, api, http.MethodDelete, base+"/elszamolas/megerosites", "", as("anna")), http.StatusNoContent)
	expectStatus(t, call(t, api, http.MethodPost, base+"/elszamolas/megerosites", "", as("anna")), http.StatusNoContent)
	r := get("anna")
	if r.Elszamolas.Allapot != "megerosites" || !r.Elszamolas.Megerositette || !r.Elszamolas.CanClose ||
		len(r.Elszamolas.Hianyzok) != 1 || r.Elszamolas.Hianyzok[0].Name != "Béla" || len(r.Elszamolas.Megerositettek) != 1 {
		t.Fatalf("Anna's view = %+v", r.Elszamolas)
	}

	// Closing early, without Bela.
	expectCoded(http.MethodPost, base+"/elszamolas/lezaras", "", "bela", http.StatusForbidden, "koltsegek_not_szervezo")
	expectCoded(http.MethodPost, base+"/elszamolas/ujranyitas", "", "anna", http.StatusConflict, "koltsegek_elszamolas_not_kiszamolva")
	expectStatus(t, call(t, api, http.MethodPost, base+"/elszamolas/lezaras", "", as("anna")), http.StatusNoContent)
	expectCoded(http.MethodPost, base+"/elszamolas/lezaras", "", "anna", http.StatusConflict, "koltsegek_elszamolas_kiszamolva")
	expectCoded(http.MethodPost, base+"/kiadas", `{"description":"Bor","amount":100}`, "anna", http.StatusConflict, "koltsegek_locked")
	r = get("bela")
	if r.Elszamolas.Allapot != "kiszamolva" || r.Elszamolas.KiszamolvaAt == nil || r.Elszamolas.LezartaName != "Anna" || len(r.Elszamolas.Utalasok) != 1 {
		t.Fatalf("after closing = %+v", r.Elszamolas)
	}
	u := r.Elszamolas.Utalasok[0]
	if u.FromID != "bela" || u.FromName != "Béla" || u.ToID != "anna" || u.ToName != "Anna" || u.Amount != 4000 || u.MegvoltAt != nil || !u.CanMark {
		t.Fatalf("utalas = %+v", u)
	}

	// Reopening and closing again.
	expectCoded(http.MethodPost, base+"/elszamolas/ujranyitas", "", "bela", http.StatusForbidden, "koltsegek_not_szervezo")
	expectStatus(t, call(t, api, http.MethodPost, base+"/elszamolas/ujranyitas", "", as("anna")), http.StatusNoContent)
	if r := get("anna"); r.Elszamolas.Allapot != "megerosites" || len(r.Elszamolas.Megerositettek) != 0 || len(r.Elszamolas.Utalasok) != 0 {
		t.Fatalf("after reopening = %+v", r.Elszamolas)
	}
	expectStatus(t, call(t, api, http.MethodPost, base+"/elszamolas/lezaras", "", as("anna")), http.StatusNoContent)
	u2 := get("anna").Elszamolas.Utalasok[0]

	// Marking it.
	megvolt := base + "/elszamolas/utalas/" + u2.ID + "/megvolt"
	expectCoded(http.MethodPut, megvolt, `{"megvolt":true}`, "dani", http.StatusForbidden, "not_permitted")
	expectStatus(t, call(t, api, http.MethodPut, megvolt, `{"megvolt":true}`, as("eve")), http.StatusNotFound)
	expectStatus(t, call(t, api, http.MethodPut, megvolt, `nope`, as("bela")), http.StatusBadRequest)
	expectStatus(t, call(t, api, http.MethodPut, base+"/elszamolas/utalas/nope/megvolt", `{"megvolt":true}`, as("bela")), http.StatusNotFound)
	expectStatus(t, call(t, api, http.MethodPut, megvolt, `{"megvolt":true}`, as("bela")), http.StatusNoContent)
	r = get("anna")
	if r.Elszamolas.Allapot != "elszamolva" || r.Elszamolas.Utalasok[0].MegvoltAt == nil || r.Elszamolas.Utalasok[0].MegvoltByName != "Béla" || r.Elszamolas.CanReopen {
		t.Fatalf("after megvolt = %+v", r.Elszamolas)
	}
	expectCoded(http.MethodPut, megvolt, `{"megvolt":false}`, "anna", http.StatusConflict, "koltsegek_elszamolva")
	expectCoded(http.MethodPost, base+"/elszamolas/ujranyitas", "", "anna", http.StatusConflict, "koltsegek_elszamolas_megvolt")
}
