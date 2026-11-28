package httpapi_test

import (
	"net/http"
	"testing"
)

type szallasResp struct {
	Note   string `json:"note"`
	Szobak []struct {
		ID    string `json:"id"`
		Name  string `json:"name"`
		Agyak []struct {
			ID       string `json:"id"`
			SzobaID  string `json:"szobaId"`
			Capacity int    `json:"capacity"`
			Label    string `json:"label"`
			Alvok    []struct {
				FelhasznaloID string `json:"felhasznaloId"`
				Name          string `json:"name"`
			} `json:"alvok"`
			Szabad int `json:"szabad"`
		} `json:"agyak"`
	} `json:"szobak"`
	MasholAlszik []struct {
		FelhasznaloID string `json:"felhasznaloId"`
	} `json:"masholAlszik"`
	AgyNelkul []struct {
		FelhasznaloID string `json:"felhasznaloId"`
		Name          string `json:"name"`
	} `json:"agyNelkul"`
	CanSetup          bool   `json:"canSetup"`
	CanClaim          bool   `json:"canClaim"`
	SajatAgyID        string `json:"sajatAgyId"`
	SajatMasholAlszik bool   `json:"sajatMasholAlszik"`
	IsSzervezo        bool   `json:"isSzervezo"`
}

func TestSzallasOverHTTP(t *testing.T) {
	api, esemenyID, idopontok := lifecycleSetup(t)
	base := "/esemeny/" + esemenyID + "/szallas"
	expectStatus(t, call(t, api, http.MethodGet, base, "", as("anna")), http.StatusConflict) // off
	expectStatus(t, call(t, api, http.MethodPut, "/esemeny/"+esemenyID+"/kiegeszito/szallas", `{"enabled":true}`, as("anna")), http.StatusNoContent)

	// Setup is the Szervezo's, and works before Veglegesitve.
	rec := call(t, api, http.MethodPost, base+"/szoba", `{"name":"Emeleti"}`, as("bela"))
	expectStatus(t, rec, http.StatusForbidden)
	if e := decode[errorResp](t, rec); e.Code != "not_permitted" {
		t.Fatalf("code = %q, want not_permitted", e.Code)
	}
	expectStatus(t, call(t, api, http.MethodPost, base+"/szoba", `{"name":"Emeleti"}`, as("dani")), http.StatusNotFound)
	expectStatus(t, call(t, api, http.MethodPost, base+"/szoba", `{"name":" "}`, as("anna")), http.StatusBadRequest)
	rec = call(t, api, http.MethodPost, base+"/szoba", `{"name":"Emeleti"}`, as("anna"))
	expectStatus(t, rec, http.StatusCreated)
	szoba := decode[struct {
		ID   string `json:"id"`
		Name string `json:"name"`
	}](t, rec)
	expectStatus(t, call(t, api, http.MethodPut, base, `{"note":"Napfény Vendégház"}`, as("anna")), http.StatusNoContent)
	expectStatus(t, call(t, api, http.MethodPut, base, `{"note":"x"}`, as("bela")), http.StatusForbidden)
	expectStatus(t, call(t, api, http.MethodPut, base+"/szoba/"+szoba.ID, `{"name":"Alsó"}`, as("anna")), http.StatusNoContent)
	expectStatus(t, call(t, api, http.MethodPut, base+"/szoba/nope", `{"name":"Alsó"}`, as("anna")), http.StatusNotFound)
	expectStatus(t, call(t, api, http.MethodPost, base+"/szoba/"+szoba.ID+"/agy", `{"capacity":3}`, as("anna")), http.StatusBadRequest)
	rec = call(t, api, http.MethodPost, base+"/szoba/"+szoba.ID+"/agy", `{"capacity":1,"label":"ablaknál"}`, as("anna"))
	expectStatus(t, rec, http.StatusCreated)
	single := decode[struct {
		ID string `json:"id"`
	}](t, rec)
	rec = call(t, api, http.MethodPost, base+"/szoba/"+szoba.ID+"/agy", `{"capacity":2}`, as("anna"))
	expectStatus(t, rec, http.StatusCreated)
	double := decode[struct {
		ID string `json:"id"`
	}](t, rec)

	// Nothing to claim before Veglegesitve.
	rec = call(t, api, http.MethodPost, base+"/agy/"+single.ID+"/foglalas", `{}`, as("bela"))
	expectStatus(t, rec, http.StatusConflict)
	if e := decode[errorResp](t, rec); e.Code != "claiming_closed" {
		t.Fatalf("code = %q, want claiming_closed", e.Code)
	}
	for _, who := range []string{"anna", "bela"} {
		expectStatus(t, call(t, api, http.MethodPut, "/esemeny/"+esemenyID+"/idopont/"+idopontok[0]+"/visszajelzes", `{"valasz":"igen"}`, as(who)), http.StatusNoContent)
	}
	expectStatus(t, call(t, api, http.MethodPost, "/esemeny/"+esemenyID+"/veglegesites", `{"idopontId":"`+idopontok[0]+`"}`, as("anna")), http.StatusNoContent)

	// Claiming.
	expectStatus(t, call(t, api, http.MethodPost, base+"/agy/nope/foglalas", `{}`, as("anna")), http.StatusNotFound)
	expectStatus(t, call(t, api, http.MethodPost, base+"/agy/"+single.ID+"/foglalas", `{"vendegId":"v1"}`, as("anna")), http.StatusNotFound)
	expectStatus(t, call(t, api, http.MethodPost, base+"/agy/"+single.ID+"/foglalas", `{"felhasznaloId":"bela"}`, as("anna")), http.StatusForbidden)
	expectStatus(t, call(t, api, http.MethodPost, base+"/agy/"+single.ID+"/foglalas", `{}`, as("anna")), http.StatusNoContent)
	rec = call(t, api, http.MethodPost, base+"/agy/"+single.ID+"/foglalas", `{}`, as("bela"))
	expectStatus(t, rec, http.StatusConflict)
	if e := decode[errorResp](t, rec); e.Code != "szallas_agy_full" {
		t.Fatalf("code = %q, want szallas_agy_full", e.Code)
	}
	rec = call(t, api, http.MethodPost, base+"/agy/"+double.ID+"/foglalas", `{}`, as("anna"))
	expectStatus(t, rec, http.StatusConflict)
	if e := decode[errorResp](t, rec); e.Code != "szallas_already_in_agy" {
		t.Fatalf("code = %q, want szallas_already_in_agy", e.Code)
	}
	expectStatus(t, call(t, api, http.MethodPost, base+"/agy/"+double.ID+"/foglalas", `{}`, as("bela")), http.StatusNoContent)
	z := decode[szallasResp](t, call(t, api, http.MethodGet, base, "", as("anna")))
	if z.Note != "Napfény Vendégház" || len(z.Szobak) != 1 || z.Szobak[0].Name != "Alsó" || len(z.Szobak[0].Agyak) != 2 ||
		z.Szobak[0].Agyak[0].Label != "ablaknál" || z.Szobak[0].Agyak[0].Szabad != 0 || z.Szobak[0].Agyak[0].Alvok[0].Name != "Anna" ||
		z.Szobak[0].Agyak[1].Capacity != 2 || z.Szobak[0].Agyak[1].Szabad != 1 || z.Szobak[0].Agyak[1].Alvok[0].FelhasznaloID != "bela" ||
		z.SajatAgyID != single.ID || !z.IsSzervezo || !z.CanSetup || !z.CanClaim || len(z.AgyNelkul) != 0 {
		t.Fatalf("szallas = %+v", z)
	}

	// Moving people is the Szervezo's.
	rec = call(t, api, http.MethodPut, base+"/agy/"+double.ID+"/alvo", `{"felhasznaloId":"anna"}`, as("bela"))
	expectStatus(t, rec, http.StatusForbidden)
	if e := decode[errorResp](t, rec); e.Code != "not_permitted" {
		t.Fatalf("code = %q, want not_permitted", e.Code)
	}
	expectStatus(t, call(t, api, http.MethodPut, base+"/agy/"+double.ID+"/alvo", `{"felhasznaloId":"anna"}`, as("anna")), http.StatusNoContent)
	expectStatus(t, call(t, api, http.MethodPut, base+"/agy/"+single.ID+"/alvo", `{"felhasznaloId":"anna"}`, as("anna")), http.StatusNoContent)
	expectStatus(t, call(t, api, http.MethodDelete, base+"/foglalas?felhasznaloId=anna", "", as("bela")), http.StatusForbidden)
	expectStatus(t, call(t, api, http.MethodDelete, base+"/foglalas?felhasznaloId=anna", "", as("anna")), http.StatusNoContent)
	z = decode[szallasResp](t, call(t, api, http.MethodGet, base, "", as("anna")))
	if len(z.AgyNelkul) != 1 || z.AgyNelkul[0].FelhasznaloID != "anna" {
		t.Fatalf("without a bed = %+v, want Anna", z.AgyNelkul)
	}

	// Máshol alszik.
	expectStatus(t, call(t, api, http.MethodPut, base+"/mashol-alszik", `{"on":true}`, as("bela")), http.StatusNoContent)
	expectStatus(t, call(t, api, http.MethodPut, base+"/mashol-alszik", `{"on":true}`, as("anna")), http.StatusNoContent)
	z = decode[szallasResp](t, call(t, api, http.MethodGet, base, "", as("bela")))
	if !z.SajatMasholAlszik || len(z.MasholAlszik) != 2 || z.SajatAgyID != "" || len(z.AgyNelkul) != 0 || z.Szobak[0].Agyak[1].Szabad != 2 {
		t.Fatalf("after máshol alszik: %+v", z)
	}
	expectStatus(t, call(t, api, http.MethodPut, base+"/mashol-alszik", `{"on":false}`, as("anna")), http.StatusNoContent)

	// The femohely can't go below the sleepers; deleting frees them.
	expectStatus(t, call(t, api, http.MethodPost, base+"/agy/"+double.ID+"/foglalas", `{}`, as("anna")), http.StatusNoContent)
	expectStatus(t, call(t, api, http.MethodPost, base+"/agy/"+double.ID+"/foglalas", `{}`, as("bela")), http.StatusNoContent)
	rec = call(t, api, http.MethodPut, base+"/agy/"+double.ID, `{"capacity":1}`, as("anna"))
	expectStatus(t, rec, http.StatusConflict)
	if e := decode[errorResp](t, rec); e.Code != "szallas_capacity_below_alvok" {
		t.Fatalf("code = %q, want szallas_capacity_below_alvok", e.Code)
	}
	expectStatus(t, call(t, api, http.MethodPut, base+"/agy/"+single.ID, `{"capacity":2,"label":"tágas"}`, as("anna")), http.StatusNoContent)
	expectStatus(t, call(t, api, http.MethodDelete, base+"/agy/"+double.ID, "", as("bela")), http.StatusForbidden)
	expectStatus(t, call(t, api, http.MethodDelete, base+"/agy/"+double.ID, "", as("anna")), http.StatusNoContent)
	expectStatus(t, call(t, api, http.MethodDelete, base+"/agy/"+double.ID, "", as("anna")), http.StatusNotFound)

	// Deleting a Szoba.
	expectStatus(t, call(t, api, http.MethodDelete, base+"/szoba/"+szoba.ID, "", as("bela")), http.StatusForbidden)
	expectStatus(t, call(t, api, http.MethodDelete, base+"/szoba/"+szoba.ID, "", as("anna")), http.StatusNoContent)
	expectStatus(t, call(t, api, http.MethodDelete, base+"/szoba/"+szoba.ID, "", as("anna")), http.StatusNotFound)
	z = decode[szallasResp](t, call(t, api, http.MethodGet, base, "", as("anna")))
	if len(z.Szobak) != 0 || z.Szobak == nil || len(z.AgyNelkul) != 2 {
		t.Fatalf("after deleting: %+v", z)
	}
}
