package httpapi_test

import (
	"net/http"
	"testing"
)

type vendegEsemenyResp struct {
	CsoportName    string `json:"csoportName"`
	ViewerIsVendeg bool   `json:"viewerIsVendeg"`
	Idopontok      []struct {
		ID             string `json:"id"`
		Igen           int    `json:"igen"`
		VendegIgen     int    `json:"vendegIgen"`
		Visszajelzesek []struct {
			FelhasznaloID string `json:"felhasznaloId"`
			VendegID      string `json:"vendegId"`
			Name          string `json:"name"`
			IsVendeg      bool   `json:"isVendeg"`
			VendegAddedBy string `json:"vendegAddedBy"`
		} `json:"visszajelzesek"`
	} `json:"idopontok"`
	Resztvevok []struct {
		FelhasznaloID string `json:"felhasznaloId"`
		VendegID      string `json:"vendegId"`
		IsVendeg      bool   `json:"isVendeg"`
		AddedBy       string `json:"addedBy"`
	} `json:"resztvevok"`
	Vendegek []struct {
		ID        string `json:"id"`
		Kind      string `json:"kind"`
		Name      string `json:"name"`
		AddedBy   string `json:"addedBy"`
		CanAnswer bool   `json:"canAnswer"`
		CanRemove bool   `json:"canRemove"`
	} `json:"vendegek"`
}

func TestAnEsemenyMeghivoOverHTTP(t *testing.T) {
	api, esemenyID, _ := lifecycleSetup(t)
	login(t, api, "vera", "555", "Vera")

	// Only Tags create them.
	expectStatus(t, call(t, api, http.MethodPost, "/esemeny/"+esemenyID+"/meghivo", "", as("vera")), http.StatusNotFound)
	rec := call(t, api, http.MethodPost, "/esemeny/"+esemenyID+"/meghivo", "", as("bela"))
	expectStatus(t, rec, http.StatusCreated)
	m := decode[meghivoResp](t, rec)
	list := decode[[]meghivoResp](t, call(t, api, http.MethodGet, "/esemeny/"+esemenyID+"/meghivo", "", as("anna")))
	if len(list) != 1 || list[0].ID != m.ID {
		t.Fatalf("meghivok = %+v", list)
	}

	// The preview needs no Felhasznalo.
	rec = call(t, api, http.MethodGet, "/esemeny-meghivo/"+m.Token, "", map[string]string{"X-Internal-Secret": secret})
	expectStatus(t, rec, http.StatusOK)
	if p := decode[struct {
		Title        string `json:"title"`
		SzervezoName string `json:"szervezoName"`
	}](t, rec); p.Title == "" || p.SzervezoName != "Anna" {
		t.Fatalf("preview = %+v", p)
	}
	expectStatus(t, call(t, api, http.MethodGet, "/esemeny-meghivo/"+m.Token+"/esemeny", "", as("vera")), http.StatusNotFound)

	rec = call(t, api, http.MethodPost, "/esemeny-meghivo/"+m.Token+"/join", "", as("vera"))
	expectStatus(t, rec, http.StatusOK)
	joined := decode[struct {
		EsemenyID string `json:"esemenyId"`
		Joined    bool   `json:"joined"`
	}](t, rec)
	if joined.EsemenyID != esemenyID || !joined.Joined {
		t.Fatalf("join = %+v", joined)
	}
	rec = call(t, api, http.MethodGet, "/esemeny-meghivo/"+m.Token+"/esemeny", "", as("vera"))
	expectStatus(t, rec, http.StatusOK)
	d := decode[vendegEsemenyResp](t, call(t, api, http.MethodGet, "/esemeny/"+esemenyID, "", as("vera")))
	if !d.ViewerIsVendeg || d.CsoportName != "" {
		t.Fatalf("Vera's view: %+v", d)
	}

	// Revoked: 410 with the reason.
	expectStatus(t, call(t, api, http.MethodDelete, "/esemeny/"+esemenyID+"/meghivo/"+m.ID, "", as("bela")), http.StatusNoContent)
	login(t, api, "zoli", "666", "Zoli")
	rec = call(t, api, http.MethodPost, "/esemeny-meghivo/"+m.Token+"/join", "", as("zoli"))
	expectStatus(t, rec, http.StatusGone)
	if c := errorCode(t, rec); c != "revoked" {
		t.Fatalf("code = %q, want revoked", c)
	}
	expectStatus(t, call(t, api, http.MethodGet, "/esemeny-meghivo/no-such-token", "", map[string]string{"X-Internal-Secret": secret}), http.StatusNotFound)

	// Expired once Elmaradt.
	m2 := decode[meghivoResp](t, call(t, api, http.MethodPost, "/esemeny/"+esemenyID+"/meghivo", "", as("anna")))
	expectStatus(t, call(t, api, http.MethodPost, "/esemeny/"+esemenyID+"/lemondas", "", as("anna")), http.StatusNoContent)
	rec = call(t, api, http.MethodGet, "/esemeny-meghivo/"+m2.Token, "", map[string]string{"X-Internal-Secret": secret})
	expectStatus(t, rec, http.StatusGone)
	if c := errorCode(t, rec); c != "expired" {
		t.Fatalf("code = %q, want expired", c)
	}
	rec = call(t, api, http.MethodPost, "/esemeny/"+esemenyID+"/meghivo", "", as("anna"))
	expectStatus(t, rec, http.StatusConflict)
	if c := errorCode(t, rec); c != "vendeg_closed" {
		t.Fatalf("code = %q, want vendeg_closed", c)
	}
}

func TestANameOnlyVendegOverHTTP(t *testing.T) {
	api, esemenyID, idopontok := lifecycleSetup(t)

	expectStatus(t, call(t, api, http.MethodPost, "/esemeny/"+esemenyID+"/vendeg", `{"name":""}`, as("bela")), http.StatusBadRequest)
	rec := call(t, api, http.MethodPost, "/esemeny/"+esemenyID+"/vendeg", `{"name":"Nagyi"}`, as("bela"))
	expectStatus(t, rec, http.StatusCreated)
	v := decode[struct {
		ID   string `json:"id"`
		Kind string `json:"kind"`
		Name string `json:"name"`
	}](t, rec)
	if v.Kind != "nev" || v.Name != "Nagyi" {
		t.Fatalf("vendeg = %+v", v)
	}

	path := "/esemeny/" + esemenyID + "/idopont/" + idopontok[0] + "/vendeg/" + v.ID + "/visszajelzes"
	expectStatus(t, call(t, api, http.MethodPut, path, `{"valasz":"igen"}`, as("anna")), http.StatusForbidden)
	expectStatus(t, call(t, api, http.MethodPut, path, `{"valasz":"igen"}`, as("bela")), http.StatusNoContent)
	expectStatus(t, call(t, api, http.MethodPut, "/esemeny/"+esemenyID+"/idopont/"+idopontok[0]+"/visszajelzes", `{"valasz":"igen"}`, as("bela")), http.StatusNoContent)
	expectStatus(t, call(t, api, http.MethodPost, "/esemeny/"+esemenyID+"/veglegesites", `{"idopontId":"`+idopontok[0]+`"}`, as("anna")), http.StatusNoContent)

	d := decode[vendegEsemenyResp](t, call(t, api, http.MethodGet, "/esemeny/"+esemenyID, "", as("bela")))
	first := d.Idopontok[0]
	if first.Igen != 1 || first.VendegIgen != 1 {
		t.Fatalf("counts = %d + %d vendeg, want 1 + 1", first.Igen, first.VendegIgen)
	}
	var nagyi bool
	for _, vj := range first.Visszajelzesek {
		nagyi = nagyi || (vj.VendegID == v.ID && vj.FelhasznaloID == "" && vj.Name == "Nagyi" && vj.IsVendeg && vj.VendegAddedBy == "bela")
	}
	if !nagyi {
		t.Fatalf("visszajelzesek = %+v, want Nagyi's", first.Visszajelzesek)
	}
	if len(d.Resztvevok) != 2 || d.Resztvevok[1].VendegID != v.ID || !d.Resztvevok[1].IsVendeg || d.Resztvevok[1].AddedBy != "bela" {
		t.Fatalf("resztvevok = %+v", d.Resztvevok)
	}
	if len(d.Vendegek) != 1 || !d.Vendegek[0].CanAnswer || !d.Vendegek[0].CanRemove || d.Vendegek[0].AddedBy != "bela" {
		t.Fatalf("vendegek = %+v", d.Vendegek)
	}

	expectStatus(t, call(t, api, http.MethodDelete, "/esemeny/"+esemenyID+"/vendeg/"+v.ID, "", as("dani")), http.StatusNotFound)
	expectStatus(t, call(t, api, http.MethodDelete, "/esemeny/"+esemenyID+"/vendeg/"+v.ID, "", as("anna")), http.StatusNoContent)
	expectStatus(t, call(t, api, http.MethodDelete, "/esemeny/"+esemenyID+"/vendeg/"+v.ID, "", as("anna")), http.StatusNotFound)
}
