package httpapi_test

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

type szerkesztesDetailResp struct {
	Title       string `json:"title"`
	Description string `json:"description"`
	Helyszin    struct {
		Name string `json:"name"`
		URL  string `json:"url"`
	} `json:"helyszin"`
	Idopontok []struct {
		ID        string    `json:"id"`
		Start     time.Time `json:"start"`
		AddedBy   string    `json:"addedBy"`
		Removable bool      `json:"removable"`
	} `json:"idopontok"`
}

func errorCode(t *testing.T, rec *httptest.ResponseRecorder) string {
	t.Helper()
	return decode[struct {
		Code string `json:"code"`
	}](t, rec).Code
}

func TestSzervezoEditsTheDetailsOverHTTP(t *testing.T) {
	api, esemenyID, _ := lifecycleSetup(t)

	expectStatus(t, call(t, api, http.MethodPut, "/esemeny/"+esemenyID,
		`{"title":"Új cím","description":"Új","helyszin":{"name":"Hely","url":"https://maps.app.goo.gl/x"}}`, as("anna")), http.StatusNoContent)
	d := decode[szerkesztesDetailResp](t, call(t, api, http.MethodGet, "/esemeny/"+esemenyID, "", as("bela")))
	if d.Title != "Új cím" || d.Description != "Új" || d.Helyszin.Name != "Hely" || d.Helyszin.URL != "https://maps.app.goo.gl/x" {
		t.Fatalf("detail = %+v", d)
	}

	for name, tc := range map[string]struct {
		who, body string
		want      int
	}{
		"another Tag":     {"bela", `{"title":"X"}`, http.StatusForbidden},
		"a non-Tag":       {"dani", `{"title":"X"}`, http.StatusNotFound},
		"no title":        {"anna", `{"title":" "}`, http.StatusBadRequest},
		"bad URL":         {"anna", `{"title":"X","helyszin":{"url":"nope"}}`, http.StatusBadRequest},
		"not JSON":        {"anna", `nope`, http.StatusBadRequest},
		"unknown Esemény": {"anna", `{"title":"X"}`, http.StatusNotFound},
	} {
		id := esemenyID
		if name == "unknown Esemény" {
			id = "no-such-esemeny"
		}
		if rec := call(t, api, http.MethodPut, "/esemeny/"+id, tc.body, as(tc.who)); rec.Code != tc.want {
			t.Errorf("%s: status = %d, want %d (body %s)", name, rec.Code, tc.want, rec.Body)
		}
	}

	expectStatus(t, call(t, api, http.MethodPost, "/esemeny/"+esemenyID+"/lemondas", "", as("anna")), http.StatusNoContent)
	rec := call(t, api, http.MethodPut, "/esemeny/"+esemenyID, `{"title":"X"}`, as("anna"))
	expectStatus(t, rec, http.StatusConflict)
	if code := errorCode(t, rec); code != "esemeny_elmaradt" {
		t.Fatalf("code = %q, want esemeny_elmaradt", code)
	}
}

func TestAddingAndRemovingIdopontokOverHTTP(t *testing.T) {
	api, esemenyID, idopontok := lifecycleSetup(t)

	rec := call(t, api, http.MethodPost, "/esemeny/"+esemenyID+"/idopont", `{"start":"2026-12-20T18:00","end":"","egeszNapos":false}`, as("bela"))
	expectStatus(t, rec, http.StatusCreated)
	added := decode[struct {
		ID string `json:"id"`
	}](t, rec).ID
	d := decode[szerkesztesDetailResp](t, call(t, api, http.MethodGet, "/esemeny/"+esemenyID, "", as("bela")))
	if len(d.Idopontok) != 3 || d.Idopontok[2].ID != added || d.Idopontok[2].AddedBy != "bela" {
		t.Fatalf("idopontok = %+v", d.Idopontok)
	}
	// Béla may remove only the one he added; Anna (Szervező) any.
	for n, want := range []bool{false, false, true} {
		if d.Idopontok[n].Removable != want {
			t.Errorf("idopont %d removable for Béla = %v, want %v", n, d.Idopontok[n].Removable, want)
		}
	}

	whole := call(t, api, http.MethodPost, "/esemeny/"+esemenyID+"/idopont", `{"start":"2026-12-24","end":"2026-12-26","egeszNapos":true}`, as("anna"))
	expectStatus(t, whole, http.StatusCreated)

	for name, tc := range map[string]struct {
		who, body string
		want      int
	}{
		"malformed":    {"bela", `{"start":"holnap"}`, http.StatusBadRequest},
		"skipped hour": {"bela", `{"start":"2026-03-29T02:30"}`, http.StatusUnprocessableEntity},
		"a non-Tag":    {"dani", `{"start":"2026-12-21T18:00"}`, http.StatusNotFound},
		"not JSON":     {"bela", `nope`, http.StatusBadRequest},
	} {
		if rec := call(t, api, http.MethodPost, "/esemeny/"+esemenyID+"/idopont", tc.body, as(tc.who)); rec.Code != tc.want {
			t.Errorf("add, %s: status = %d, want %d (body %s)", name, rec.Code, tc.want, rec.Body)
		}
	}

	// Another Tag may not remove it; its adder and the Szervező may.
	path := "/esemeny/" + esemenyID + "/idopont/" + added
	expectStatus(t, call(t, api, http.MethodDelete, path, "", as("dani")), http.StatusNotFound)
	expectStatus(t, call(t, api, http.MethodDelete, "/esemeny/"+esemenyID+"/idopont/"+idopontok[0], "", as("bela")), http.StatusForbidden)
	expectStatus(t, call(t, api, http.MethodDelete, path, "", as("bela")), http.StatusNoContent)
	expectStatus(t, call(t, api, http.MethodDelete, path, "", as("bela")), http.StatusNotFound)
	expectStatus(t, call(t, api, http.MethodDelete, "/esemeny/"+esemenyID+"/idopont/"+idopontok[0], "", as("anna")), http.StatusNoContent)
}

func TestRemovalRefusalsCarryTheirCodes(t *testing.T) {
	api, esemenyID, idopontok := lifecycleSetup(t)
	added := decode[struct {
		ID string `json:"id"`
	}](t, call(t, api, http.MethodPost, "/esemeny/"+esemenyID+"/idopont", `{"start":"2026-12-20T18:00"}`, as("bela"))).ID
	expectStatus(t, call(t, api, http.MethodPut, "/esemeny/"+esemenyID+"/idopont/"+added+"/visszajelzes", `{"valasz":"igen"}`, as("anna")), http.StatusNoContent)

	rec := call(t, api, http.MethodDelete, "/esemeny/"+esemenyID+"/idopont/"+added, "", as("bela"))
	expectStatus(t, rec, http.StatusConflict)
	if code := errorCode(t, rec); code != "idopont_has_answers" {
		t.Fatalf("code = %q, want idopont_has_answers", code)
	}

	// Anna may remove it with its answers; then only the original two stay.
	expectStatus(t, call(t, api, http.MethodDelete, "/esemeny/"+esemenyID+"/idopont/"+added, "", as("anna")), http.StatusNoContent)
	expectStatus(t, call(t, api, http.MethodDelete, "/esemeny/"+esemenyID+"/idopont/"+idopontok[0], "", as("anna")), http.StatusNoContent)
	rec = call(t, api, http.MethodDelete, "/esemeny/"+esemenyID+"/idopont/"+idopontok[1], "", as("anna"))
	expectStatus(t, rec, http.StatusConflict)
	if code := errorCode(t, rec); code != "last_idopont" {
		t.Fatalf("code = %q, want last_idopont", code)
	}

	// After confirming, neither adding nor removing is possible.
	expectStatus(t, call(t, api, http.MethodPost, "/esemeny/"+esemenyID+"/veglegesites", `{"idopontId":"`+idopontok[1]+`"}`, as("anna")), http.StatusNoContent)
	for name, rec := range map[string]*httptest.ResponseRecorder{
		"add":    call(t, api, http.MethodPost, "/esemeny/"+esemenyID+"/idopont", `{"start":"2026-12-21T18:00"}`, as("bela")),
		"remove": call(t, api, http.MethodDelete, "/esemeny/"+esemenyID+"/idopont/"+idopontok[1], "", as("anna")),
	} {
		expectStatus(t, rec, http.StatusConflict)
		if code := errorCode(t, rec); code != "not_egyeztetes" {
			t.Errorf("%s: code = %q, want not_egyeztetes", name, code)
		}
	}
}

func TestTheIdopontCapAnswers409(t *testing.T) {
	api, esemenyID, _ := lifecycleSetup(t)
	for i := 0; i < 18; i++ {
		day := time.Date(2027, 1, 1+i, 0, 0, 0, 0, time.UTC).Format("2006-01-02")
		expectStatus(t, call(t, api, http.MethodPost, "/esemeny/"+esemenyID+"/idopont", `{"start":"`+day+`T18:00"}`, as("bela")), http.StatusCreated)
	}
	rec := call(t, api, http.MethodPost, "/esemeny/"+esemenyID+"/idopont", `{"start":"2027-02-01T18:00"}`, as("bela"))
	expectStatus(t, rec, http.StatusConflict)
	if code := errorCode(t, rec); code != "too_many_idopontok" {
		t.Fatalf("code = %q, want too_many_idopontok", code)
	}
}

func TestCopyingAnEsemenyOverHTTP(t *testing.T) {
	api, esemenyID, _ := lifecycleSetup(t)

	rec := call(t, api, http.MethodPost, "/esemeny/"+esemenyID+"/masolas", "", as("bela"))
	expectStatus(t, rec, http.StatusCreated)
	cp := decode[esemenyResp](t, rec)
	if cp.ID == esemenyID || cp.SzervezoID != "bela" || cp.Allapot != "egyeztetes" || cp.Title != "Társasjáték" {
		t.Fatalf("copy = %+v", cp)
	}
	d := decode[szerkesztesDetailResp](t, call(t, api, http.MethodGet, "/esemeny/"+cp.ID, "", as("anna")))
	// proposal's first Időpont is 2026-12-05T20:00 Budapest (19:00 UTC).
	if len(d.Idopontok) != 2 || !d.Idopontok[0].Start.Equal(time.Date(2026, 12, 12, 19, 0, 0, 0, time.UTC)) {
		t.Fatalf("idopontok = %+v, want a week later", d.Idopontok)
	}

	expectStatus(t, call(t, api, http.MethodPost, "/esemeny/"+esemenyID+"/masolas", "", as("dani")), http.StatusNotFound)
	expectStatus(t, call(t, api, http.MethodPost, "/esemeny/no-such-esemeny/masolas", "", as("anna")), http.StatusNotFound)
	// Any state may be copied.
	expectStatus(t, call(t, api, http.MethodPost, "/esemeny/"+esemenyID+"/lemondas", "", as("anna")), http.StatusNoContent)
	expectStatus(t, call(t, api, http.MethodPost, "/esemeny/"+esemenyID+"/masolas", "", as("bela")), http.StatusCreated)
}

func TestSzerkesztesRoutesRequireAFelhasznalo(t *testing.T) {
	api := newAPI(t)
	headers := map[string]string{"X-Internal-Secret": secret}
	for _, tc := range []struct{ method, path string }{
		{http.MethodPut, "/esemeny/x"},
		{http.MethodPost, "/esemeny/x/masolas"},
		{http.MethodPost, "/esemeny/x/idopont"},
		{http.MethodDelete, "/esemeny/x/idopont/y"},
	} {
		expectStatus(t, call(t, api, tc.method, tc.path, "{}", headers), http.StatusBadRequest)
	}
}
