package httpapi_test

import (
	"net/http"
	"testing"
)

type allapotResp struct {
	Allapot            string  `json:"allapot"`
	ConfirmedIdopontID *string `json:"confirmedIdopontId"`
	Idopontok          []struct {
		ID         string `json:"id"`
		Answerable bool   `json:"answerable"`
		Confirmed  bool   `json:"confirmed"`
	} `json:"idopontok"`
	Resztvevok []struct {
		FelhasznaloID string `json:"felhasznaloId"`
		Name          string `json:"name"`
		IsTag         bool   `json:"isTag"`
	} `json:"resztvevok"`
}

// lifecycleSetup has Anna propose an Esemeny in a Csoport that Bela has
// joined, and returns its id and its Idopontok' ids.
func lifecycleSetup(t *testing.T) (http.Handler, string, []string) {
	t.Helper()
	api := newAPI(t)
	login(t, api, "anna", "111", "Anna")
	login(t, api, "bela", "222", "Béla")
	login(t, api, "dani", "444", "Dani")
	c := decode[csoportResp](t, call(t, api, http.MethodPost, "/csoport", `{"name":"Péntek esték"}`, as("anna")))
	m := decode[meghivoResp](t, call(t, api, http.MethodPost, "/csoport/"+c.ID+"/meghivo", "", as("anna")))
	expectStatus(t, call(t, api, http.MethodPost, "/csoport-meghivo/"+m.Token+"/join", "", as("bela")), http.StatusOK)
	e := decode[esemenyResp](t, call(t, api, http.MethodPost, "/csoport/"+c.ID+"/esemeny", proposal, as("anna")))
	d := decode[allapotResp](t, call(t, api, http.MethodGet, "/esemeny/"+e.ID, "", as("anna")))
	return api, e.ID, []string{d.Idopontok[0].ID, d.Idopontok[1].ID}
}

func TestSzervezoConfirmsReopensAndCancelsOverHTTP(t *testing.T) {
	api, esemenyID, idopontok := lifecycleSetup(t)
	answer := func(who, idopontID, valasz string) {
		t.Helper()
		expectStatus(t, call(t, api, http.MethodPut, "/esemeny/"+esemenyID+"/idopont/"+idopontID+"/visszajelzes", `{"valasz":"`+valasz+`"}`, as(who)), http.StatusNoContent)
	}
	answer("bela", idopontok[1], "igen")

	expectStatus(t, call(t, api, http.MethodPost, "/esemeny/"+esemenyID+"/veglegesites", `{"idopontId":"`+idopontok[1]+`"}`, as("anna")), http.StatusNoContent)
	d := decode[allapotResp](t, call(t, api, http.MethodGet, "/esemeny/"+esemenyID, "", as("bela")))
	if d.Allapot != "veglegesitve" || d.ConfirmedIdopontID == nil || *d.ConfirmedIdopontID != idopontok[1] {
		t.Fatalf("after confirming: %+v", d)
	}
	if d.Idopontok[0].Answerable || d.Idopontok[0].Confirmed || !d.Idopontok[1].Answerable || !d.Idopontok[1].Confirmed {
		t.Fatalf("idopontok after confirming = %+v, want only the second answerable and confirmed", d.Idopontok)
	}
	if len(d.Resztvevok) != 1 || d.Resztvevok[0].FelhasznaloID != "bela" || d.Resztvevok[0].Name != "Béla" || !d.Resztvevok[0].IsTag {
		t.Fatalf("resztvevok = %+v, want Béla", d.Resztvevok)
	}
	// The other Idopont is closed now.
	expectStatus(t, call(t, api, http.MethodPut, "/esemeny/"+esemenyID+"/idopont/"+idopontok[0]+"/visszajelzes", `{"valasz":"igen"}`, as("bela")), http.StatusConflict)

	expectStatus(t, call(t, api, http.MethodPost, "/esemeny/"+esemenyID+"/ujranyitas", "", as("anna")), http.StatusNoContent)
	d = decode[allapotResp](t, call(t, api, http.MethodGet, "/esemeny/"+esemenyID, "", as("anna")))
	if d.Allapot != "egyeztetes" || d.ConfirmedIdopontID != nil || len(d.Resztvevok) != 0 || d.Resztvevok == nil {
		t.Fatalf("after reopening: %+v, want Egyeztetes with no confirmed Idopont and an empty list", d)
	}

	expectStatus(t, call(t, api, http.MethodPost, "/esemeny/"+esemenyID+"/lemondas", "", as("anna")), http.StatusNoContent)
	d = decode[allapotResp](t, call(t, api, http.MethodGet, "/esemeny/"+esemenyID, "", as("anna")))
	if d.Allapot != "elmaradt" || d.Idopontok[0].Answerable || d.Idopontok[1].Answerable {
		t.Fatalf("after cancelling: %+v", d)
	}
}

func TestLifecycleRoutesMapTheirErrors(t *testing.T) {
	api, esemenyID, idopontok := lifecycleSetup(t)
	confirm := `{"idopontId":"` + idopontok[0] + `"}`

	for _, tc := range []struct {
		name, who, path, body string
		want                  int
	}{
		{"a Tag who isn't the Szervezo confirms", "bela", "/veglegesites", confirm, http.StatusForbidden},
		{"a Tag who isn't the Szervezo reopens", "bela", "/ujranyitas", "", http.StatusForbidden},
		{"a Tag who isn't the Szervezo cancels", "bela", "/lemondas", "", http.StatusForbidden},
		{"a non-Tag confirms", "dani", "/veglegesites", confirm, http.StatusNotFound},
		{"a non-Tag reopens", "dani", "/ujranyitas", "", http.StatusNotFound},
		{"a non-Tag cancels", "dani", "/lemondas", "", http.StatusNotFound},
		{"an unknown Idopont", "anna", "/veglegesites", `{"idopontId":"nope"}`, http.StatusNotFound},
		{"not JSON", "anna", "/veglegesites", `nope`, http.StatusBadRequest},
		{"reopening in Egyeztetes", "anna", "/ujranyitas", "", http.StatusConflict},
	} {
		if rec := call(t, api, http.MethodPost, "/esemeny/"+esemenyID+tc.path, tc.body, as(tc.who)); rec.Code != tc.want {
			t.Errorf("%s: status = %d, want %d (body %s)", tc.name, rec.Code, tc.want, rec.Body)
		}
	}

	expectStatus(t, call(t, api, http.MethodPost, "/esemeny/"+esemenyID+"/lemondas", "", as("anna")), http.StatusNoContent)
	expectStatus(t, call(t, api, http.MethodPost, "/esemeny/"+esemenyID+"/veglegesites", confirm, as("anna")), http.StatusConflict)
	expectStatus(t, call(t, api, http.MethodPost, "/esemeny/"+esemenyID+"/lemondas", "", as("anna")), http.StatusConflict)
	expectStatus(t, call(t, api, http.MethodPost, "/esemeny/no-such-esemeny/lemondas", "", as("anna")), http.StatusNotFound)
}
