package httpapi_test

import (
	"net/http"
	"testing"
)

type koltsegekResp struct {
	Kiadasok []struct {
		ID          string `json:"id"`
		Description string `json:"description"`
		Amount      int    `json:"amount"`
		PayerID     string `json:"payerId"`
		PayerName   string `json:"payerName"`
		Allapot     string `json:"allapot"`
		Egyenlo     bool   `json:"egyenlo"`
		CreatedBy   string `json:"createdBy"`
		CanEdit     bool   `json:"canEdit"`
		Reszek      []struct {
			FelhasznaloID string `json:"felhasznaloId"`
			Name          string `json:"name"`
			Amount        int    `json:"amount"`
		} `json:"reszek"`
	} `json:"kiadasok"`
	Egyenlegek []struct {
		FelhasznaloID string `json:"felhasznaloId"`
		Name          string `json:"name"`
		Fizetett      int    `json:"fizetett"`
		Resz          int    `json:"resz"`
		Netto         int    `json:"netto"`
	} `json:"egyenlegek"`
	Tervezett int  `json:"tervezett"`
	CanAdd    bool `json:"canAdd"`
}

func TestKoltsegekOverHTTP(t *testing.T) {
	api, esemenyID, idopontok := lifecycleSetup(t)
	base := "/esemeny/" + esemenyID + "/koltsegek"
	toggle := "/esemeny/" + esemenyID + "/kiegeszito/koltsegek"
	expectStatus(t, call(t, api, http.MethodPut, toggle, `{"enabled":true}`, as("anna")), http.StatusNoContent)

	// No Kiadasok before Veglegesitve.
	rec := call(t, api, http.MethodPost, base+"/kiadas", `{"description":"Bor","amount":1000}`, as("anna"))
	expectStatus(t, rec, http.StatusConflict)
	if e := decode[errorResp](t, rec); e.Code != "koltsegek_closed" {
		t.Fatalf("code = %q, want koltsegek_closed", e.Code)
	}

	for _, who := range []string{"anna", "bela"} {
		expectStatus(t, call(t, api, http.MethodPut, "/esemeny/"+esemenyID+"/idopont/"+idopontok[0]+"/visszajelzes", `{"valasz":"igen"}`, as(who)), http.StatusNoContent)
	}
	expectStatus(t, call(t, api, http.MethodPost, "/esemeny/"+esemenyID+"/veglegesites", `{"idopontId":"`+idopontok[0]+`"}`, as("anna")), http.StatusNoContent)

	// Bela records a Kiadas Anna paid, split equally by default.
	rec = call(t, api, http.MethodPost, base+"/kiadas", `{"description":"Bor","amount":1001,"payerId":"anna"}`, as("bela"))
	expectStatus(t, rec, http.StatusCreated)
	bor := decode[struct {
		ID      string `json:"id"`
		Allapot string `json:"allapot"`
		Egyenlo bool   `json:"egyenlo"`
	}](t, rec)
	if bor.Allapot != "kifizetve" || !bor.Egyenlo {
		t.Fatalf("kiadas = %+v", bor)
	}
	// A planned one with custom amounts.
	expectStatus(t, call(t, api, http.MethodPost, base+"/kiadas",
		`{"description":"Szállás","amount":120000,"allapot":"tervezett","split":{"egyenlo":false,"reszek":[{"felhasznaloId":"bela","amount":80000},{"felhasznaloId":"anna","amount":40000}]}}`,
		as("bela")), http.StatusCreated)

	rec = call(t, api, http.MethodPost, base+"/kiadas",
		`{"description":"Kaja","amount":100,"split":{"egyenlo":false,"reszek":[{"felhasznaloId":"bela","amount":10}]}}`, as("bela"))
	expectStatus(t, rec, http.StatusConflict)
	if e := decode[errorResp](t, rec); e.Code != "koltsegek_split_sum" {
		t.Fatalf("code = %q, want koltsegek_split_sum", e.Code)
	}
	expectStatus(t, call(t, api, http.MethodPost, base+"/kiadas", `{"description":"","amount":100}`, as("bela")), http.StatusBadRequest)
	expectStatus(t, call(t, api, http.MethodPost, base+"/kiadas", `{"description":"x","amount":100}`, as("dani")), http.StatusNotFound)
	rec = call(t, api, http.MethodPost, base+"/kiadas", `{"description":"x","amount":100,"payerId":"dani"}`, as("bela"))
	expectStatus(t, rec, http.StatusForbidden)
	if e := decode[errorResp](t, rec); e.Code != "koltsegek_not_resztvevo" {
		t.Fatalf("code = %q, want koltsegek_not_resztvevo", e.Code)
	}

	k := decode[koltsegekResp](t, call(t, api, http.MethodGet, base, "", as("bela")))
	if len(k.Kiadasok) != 2 || k.Tervezett != 1 || !k.CanAdd {
		t.Fatalf("koltsegek = %+v", k)
	}
	b := k.Kiadasok[0]
	if b.Description != "Bor" || b.PayerID != "anna" || b.PayerName != "Anna" || b.CreatedBy != "bela" || !b.CanEdit ||
		len(b.Reszek) != 2 || b.Reszek[0].FelhasznaloID != "anna" || b.Reszek[0].Amount != 501 || b.Reszek[1].Name != "Béla" || b.Reszek[1].Amount != 500 {
		t.Fatalf("bor = %+v", b)
	}
	sz := k.Kiadasok[1]
	if sz.Allapot != "tervezett" || sz.Egyenlo || len(sz.Reszek) != 2 || sz.Reszek[0].Amount != 40000 || sz.Reszek[1].Amount != 80000 {
		t.Fatalf("szallas = %+v", sz)
	}
	if len(k.Egyenlegek) != 2 || k.Egyenlegek[0].Name != "Anna" || k.Egyenlegek[0].Fizetett != 1001 || k.Egyenlegek[0].Resz != 501 || k.Egyenlegek[0].Netto != 500 ||
		k.Egyenlegek[1].FelhasznaloID != "bela" || k.Egyenlegek[1].Netto != -500 {
		t.Fatalf("egyenlegek = %+v", k.Egyenlegek)
	}

	// Marking the cabin paid with the real amount: the custom split must
	// add up again.
	rec = call(t, api, http.MethodPut, base+"/kiadas/"+sz.ID, `{"description":"Szállás","amount":90000,"allapot":"kifizetve"}`, as("bela"))
	expectStatus(t, rec, http.StatusConflict)
	expectStatus(t, call(t, api, http.MethodPut, base+"/kiadas/"+sz.ID,
		`{"description":"Szállás","amount":90000,"allapot":"kifizetve","split":{"egyenlo":true,"reszek":[{"felhasznaloId":"anna"},{"felhasznaloId":"bela"}]}}`,
		as("bela")), http.StatusNoContent)
	k = decode[koltsegekResp](t, call(t, api, http.MethodGet, base, "", as("anna")))
	if k.Tervezett != 0 || k.Kiadasok[1].Allapot != "kifizetve" || !k.Kiadasok[1].Egyenlo || k.Kiadasok[1].Reszek[0].Amount != 45000 {
		t.Fatalf("koltsegek = %+v", k)
	}

	// Switching off is refused while it has Kiadasok.
	rec = call(t, api, http.MethodPut, toggle, `{"enabled":false}`, as("anna"))
	expectStatus(t, rec, http.StatusConflict)
	if e := decode[errorResp](t, rec); e.Code != "kiegeszito_in_use" {
		t.Fatalf("code = %q, want kiegeszito_in_use", e.Code)
	}

	// Deleting.
	expectStatus(t, call(t, api, http.MethodDelete, base+"/kiadas/nope", "", as("anna")), http.StatusNotFound)
	for _, id := range []string{bor.ID, sz.ID} {
		expectStatus(t, call(t, api, http.MethodDelete, base+"/kiadas/"+id, "", as("anna")), http.StatusNoContent)
	}
	expectStatus(t, call(t, api, http.MethodPut, toggle, `{"enabled":false}`, as("anna")), http.StatusNoContent)
	rec = call(t, api, http.MethodGet, base, "", as("anna"))
	expectStatus(t, rec, http.StatusConflict)
	if e := decode[errorResp](t, rec); e.Code != "kiegeszito_off" {
		t.Fatalf("code = %q, want kiegeszito_off", e.Code)
	}
}

func TestOnlyItsPayerCreatorOrTheSzervezoEditsAKiadasOverHTTP(t *testing.T) {
	api, esemenyID, idopontok := lifecycleSetup(t)
	base := "/esemeny/" + esemenyID + "/koltsegek"
	expectStatus(t, call(t, api, http.MethodPut, "/esemeny/"+esemenyID+"/kiegeszito/koltsegek", `{"enabled":true}`, as("anna")), http.StatusNoContent)
	for _, who := range []string{"anna", "bela"} {
		expectStatus(t, call(t, api, http.MethodPut, "/esemeny/"+esemenyID+"/idopont/"+idopontok[0]+"/visszajelzes", `{"valasz":"igen"}`, as(who)), http.StatusNoContent)
	}
	expectStatus(t, call(t, api, http.MethodPost, "/esemeny/"+esemenyID+"/veglegesites", `{"idopontId":"`+idopontok[0]+`"}`, as("anna")), http.StatusNoContent)

	rec := call(t, api, http.MethodPost, base+"/kiadas", `{"description":"Benzin","amount":5000}`, as("anna"))
	expectStatus(t, rec, http.StatusCreated)
	k := decode[struct {
		ID string `json:"id"`
	}](t, rec)
	rec = call(t, api, http.MethodPut, base+"/kiadas/"+k.ID, `{"description":"Benzin","amount":6000}`, as("bela"))
	expectStatus(t, rec, http.StatusForbidden)
	if e := decode[errorResp](t, rec); e.Code != "not_permitted" {
		t.Fatalf("code = %q, want not_permitted", e.Code)
	}
	expectStatus(t, call(t, api, http.MethodDelete, base+"/kiadas/"+k.ID, "", as("bela")), http.StatusForbidden)
	expectStatus(t, call(t, api, http.MethodPut, base+"/kiadas/"+k.ID, `nope`, as("anna")), http.StatusBadRequest)
	expectStatus(t, call(t, api, http.MethodPut, base+"/kiadas/"+k.ID, `{"description":"Benzin","amount":6000}`, as("anna")), http.StatusNoContent)
	if r := decode[koltsegekResp](t, call(t, api, http.MethodGet, base, "", as("bela"))); r.Kiadasok[0].Amount != 6000 || r.Kiadasok[0].CanEdit {
		t.Fatalf("koltsegek = %+v", r)
	}
}
