package httpapi_test

import (
	"net/http"
	"testing"
	"time"
)

type esemenyResp struct {
	ID         string `json:"id"`
	CsoportID  string `json:"csoportId"`
	Title      string `json:"title"`
	SzervezoID string `json:"szervezoId"`
	Allapot    string `json:"allapot"`
	Helyszin   struct {
		Name string `json:"name"`
		URL  string `json:"url"`
	} `json:"helyszin"`
}

type idopontResp struct {
	ID             string     `json:"id"`
	Start          time.Time  `json:"start"`
	End            *time.Time `json:"end"`
	Igen           int        `json:"igen"`
	Talan          int        `json:"talan"`
	Nem            int        `json:"nem"`
	Helyezes       *int       `json:"helyezes"`
	Sajat          *string    `json:"sajat"`
	Visszajelzesek []struct {
		Name   string `json:"name"`
		Valasz string `json:"valasz"`
	} `json:"visszajelzesek"`
}

type esemenyDetailResp struct {
	esemenyResp
	CsoportName  string        `json:"csoportName"`
	SzervezoName string        `json:"szervezoName"`
	Idopontok    []idopontResp `json:"idopontok"`
}

const proposal = `{
	"title": "Társasjáték",
	"description": "Catan",
	"helyszin": {"name": "Nálam", "url": "https://maps.app.goo.gl/abc"},
	"idopontok": [
		{"start": "2026-12-05T20:00", "end": "2026-12-05T23:00"},
		{"start": "2026-12-06T18:00", "end": ""}
	]
}`

func TestProposeListGetAndAnswerAnEsemeny(t *testing.T) {
	api := newAPI(t)
	login(t, api, "anna", "111", "Anna")
	c := decode[csoportResp](t, call(t, api, http.MethodPost, "/csoport", `{"name":"Péntek esték"}`, as("anna")))

	rec := call(t, api, http.MethodPost, "/csoport/"+c.ID+"/esemeny", proposal, as("anna"))
	expectStatus(t, rec, http.StatusCreated)
	e := decode[esemenyResp](t, rec)
	if e.ID == "" || e.CsoportID != c.ID || e.SzervezoID != "anna" || e.Allapot != "egyeztetes" || e.Helyszin.Name != "Nálam" {
		t.Fatalf("created = %+v", e)
	}

	rec = call(t, api, http.MethodGet, "/csoport/"+c.ID+"/esemeny", "", as("anna"))
	expectStatus(t, rec, http.StatusOK)
	list := decode[[]struct {
		esemenyResp
		IdopontCount int        `json:"idopontCount"`
		FirstStart   *time.Time `json:"firstStart"`
	}](t, rec)
	if len(list) != 1 || list[0].ID != e.ID || list[0].IdopontCount != 2 || list[0].FirstStart == nil {
		t.Fatalf("list = %+v", list)
	}

	rec = call(t, api, http.MethodGet, "/esemeny/"+e.ID, "", as("anna"))
	expectStatus(t, rec, http.StatusOK)
	d := decode[esemenyDetailResp](t, rec)
	if d.CsoportName != "Péntek esték" || d.SzervezoName != "Anna" || len(d.Idopontok) != 2 {
		t.Fatalf("detail = %+v", d)
	}
	first := d.Idopontok[0]
	if !first.Start.Equal(time.Date(2026, 12, 5, 19, 0, 0, 0, time.UTC)) || first.End == nil {
		t.Fatalf("first idopont = %+v, want 19:00 UTC with an end", first)
	}
	if first.Helyezes != nil || first.Sajat != nil || first.Visszajelzesek == nil {
		t.Fatalf("unanswered idopont = %+v, want null place and answer, empty list", first)
	}
	if d.Idopontok[1].End != nil {
		t.Fatalf("second idopont has an end: %+v", d.Idopontok[1])
	}

	path := "/esemeny/" + e.ID + "/idopont/" + first.ID + "/visszajelzes"
	expectStatus(t, call(t, api, http.MethodPut, path, `{"valasz":"talan"}`, as("anna")), http.StatusNoContent)
	expectStatus(t, call(t, api, http.MethodPut, path, `{"valasz":"igen"}`, as("anna")), http.StatusNoContent)

	d = decode[esemenyDetailResp](t, call(t, api, http.MethodGet, "/esemeny/"+e.ID, "", as("anna")))
	first = d.Idopontok[0]
	if first.Igen != 1 || first.Talan != 0 || first.Helyezes == nil || *first.Helyezes != 1 ||
		first.Sajat == nil || *first.Sajat != "igen" || len(first.Visszajelzesek) != 1 || first.Visszajelzesek[0].Name != "Anna" {
		t.Fatalf("answered idopont = %+v", first)
	}
}

func TestEsemenyRoutesRejectBadInput(t *testing.T) {
	api := newAPI(t)
	login(t, api, "anna", "111", "Anna")
	c := decode[csoportResp](t, call(t, api, http.MethodPost, "/csoport", `{"name":"X"}`, as("anna")))
	e := decode[esemenyResp](t, call(t, api, http.MethodPost, "/csoport/"+c.ID+"/esemeny", proposal, as("anna")))
	d := decode[esemenyDetailResp](t, call(t, api, http.MethodGet, "/esemeny/"+e.ID, "", as("anna")))

	for name, tc := range map[string]struct{ method, path, body string }{
		"no title":         {http.MethodPost, "/csoport/" + c.ID + "/esemeny", `{"title":"","idopontok":[{"start":"2026-12-05T20:00"}]}`},
		"end before start": {http.MethodPost, "/csoport/" + c.ID + "/esemeny", `{"title":"X","idopontok":[{"start":"2026-12-05T20:00","end":"2026-12-05T19:00"}]}`},
		"bad URL":          {http.MethodPost, "/csoport/" + c.ID + "/esemeny", `{"title":"X","helyszin":{"url":"nope"},"idopontok":[{"start":"2026-12-05T20:00"}]}`},
		"not JSON":         {http.MethodPost, "/csoport/" + c.ID + "/esemeny", `nope`},
		"bad valasz":       {http.MethodPut, "/esemeny/" + e.ID + "/idopont/" + d.Idopontok[0].ID + "/visszajelzes", `{"valasz":"maybe"}`},
	} {
		if rec := call(t, api, tc.method, tc.path, tc.body, as("anna")); rec.Code != http.StatusBadRequest {
			t.Errorf("%s: status = %d, want 400 (body %s)", name, rec.Code, rec.Body)
		}
	}
}

func TestNonTagGets404ForEsemenyRoutes(t *testing.T) {
	api := newAPI(t)
	login(t, api, "anna", "111", "Anna")
	login(t, api, "dani", "444", "Dani")
	c := decode[csoportResp](t, call(t, api, http.MethodPost, "/csoport", `{"name":"Titkos"}`, as("anna")))
	e := decode[esemenyResp](t, call(t, api, http.MethodPost, "/csoport/"+c.ID+"/esemeny", proposal, as("anna")))
	d := decode[esemenyDetailResp](t, call(t, api, http.MethodGet, "/esemeny/"+e.ID, "", as("anna")))

	for _, tc := range []struct{ method, path, body string }{
		{http.MethodPost, "/csoport/" + c.ID + "/esemeny", proposal},
		{http.MethodGet, "/csoport/" + c.ID + "/esemeny", ""},
		{http.MethodGet, "/esemeny/" + e.ID, ""},
		{http.MethodGet, "/esemeny/no-such-esemeny", ""},
		{http.MethodPut, "/esemeny/" + e.ID + "/idopont/" + d.Idopontok[0].ID + "/visszajelzes", `{"valasz":"igen"}`},
	} {
		if rec := call(t, api, tc.method, tc.path, tc.body, as("dani")); rec.Code != http.StatusNotFound {
			t.Errorf("%s %s as a non-Tag: status = %d, want 404", tc.method, tc.path, rec.Code)
		}
	}
}

func TestEsemenyRoutesRequireAFelhasznalo(t *testing.T) {
	api := newAPI(t)
	headers := map[string]string{"X-Internal-Secret": secret}
	expectStatus(t, call(t, api, http.MethodGet, "/esemeny/x", "", headers), http.StatusBadRequest)
	expectStatus(t, call(t, api, http.MethodGet, "/esemeny/x", "", nil), http.StatusUnauthorized)
}
