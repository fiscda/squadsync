package httpapi_test

import (
	"net/http"
	"slices"
	"strings"
	"testing"
	"time"
)

func TestAdminsSetTheEmlekeztetokOverHTTP(t *testing.T) {
	api, esemenyID, _ := lifecycleSetup(t)
	e := decode[struct {
		CsoportID string `json:"csoportId"`
	}](t, call(t, api, http.MethodGet, "/esemeny/"+esemenyID, "", as("anna")))
	path := "/csoport/" + e.CsoportID
	type resp struct {
		EmlekeztetoPercek []int `json:"emlekeztetoPercek"`
	}

	if got := decode[resp](t, call(t, api, http.MethodGet, path, "", as("bela"))); !slices.Equal(got.EmlekeztetoPercek, []int{1440, 60}) {
		t.Fatalf("default = %v, want [1440 60]", got.EmlekeztetoPercek)
	}
	expectStatus(t, call(t, api, http.MethodPut, path+"/emlekezteto", `{"percek":[180]}`, as("bela")), http.StatusForbidden)
	expectStatus(t, call(t, api, http.MethodPut, path+"/emlekezteto", `{"percek":[120]}`, as("anna")), http.StatusBadRequest)
	expectStatus(t, call(t, api, http.MethodPut, path+"/emlekezteto", `{"percek":[180]}`, as("dani")), http.StatusNotFound)
	expectStatus(t, call(t, api, http.MethodPut, path+"/emlekezteto", `{"percek":[15,10080]}`, as("anna")), http.StatusNoContent)
	if got := decode[resp](t, call(t, api, http.MethodGet, path, "", as("bela"))); !slices.Equal(got.EmlekeztetoPercek, []int{10080, 15}) {
		t.Fatalf("after setting = %v, want [10080 15]", got.EmlekeztetoPercek)
	}
	expectStatus(t, call(t, api, http.MethodPut, path+"/emlekezteto", `{"percek":[]}`, as("anna")), http.StatusNoContent)
	if got := decode[resp](t, call(t, api, http.MethodGet, path, "", as("bela"))); got.EmlekeztetoPercek == nil || len(got.EmlekeztetoPercek) != 0 {
		t.Fatalf("after clearing = %v, want []", got.EmlekeztetoPercek)
	}
}

func TestTheSzervezoSetsTheHataridoOverHTTP(t *testing.T) {
	api, esemenyID, idopontok := lifecycleSetup(t)
	path := "/esemeny/" + esemenyID
	type resp struct {
		Hatarido *time.Time `json:"hatarido"`
	}
	if got := decode[resp](t, call(t, api, http.MethodGet, path, "", as("bela"))); got.Hatarido != nil {
		t.Fatalf("hatarido = %v, want null", got.Hatarido)
	}

	expectStatus(t, call(t, api, http.MethodPut, path+"/hatarido", `{"hatarido":"2026-03-05T20:00"}`, as("bela")), http.StatusForbidden)
	expectStatus(t, call(t, api, http.MethodPut, path+"/hatarido", `{"hatarido":"2026-03-05T20:00"}`, as("dani")), http.StatusNotFound)
	expectStatus(t, call(t, api, http.MethodPut, path+"/hatarido", `{"hatarido":"tegnap"}`, as("anna")), http.StatusBadRequest)
	rec := call(t, api, http.MethodPut, path+"/hatarido", `{"hatarido":"2026-03-01T20:00"}`, as("anna"))
	expectStatus(t, rec, http.StatusBadRequest)
	if !strings.Contains(rec.Body.String(), `"code":"hatarido_not_future"`) {
		t.Fatalf("body = %s, want the hatarido_not_future code", rec.Body)
	}

	expectStatus(t, call(t, api, http.MethodPut, path+"/hatarido", `{"hatarido":"2026-03-05T20:00"}`, as("anna")), http.StatusNoContent)
	want := time.Date(2026, time.March, 5, 19, 0, 0, 0, time.UTC)
	if got := decode[resp](t, call(t, api, http.MethodGet, path, "", as("bela"))); got.Hatarido == nil || !got.Hatarido.Equal(want) {
		t.Fatalf("hatarido = %v, want %v", got.Hatarido, want)
	}
	expectStatus(t, call(t, api, http.MethodPut, path+"/hatarido", `{"hatarido":""}`, as("anna")), http.StatusNoContent)
	if got := decode[resp](t, call(t, api, http.MethodGet, path, "", as("bela"))); got.Hatarido != nil {
		t.Fatalf("hatarido after clearing = %v, want null", got.Hatarido)
	}

	expectStatus(t, call(t, api, http.MethodPost, path+"/veglegesites", `{"idopontId":"`+idopontok[0]+`"}`, as("anna")), http.StatusNoContent)
	expectStatus(t, call(t, api, http.MethodPut, path+"/hatarido", `{"hatarido":"2026-03-05T20:00"}`, as("anna")), http.StatusConflict)
}

func TestAProposalCarriesItsHataridoOverHTTP(t *testing.T) {
	api := newAPI(t)
	login(t, api, "anna", "111", "Anna")
	c := decode[csoportResp](t, call(t, api, http.MethodPost, "/csoport", `{"name":"Péntek esték"}`, as("anna")))
	withHatarido := strings.Replace(proposal, `"idopontok"`, `"hatarido": "2026-03-05T20:00", "idopontok"`, 1)
	e := decode[esemenyResp](t, call(t, api, http.MethodPost, "/csoport/"+c.ID+"/esemeny", withHatarido, as("anna")))
	got := decode[struct {
		Hatarido *time.Time `json:"hatarido"`
	}](t, call(t, api, http.MethodGet, "/esemeny/"+e.ID, "", as("anna")))
	if got.Hatarido == nil || !got.Hatarido.Equal(time.Date(2026, time.March, 5, 19, 0, 0, 0, time.UTC)) {
		t.Fatalf("hatarido = %v", got.Hatarido)
	}

	past := strings.Replace(proposal, `"idopontok"`, `"hatarido": "2026-03-01T20:00", "idopontok"`, 1)
	rec := call(t, api, http.MethodPost, "/csoport/"+c.ID+"/esemeny", past, as("anna"))
	expectStatus(t, rec, http.StatusBadRequest)
	if !strings.Contains(rec.Body.String(), "hatarido_not_future") {
		t.Fatalf("body = %s", rec.Body)
	}
}
