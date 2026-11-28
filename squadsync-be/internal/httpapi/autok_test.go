package httpapi_test

import (
	"net/http"
	"testing"
)

type autokResp struct {
	Autok []struct {
		ID        string `json:"id"`
		SoforID   string `json:"soforId"`
		SoforName string `json:"soforName"`
		Seats     int    `json:"seats"`
		Note      string `json:"note"`
		Utasok    []struct {
			FelhasznaloID string `json:"felhasznaloId"`
			Name          string `json:"name"`
		} `json:"utasok"`
		Szabad    int  `json:"szabad"`
		CanEdit   bool `json:"canEdit"`
		CanDelete bool `json:"canDelete"`
	} `json:"autok"`
	SajatUton []struct {
		FelhasznaloID string `json:"felhasznaloId"`
	} `json:"sajatUton"`
	HelyNelkul []struct {
		FelhasznaloID string `json:"felhasznaloId"`
		Name          string `json:"name"`
	} `json:"helyNelkul"`
	CanClaim        bool   `json:"canClaim"`
	SajatAutoID     string `json:"sajatAutoId"`
	SajatUlesAutoID string `json:"sajatUlesAutoId"`
	SajatUtonVan    bool   `json:"sajatUtonVan"`
	IsSzervezo      bool   `json:"isSzervezo"`
}

func TestAutokOverHTTP(t *testing.T) {
	api, esemenyID, idopontok := lifecycleSetup(t)
	base := "/esemeny/" + esemenyID + "/autok"
	expectStatus(t, call(t, api, http.MethodGet, base, "", as("anna")), http.StatusConflict) // off
	expectStatus(t, call(t, api, http.MethodPut, "/esemeny/"+esemenyID+"/kiegeszito/autok", `{"enabled":true}`, as("anna")), http.StatusNoContent)

	// Nothing to claim before Veglegesitve.
	rec := call(t, api, http.MethodPost, base+"/auto", `{"seats":3}`, as("bela"))
	expectStatus(t, rec, http.StatusConflict)
	if e := decode[errorResp](t, rec); e.Code != "claiming_closed" {
		t.Fatalf("code = %q, want claiming_closed", e.Code)
	}
	for _, who := range []string{"anna", "bela"} {
		expectStatus(t, call(t, api, http.MethodPut, "/esemeny/"+esemenyID+"/idopont/"+idopontok[0]+"/visszajelzes", `{"valasz":"igen"}`, as(who)), http.StatusNoContent)
	}
	expectStatus(t, call(t, api, http.MethodPost, "/esemeny/"+esemenyID+"/veglegesites", `{"idopontId":"`+idopontok[0]+`"}`, as("anna")), http.StatusNoContent)

	// Offering.
	expectStatus(t, call(t, api, http.MethodPost, base+"/auto", `{"seats":0}`, as("bela")), http.StatusBadRequest)
	expectStatus(t, call(t, api, http.MethodPost, base+"/auto", `{"seats":2}`, as("dani")), http.StatusNotFound)
	rec = call(t, api, http.MethodPost, base+"/auto", `{"seats":1,"note":"indulás 8:00, Keleti"}`, as("bela"))
	expectStatus(t, rec, http.StatusCreated)
	auto := decode[struct {
		ID    string `json:"id"`
		Seats int    `json:"seats"`
	}](t, rec)
	rec = call(t, api, http.MethodPost, base+"/auto", `{"seats":2}`, as("bela"))
	expectStatus(t, rec, http.StatusConflict)
	if e := decode[errorResp](t, rec); e.Code != "autok_already_sofor" {
		t.Fatalf("code = %q, want autok_already_sofor", e.Code)
	}
	rec = call(t, api, http.MethodPost, base+"/auto/"+auto.ID+"/ules", `{}`, as("bela"))
	expectStatus(t, rec, http.StatusConflict)
	if e := decode[errorResp](t, rec); e.Code != "autok_sofor_cant_ride" {
		t.Fatalf("code = %q, want autok_sofor_cant_ride", e.Code)
	}

	// Seats.
	expectStatus(t, call(t, api, http.MethodPost, base+"/auto/nope/ules", `{}`, as("anna")), http.StatusNotFound)
	expectStatus(t, call(t, api, http.MethodPost, base+"/auto/"+auto.ID+"/ules", `{"vendegId":"v1"}`, as("anna")), http.StatusNotFound)
	expectStatus(t, call(t, api, http.MethodPost, base+"/auto/"+auto.ID+"/ules", `{}`, as("anna")), http.StatusNoContent)
	a := decode[autokResp](t, call(t, api, http.MethodGet, base, "", as("anna")))
	if len(a.Autok) != 1 || a.Autok[0].SoforName != "Béla" || a.Autok[0].Note != "indulás 8:00, Keleti" || a.Autok[0].Szabad != 0 ||
		len(a.Autok[0].Utasok) != 1 || a.Autok[0].Utasok[0].FelhasznaloID != "anna" || a.Autok[0].Utasok[0].Name != "Anna" ||
		a.Autok[0].CanEdit || !a.Autok[0].CanDelete || a.SajatUlesAutoID != auto.ID || !a.IsSzervezo || len(a.HelyNelkul) != 0 {
		t.Fatalf("autok = %+v", a)
	}

	// Moving people is the Szervezo's.
	rec = call(t, api, http.MethodPut, base+"/auto/"+auto.ID+"/utas", `{"felhasznaloId":"anna"}`, as("bela"))
	expectStatus(t, rec, http.StatusForbidden)
	if e := decode[errorResp](t, rec); e.Code != "not_permitted" {
		t.Fatalf("code = %q, want not_permitted", e.Code)
	}
	expectStatus(t, call(t, api, http.MethodDelete, base+"/ules?felhasznaloId=anna", "", as("bela")), http.StatusForbidden)
	expectStatus(t, call(t, api, http.MethodDelete, base+"/ules?felhasznaloId=anna", "", as("anna")), http.StatusNoContent)
	expectStatus(t, call(t, api, http.MethodPut, base+"/auto/"+auto.ID+"/utas", `{"felhasznaloId":"anna"}`, as("anna")), http.StatusNoContent)
	rec = call(t, api, http.MethodPut, base+"/auto/"+auto.ID+"/utas", `{"felhasznaloId":"anna"}`, as("anna")) // already there
	expectStatus(t, rec, http.StatusNoContent)

	// Saját úton takes the seat away.
	expectStatus(t, call(t, api, http.MethodPut, base+"/sajat-uton", `{"on":true}`, as("anna")), http.StatusNoContent)
	a = decode[autokResp](t, call(t, api, http.MethodGet, base, "", as("anna")))
	if !a.SajatUtonVan || len(a.SajatUton) != 1 || a.Autok[0].Szabad != 1 || len(a.HelyNelkul) != 0 {
		t.Fatalf("after saját úton: %+v", a)
	}
	expectStatus(t, call(t, api, http.MethodPut, base+"/sajat-uton", `{"on":true}`, as("bela")), http.StatusConflict)
	expectStatus(t, call(t, api, http.MethodPut, base+"/sajat-uton", `{"on":false}`, as("anna")), http.StatusNoContent)
	a = decode[autokResp](t, call(t, api, http.MethodGet, base, "", as("anna")))
	if len(a.HelyNelkul) != 1 || a.HelyNelkul[0].FelhasznaloID != "anna" {
		t.Fatalf("without a seat = %+v, want Anna", a.HelyNelkul)
	}

	// Overbooking, and who is in the Auto.
	expectStatus(t, call(t, api, http.MethodPost, base+"/auto/"+auto.ID+"/ules", `{}`, as("anna")), http.StatusNoContent)
	expectStatus(t, call(t, api, http.MethodPut, base+"/auto/"+auto.ID, `{"seats":1}`, as("bela")), http.StatusNoContent)
	rec = call(t, api, http.MethodPut, base+"/auto/"+auto.ID, `{"seats":8}`, as("anna"))
	expectStatus(t, rec, http.StatusForbidden)
	u := decode[struct {
		Auto struct {
			SoforID string `json:"soforId"`
		} `json:"auto"`
		Utasok []struct {
			FelhasznaloID string `json:"felhasznaloId"`
		} `json:"utasok"`
	}](t, call(t, api, http.MethodGet, base+"/auto/"+auto.ID+"/utasai", "", as("bela")))
	if u.Auto.SoforID != "bela" || len(u.Utasok) != 1 || u.Utasok[0].FelhasznaloID != "anna" {
		t.Fatalf("utasai = %+v", u)
	}
	expectStatus(t, call(t, api, http.MethodGet, base+"/auto/"+auto.ID+"/utasai", "", as("dani")), http.StatusNotFound)

	// Deleting.
	expectStatus(t, call(t, api, http.MethodDelete, base+"/auto/"+auto.ID, "", as("dani")), http.StatusNotFound)
	expectStatus(t, call(t, api, http.MethodDelete, base+"/auto/"+auto.ID, "", as("bela")), http.StatusNoContent)
	expectStatus(t, call(t, api, http.MethodDelete, base+"/auto/"+auto.ID, "", as("bela")), http.StatusNotFound)
	a = decode[autokResp](t, call(t, api, http.MethodGet, base, "", as("anna")))
	if len(a.Autok) != 0 || a.Autok == nil || len(a.HelyNelkul) != 2 {
		t.Fatalf("after deleting: %+v", a)
	}
}

func TestUzemanyagOverHTTP(t *testing.T) {
	api, esemenyID, idopontok := lifecycleSetup(t)
	base := "/esemeny/" + esemenyID + "/autok"
	expectStatus(t, call(t, api, http.MethodPut, "/esemeny/"+esemenyID+"/kiegeszito/autok", `{"enabled":true}`, as("anna")), http.StatusNoContent)
	for _, who := range []string{"anna", "bela"} {
		expectStatus(t, call(t, api, http.MethodPut, "/esemeny/"+esemenyID+"/idopont/"+idopontok[0]+"/visszajelzes", `{"valasz":"igen"}`, as(who)), http.StatusNoContent)
	}
	expectStatus(t, call(t, api, http.MethodPost, "/esemeny/"+esemenyID+"/veglegesites", `{"idopontId":"`+idopontok[0]+`"}`, as("anna")), http.StatusNoContent)
	auto := decode[struct {
		ID string `json:"id"`
	}](t, call(t, api, http.MethodPost, base+"/auto", `{"seats":2}`, as("bela")))
	expectStatus(t, call(t, api, http.MethodPost, base+"/auto/"+auto.ID+"/ules", `{}`, as("anna")), http.StatusNoContent)

	// Koltsegek is off.
	rec := call(t, api, http.MethodPost, base+"/auto/"+auto.ID+"/uzemanyag", `{"amount":1001}`, as("anna"))
	expectStatus(t, rec, http.StatusConflict)
	if e := decode[errorResp](t, rec); e.Code != "autok_koltsegek_off" {
		t.Fatalf("code = %q, want autok_koltsegek_off", e.Code)
	}
	expectStatus(t, call(t, api, http.MethodPut, "/esemeny/"+esemenyID+"/kiegeszito/koltsegek", `{"enabled":true}`, as("anna")), http.StatusNoContent)

	expectStatus(t, call(t, api, http.MethodPost, base+"/auto/"+auto.ID+"/uzemanyag", `{"amount":0}`, as("anna")), http.StatusBadRequest)
	expectStatus(t, call(t, api, http.MethodPost, base+"/auto/nope/uzemanyag", `{"amount":10}`, as("anna")), http.StatusNotFound)
	expectStatus(t, call(t, api, http.MethodPost, base+"/auto/"+auto.ID+"/uzemanyag", `{"amount":10}`, as("dani")), http.StatusNotFound)
	rec = call(t, api, http.MethodPost, base+"/auto/"+auto.ID+"/uzemanyag", `{"amount":10,"payerId":"dani"}`, as("anna"))
	expectStatus(t, rec, http.StatusForbidden)
	if e := decode[errorResp](t, rec); e.Code != "koltsegek_not_resztvevo" {
		t.Fatalf("code = %q, want koltsegek_not_resztvevo", e.Code)
	}

	rec = call(t, api, http.MethodPost, base+"/auto/"+auto.ID+"/uzemanyag", `{"amount":1001,"payerId":"bela"}`, as("anna"))
	expectStatus(t, rec, http.StatusCreated)
	k := decode[koltsegekResp](t, call(t, api, http.MethodGet, "/esemeny/"+esemenyID+"/koltsegek", "", as("anna")))
	if len(k.Kiadasok) != 1 || k.Kiadasok[0].Description != "Üzemanyag – Béla autója" || k.Kiadasok[0].PayerID != "bela" ||
		k.Kiadasok[0].Allapot != "kifizetve" || len(k.Kiadasok[0].Reszek) != 2 {
		t.Fatalf("kiadasok = %+v", k.Kiadasok)
	}
}
