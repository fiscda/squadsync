package httpapi_test

import (
	"net/http"
	"reflect"
	"testing"

	"squadsync-be/internal/httpapi"
)

// The API's clock starts on 2026-03-02, so that is today.

type raeresDayResp struct {
	Day     string `json:"day"`
	Allapot string `json:"allapot"`
}

type heatmapDayResp struct {
	Day    string `json:"day"`
	Szabad int    `json:"szabad"`
	Osszes int    `json:"osszes"`
}

func TestRaeresRoutesRequireTheSecretAndAFelhasznalo(t *testing.T) {
	api := newAPI(t)
	for _, tc := range []struct {
		method, path string
		headers      map[string]string
		want         int
	}{
		{http.MethodGet, "/raeres?from=2026-03-02&to=2026-03-03", nil, http.StatusUnauthorized},
		{http.MethodGet, "/raeres?from=2026-03-02&to=2026-03-03", map[string]string{httpapi.HeaderSecret: secret}, http.StatusBadRequest},
		{http.MethodPut, "/raeres", map[string]string{httpapi.HeaderSecret: secret}, http.StatusBadRequest},
		{http.MethodGet, "/csoport/x/raeres?from=2026-03-02&to=2026-03-03", nil, http.StatusUnauthorized},
		{http.MethodGet, "/esemeny/x/raeres", map[string]string{httpapi.HeaderSecret: secret}, http.StatusBadRequest},
	} {
		rec := call(t, api, tc.method, tc.path, "", tc.headers)
		if rec.Code != tc.want {
			t.Errorf("%s %s: status = %d, want %d", tc.method, tc.path, rec.Code, tc.want)
		}
	}
}

func TestMarkReadAndClearOwnRaeres(t *testing.T) {
	api := newAPI(t)
	login(t, api, "anna", "111", "Anna")

	expectStatus(t, call(t, api, http.MethodPut, "/raeres", `{"from":"2026-03-10","to":"2026-03-12","allapot":"szabad"}`, as("anna")), http.StatusNoContent)
	expectStatus(t, call(t, api, http.MethodPut, "/raeres", `{"from":"2026-03-11","to":"2026-03-11","allapot":"foglalt"}`, as("anna")), http.StatusNoContent)

	rec := call(t, api, http.MethodGet, "/raeres?from=2026-03-01&to=2026-03-31", "", as("anna"))
	expectStatus(t, rec, http.StatusOK)
	want := []raeresDayResp{{"2026-03-10", "szabad"}, {"2026-03-11", "foglalt"}, {"2026-03-12", "szabad"}}
	if got := decode[[]raeresDayResp](t, rec); !reflect.DeepEqual(got, want) {
		t.Fatalf("raeres = %+v, want %+v", got, want)
	}

	expectStatus(t, call(t, api, http.MethodPut, "/raeres", `{"from":"2026-03-10","to":"2026-03-12","allapot":""}`, as("anna")), http.StatusNoContent)
	rec = call(t, api, http.MethodGet, "/raeres?from=2026-03-01&to=2026-03-31", "", as("anna"))
	if got := decode[[]raeresDayResp](t, rec); got == nil || len(got) != 0 {
		t.Fatalf("raeres after clearing = %#v, want an empty array", got)
	}
}

func TestMarkingOutsideTheWindowOrWithBadInputIs400(t *testing.T) {
	api := newAPI(t)
	login(t, api, "anna", "111", "Anna")
	for name, body := range map[string]string{
		"past":         `{"from":"2026-03-01","to":"2026-03-03","allapot":"szabad"}`,
		"too far":      `{"from":"2027-03-02","to":"2027-03-03","allapot":"szabad"}`,
		"backwards":    `{"from":"2026-03-05","to":"2026-03-04","allapot":"szabad"}`,
		"bad allapot":  `{"from":"2026-03-05","to":"2026-03-05","allapot":"talan"}`,
		"bad day":      `{"from":"holnap","to":"2026-03-05","allapot":"szabad"}`,
		"not json":     `{`,
		"missing days": `{"allapot":"szabad"}`,
	} {
		if rec := call(t, api, http.MethodPut, "/raeres", body, as("anna")); rec.Code != http.StatusBadRequest {
			t.Errorf("%s: status = %d, want 400 (body %s)", name, rec.Code, rec.Body)
		}
	}
	if rec := call(t, api, http.MethodGet, "/raeres?from=holnap&to=2026-03-05", "", as("anna")); rec.Code != http.StatusBadRequest {
		t.Errorf("reading with a bad day: status = %d, want 400", rec.Code)
	}
}

func TestMarkingAsAnUnknownFelhasznaloIs403(t *testing.T) {
	api := newAPI(t)
	rec := call(t, api, http.MethodPut, "/raeres", `{"from":"2026-03-05","to":"2026-03-05","allapot":"szabad"}`, as("ghost"))
	expectStatus(t, rec, http.StatusForbidden)
}

func TestHeatmapIsForTagsOnlyAndShowsCounts(t *testing.T) {
	api := newAPI(t)
	login(t, api, "anna", "111", "Anna")
	login(t, api, "bela", "222", "Béla")
	login(t, api, "dora", "444", "Dóra")
	c := decode[csoportResp](t, call(t, api, http.MethodPost, "/csoport", `{"name":"Péntek esték"}`, as("anna")))
	m := decode[meghivoResp](t, call(t, api, http.MethodPost, "/csoport/"+c.ID+"/meghivo", "", as("anna")))
	expectStatus(t, call(t, api, http.MethodPost, "/csoport-meghivo/"+m.Token+"/join", "", as("bela")), http.StatusOK)

	for _, id := range []string{"anna", "bela", "dora"} {
		expectStatus(t, call(t, api, http.MethodPut, "/raeres", `{"from":"2026-03-10","to":"2026-03-10","allapot":"szabad"}`, as(id)), http.StatusNoContent)
	}
	expectStatus(t, call(t, api, http.MethodPut, "/raeres", `{"from":"2026-03-11","to":"2026-03-11","allapot":"szabad"}`, as("anna")), http.StatusNoContent)

	rec := call(t, api, http.MethodGet, "/csoport/"+c.ID+"/raeres?from=2026-03-10&to=2026-03-12", "", as("bela"))
	expectStatus(t, rec, http.StatusOK)
	want := []heatmapDayResp{{"2026-03-10", 2, 2}, {"2026-03-11", 1, 2}, {"2026-03-12", 0, 2}}
	if got := decode[[]heatmapDayResp](t, rec); !reflect.DeepEqual(got, want) {
		t.Fatalf("heatmap = %+v, want %+v", got, want)
	}

	// Dora, who isn't a Tag, learns nothing, not even that it exists.
	expectStatus(t, call(t, api, http.MethodGet, "/csoport/"+c.ID+"/raeres?from=2026-03-10&to=2026-03-12", "", as("dora")), http.StatusNotFound)
	expectStatus(t, call(t, api, http.MethodGet, "/csoport/nincs/raeres?from=2026-03-10&to=2026-03-12", "", as("anna")), http.StatusNotFound)
	expectStatus(t, call(t, api, http.MethodGet, "/csoport/"+c.ID+"/raeres?from=x&to=2026-03-12", "", as("anna")), http.StatusBadRequest)
}

func TestEsemenyRaeresShowsCountsAndTheCallersOwnHint(t *testing.T) {
	api := newAPI(t)
	login(t, api, "anna", "111", "Anna")
	login(t, api, "bela", "222", "Béla")
	login(t, api, "dora", "444", "Dóra")
	c := decode[csoportResp](t, call(t, api, http.MethodPost, "/csoport", `{"name":"Péntek esték"}`, as("anna")))
	m := decode[meghivoResp](t, call(t, api, http.MethodPost, "/csoport/"+c.ID+"/meghivo", "", as("anna")))
	expectStatus(t, call(t, api, http.MethodPost, "/csoport-meghivo/"+m.Token+"/join", "", as("bela")), http.StatusOK)
	e := decode[esemenyResp](t, call(t, api, http.MethodPost, "/csoport/"+c.ID+"/esemeny",
		`{"title":"Túra","idopontok":[{"start":"2026-03-10T10:00","end":"2026-03-11T10:00"},{"start":"2026-03-20T10:00"}]}`, as("anna")))
	expectStatus(t, call(t, api, http.MethodPut, "/raeres", `{"from":"2026-03-10","to":"2026-03-11","allapot":"szabad"}`, as("anna")), http.StatusNoContent)
	expectStatus(t, call(t, api, http.MethodPut, "/raeres", `{"from":"2026-03-10","to":"2026-03-10","allapot":"szabad"}`, as("bela")), http.StatusNoContent)
	expectStatus(t, call(t, api, http.MethodPut, "/raeres", `{"from":"2026-03-20","to":"2026-03-20","allapot":"foglalt"}`, as("bela")), http.StatusNoContent)

	type line struct {
		IdopontID string  `json:"idopontId"`
		Szabad    int     `json:"szabad"`
		Osszes    int     `json:"osszes"`
		Sajat     *string `json:"sajat"`
	}
	rec := call(t, api, http.MethodGet, "/esemeny/"+e.ID+"/raeres", "", as("bela"))
	expectStatus(t, rec, http.StatusOK)
	got := decode[[]line](t, rec)
	if len(got) != 2 || got[0].Szabad != 1 || got[0].Osszes != 2 || got[0].Sajat != nil ||
		got[1].Szabad != 0 || got[1].Osszes != 2 || got[1].Sajat == nil || *got[1].Sajat != "foglalt" {
		t.Fatalf("esemeny raeres for Bela = %+v", got)
	}
	got = decode[[]line](t, call(t, api, http.MethodGet, "/esemeny/"+e.ID+"/raeres", "", as("anna")))
	if got[0].Sajat == nil || *got[0].Sajat != "szabad" || got[1].Sajat != nil {
		t.Fatalf("esemeny raeres for Anna = %+v", got)
	}

	expectStatus(t, call(t, api, http.MethodGet, "/esemeny/"+e.ID+"/raeres", "", as("dora")), http.StatusNotFound)
}
