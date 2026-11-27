package httpapi_test

import (
	"net/http"
	"testing"
)

type kiegeszitokResp struct {
	Kiegeszitok []struct {
		Kind       string `json:"kind"`
		Enabled    bool   `json:"enabled"`
		Switchable bool   `json:"switchable"`
	} `json:"kiegeszitok"`
}

type felszerelesResp struct {
	Tetelek []struct {
		ID            string `json:"id"`
		Name          string `json:"name"`
		Quantity      int    `json:"quantity"`
		MindenkiHozza bool   `json:"mindenkiHozza"`
		Vallalt       int    `json:"vallalt"`
		Hianyzik      int    `json:"hianyzik"`
		Sajat         int    `json:"sajat"`
		CanEdit       bool   `json:"canEdit"`
		Vallalasok    []struct {
			FelhasznaloID string `json:"felhasznaloId"`
			Name          string `json:"name"`
			Quantity      int    `json:"quantity"`
		} `json:"vallalasok"`
	} `json:"tetelek"`
	Hianyzik    int      `json:"hianyzik"`
	Emlekezteto []string `json:"emlekezteto"`
	CanAdd      bool     `json:"canAdd"`
	CanClaim    bool     `json:"canClaim"`
}

func TestSwitchingKiegeszitokOverHTTP(t *testing.T) {
	api, esemenyID, _ := lifecycleSetup(t)
	path := "/esemeny/" + esemenyID + "/kiegeszito/felszereles"

	d := decode[kiegeszitokResp](t, call(t, api, http.MethodGet, "/esemeny/"+esemenyID, "", as("bela")))
	if len(d.Kiegeszitok) != 2 || d.Kiegeszitok[0].Kind != "koltsegek" || d.Kiegeszitok[1].Kind != "felszereles" || d.Kiegeszitok[1].Enabled || !d.Kiegeszitok[1].Switchable {
		t.Fatalf("kiegeszitok = %+v, want Felszereles off and switchable", d.Kiegeszitok)
	}
	expectStatus(t, call(t, api, http.MethodGet, "/esemeny/"+esemenyID+"/felszereles", "", as("bela")), http.StatusConflict)

	expectStatus(t, call(t, api, http.MethodPut, path, `{"enabled":true}`, as("bela")), http.StatusForbidden)
	expectStatus(t, call(t, api, http.MethodPut, path, `{"enabled":true}`, as("dani")), http.StatusNotFound)
	expectStatus(t, call(t, api, http.MethodPut, "/esemeny/"+esemenyID+"/kiegeszito/sorok", `{"enabled":true}`, as("anna")), http.StatusBadRequest)
	expectStatus(t, call(t, api, http.MethodPut, path, `nope`, as("anna")), http.StatusBadRequest)
	expectStatus(t, call(t, api, http.MethodPut, path, `{"enabled":true}`, as("anna")), http.StatusNoContent)

	d = decode[kiegeszitokResp](t, call(t, api, http.MethodGet, "/esemeny/"+esemenyID, "", as("bela")))
	if !d.Kiegeszitok[1].Enabled {
		t.Fatalf("kiegeszitok = %+v, want Felszereles on", d.Kiegeszitok)
	}

	expectStatus(t, call(t, api, http.MethodPost, "/esemeny/"+esemenyID+"/lemondas", "", as("anna")), http.StatusNoContent)
	rec := call(t, api, http.MethodPut, path, `{"enabled":false}`, as("anna"))
	expectStatus(t, rec, http.StatusConflict)
	if e := decode[errorResp](t, rec); e.Code != "kiegeszito_locked" {
		t.Fatalf("code = %q, want kiegeszito_locked", e.Code)
	}
}

func TestFelszerelesOverHTTP(t *testing.T) {
	api, esemenyID, idopontok := lifecycleSetup(t)
	base := "/esemeny/" + esemenyID + "/felszereles"
	expectStatus(t, call(t, api, http.MethodPut, "/esemeny/"+esemenyID+"/kiegeszito/felszereles", `{"enabled":true}`, as("anna")), http.StatusNoContent)

	// Setup in Egyeztetes, by any Tag.
	rec := call(t, api, http.MethodPost, base+"/tetel", `{"name":"sátor","quantity":3}`, as("bela"))
	expectStatus(t, rec, http.StatusCreated)
	sator := decode[struct {
		ID       string `json:"id"`
		Quantity int    `json:"quantity"`
	}](t, rec)
	if sator.Quantity != 3 {
		t.Fatalf("tetel = %+v", sator)
	}
	expectStatus(t, call(t, api, http.MethodPost, base+"/tetel", `{"name":"hálózsák","mindenkiHozza":true}`, as("anna")), http.StatusCreated)
	expectStatus(t, call(t, api, http.MethodPost, base+"/tetel", `{"name":""}`, as("anna")), http.StatusBadRequest)
	expectStatus(t, call(t, api, http.MethodPost, base+"/tetel", `{"name":"x"}`, as("dani")), http.StatusNotFound)
	vallalas := base + "/tetel/" + sator.ID + "/vallalas"
	rec = call(t, api, http.MethodPut, vallalas, `{"quantity":1}`, as("bela"))
	expectStatus(t, rec, http.StatusConflict)
	if e := decode[errorResp](t, rec); e.Code != "claiming_closed" {
		t.Fatalf("code = %q, want claiming_closed", e.Code)
	}

	expectStatus(t, call(t, api, http.MethodPut, "/esemeny/"+esemenyID+"/idopont/"+idopontok[0]+"/visszajelzes", `{"valasz":"igen"}`, as("bela")), http.StatusNoContent)
	expectStatus(t, call(t, api, http.MethodPost, "/esemeny/"+esemenyID+"/veglegesites", `{"idopontId":"`+idopontok[0]+`"}`, as("anna")), http.StatusNoContent)

	// Claiming by a Resztvevo.
	expectStatus(t, call(t, api, http.MethodPut, vallalas, `{"quantity":2}`, as("bela")), http.StatusNoContent)
	rec = call(t, api, http.MethodPut, vallalas, `{"quantity":1}`, as("anna"))
	expectStatus(t, rec, http.StatusForbidden)
	if e := decode[errorResp](t, rec); e.Code != "not_resztvevo" {
		t.Fatalf("code = %q, want not_resztvevo", e.Code)
	}
	rec = call(t, api, http.MethodPut, vallalas, `{"quantity":4}`, as("bela"))
	expectStatus(t, rec, http.StatusConflict)
	if e := decode[errorResp](t, rec); e.Code != "felszereles_too_much" {
		t.Fatalf("code = %q, want felszereles_too_much", e.Code)
	}

	f := decode[felszerelesResp](t, call(t, api, http.MethodGet, base, "", as("bela")))
	if len(f.Tetelek) != 2 || f.Hianyzik != 1 || !f.CanClaim || !f.CanAdd || len(f.Emlekezteto) != 1 || f.Emlekezteto[0] != "hálózsák" {
		t.Fatalf("felszereles = %+v", f)
	}
	s := f.Tetelek[0]
	if s.Name != "sátor" || s.Vallalt != 2 || s.Hianyzik != 1 || s.Sajat != 2 || !s.CanEdit ||
		len(s.Vallalasok) != 1 || s.Vallalasok[0].FelhasznaloID != "bela" || s.Vallalasok[0].Name != "Béla" || s.Vallalasok[0].Quantity != 2 {
		t.Fatalf("sator = %+v", s)
	}
	halozsak := f.Tetelek[1]
	rec = call(t, api, http.MethodPut, base+"/tetel/"+halozsak.ID+"/vallalas", `{"quantity":1}`, as("bela"))
	expectStatus(t, rec, http.StatusConflict)
	if e := decode[errorResp](t, rec); e.Code != "felszereles_mindenki_hozza" {
		t.Fatalf("code = %q, want felszereles_mindenki_hozza", e.Code)
	}

	// Editing and deleting.
	rec = call(t, api, http.MethodPut, base+"/tetel/"+sator.ID, `{"name":"sátor","quantity":1}`, as("bela"))
	expectStatus(t, rec, http.StatusConflict)
	if e := decode[errorResp](t, rec); e.Code != "felszereles_tetel_claimed" {
		t.Fatalf("code = %q, want felszereles_tetel_claimed", e.Code)
	}
	expectStatus(t, call(t, api, http.MethodPut, base+"/tetel/"+sator.ID, `{"name":"nagy sátor","quantity":2}`, as("bela")), http.StatusNoContent)
	rec = call(t, api, http.MethodDelete, base+"/tetel/"+halozsak.ID, "", as("bela"))
	expectStatus(t, rec, http.StatusForbidden)
	if e := decode[errorResp](t, rec); e.Code != "not_permitted" {
		t.Fatalf("code = %q, want not_permitted", e.Code)
	}
	expectStatus(t, call(t, api, http.MethodDelete, base+"/tetel/"+halozsak.ID, "", as("anna")), http.StatusNoContent)
	expectStatus(t, call(t, api, http.MethodDelete, base+"/tetel/nope", "", as("anna")), http.StatusNotFound)
	expectStatus(t, call(t, api, http.MethodPut, vallalas, `{"quantity":0}`, as("bela")), http.StatusNoContent)

	f = decode[felszerelesResp](t, call(t, api, http.MethodGet, base, "", as("anna")))
	if len(f.Tetelek) != 1 || f.Tetelek[0].Name != "nagy sátor" || f.Tetelek[0].Hianyzik != 2 || len(f.Tetelek[0].Vallalasok) != 0 {
		t.Fatalf("felszereles = %+v", f)
	}

	// Switched off, it is hidden.
	expectStatus(t, call(t, api, http.MethodPut, "/esemeny/"+esemenyID+"/kiegeszito/felszereles", `{"enabled":false}`, as("anna")), http.StatusNoContent)
	rec = call(t, api, http.MethodGet, base, "", as("anna"))
	expectStatus(t, rec, http.StatusConflict)
	if e := decode[errorResp](t, rec); e.Code != "kiegeszito_off" {
		t.Fatalf("code = %q, want kiegeszito_off", e.Code)
	}
}
