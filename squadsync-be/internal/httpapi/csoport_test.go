package httpapi_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"squadsync-be/internal/httpapi"
)

func as(id string) map[string]string {
	return map[string]string{httpapi.HeaderSecret: secret, httpapi.HeaderFelhasznaloID: id}
}

// call makes a request to the API and returns the recorded response.
func call(t *testing.T, api http.Handler, method, path, body string, headers map[string]string) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	api.ServeHTTP(rec, request(method, path, body, headers))
	return rec
}

func decode[T any](t *testing.T, rec *httptest.ResponseRecorder) T {
	t.Helper()
	var v T
	if err := json.NewDecoder(rec.Body).Decode(&v); err != nil {
		t.Fatalf("decode %q: %v", rec.Body.String(), err)
	}
	return v
}

func expectStatus(t *testing.T, rec *httptest.ResponseRecorder, want int) {
	t.Helper()
	if rec.Code != want {
		t.Fatalf("status = %d, want %d (body %s)", rec.Code, want, rec.Body)
	}
}

func login(t *testing.T, api http.Handler, id, discordID, name string) {
	t.Helper()
	rec := call(t, api, http.MethodPost, "/felhasznalo/first-login", `{"discordId":"`+discordID+`","name":"`+name+`"}`, as(id))
	expectStatus(t, rec, http.StatusOK)
}

type csoportResp struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Admin bool   `json:"admin"`
	Tags  []struct {
		FelhasznaloID string `json:"felhasznaloId"`
		Name          string `json:"name"`
		Admin         bool   `json:"admin"`
	} `json:"tags"`
}

type meghivoResp struct {
	ID    string `json:"id"`
	Token string `json:"token"`
}

func TestCsoportRoutesRequireTheSecretAndAFelhasznalo(t *testing.T) {
	api := newAPI(t)
	for _, tc := range []struct {
		method, path string
		headers      map[string]string
		want         int
	}{
		{http.MethodGet, "/csoport", nil, http.StatusUnauthorized},
		{http.MethodGet, "/csoport", map[string]string{httpapi.HeaderSecret: secret}, http.StatusBadRequest},
		{http.MethodPost, "/csoport-meghivo/abc/join", map[string]string{httpapi.HeaderSecret: secret}, http.StatusBadRequest},
		{http.MethodGet, "/csoport-meghivo/abc", nil, http.StatusUnauthorized},
	} {
		rec := call(t, api, tc.method, tc.path, "", tc.headers)
		if rec.Code != tc.want {
			t.Errorf("%s %s: status = %d, want %d", tc.method, tc.path, rec.Code, tc.want)
		}
	}
}

func TestCreateCsoportThenListAndGetIt(t *testing.T) {
	api := newAPI(t)
	login(t, api, "anna", "111", "Anna")

	rec := call(t, api, http.MethodPost, "/csoport", `{"name":"Péntek esték"}`, as("anna"))
	expectStatus(t, rec, http.StatusCreated)
	created := decode[csoportResp](t, rec)
	if created.ID == "" || created.Name != "Péntek esték" {
		t.Fatalf("created = %+v", created)
	}

	rec = call(t, api, http.MethodGet, "/csoport", "", as("anna"))
	expectStatus(t, rec, http.StatusOK)
	list := decode[[]struct {
		csoportResp
		TagCount int `json:"tagCount"`
	}](t, rec)
	if len(list) != 1 || list[0].ID != created.ID || !list[0].Admin || list[0].TagCount != 1 {
		t.Fatalf("list = %+v, want the new Csoport as Admin with 1 Tag", list)
	}

	rec = call(t, api, http.MethodGet, "/csoport/"+created.ID, "", as("anna"))
	expectStatus(t, rec, http.StatusOK)
	got := decode[csoportResp](t, rec)
	if !got.Admin || len(got.Tags) != 1 || got.Tags[0].Name != "Anna" || !got.Tags[0].Admin {
		t.Fatalf("detail = %+v", got)
	}
}

func TestEmptyListOfCsoportokIsAnArray(t *testing.T) {
	api := newAPI(t)
	login(t, api, "anna", "111", "Anna")
	rec := call(t, api, http.MethodGet, "/csoport", "", as("anna"))
	expectStatus(t, rec, http.StatusOK)
	if got := rec.Body.String(); got != "[]\n" {
		t.Fatalf("body = %q, want an empty JSON array", got)
	}
}

func TestCreateCsoportRejectsBadInput(t *testing.T) {
	api := newAPI(t)
	login(t, api, "anna", "111", "Anna")
	for name, body := range map[string]string{"blank name": `{"name":"  "}`, "not JSON": `nope`} {
		rec := call(t, api, http.MethodPost, "/csoport", body, as("anna"))
		if rec.Code != http.StatusBadRequest {
			t.Errorf("%s: status = %d, want 400", name, rec.Code)
		}
	}
}

func TestNonTagGets404ForCsoportAndItsMeghivok(t *testing.T) {
	api := newAPI(t)
	login(t, api, "anna", "111", "Anna")
	login(t, api, "dani", "444", "Dani")
	created := decode[csoportResp](t, call(t, api, http.MethodPost, "/csoport", `{"name":"Titkos"}`, as("anna")))
	m := decode[meghivoResp](t, call(t, api, http.MethodPost, "/csoport/"+created.ID+"/meghivo", "", as("anna")))

	for _, tc := range []struct{ method, path string }{
		{http.MethodGet, "/csoport/" + created.ID},
		{http.MethodGet, "/csoport/" + created.ID + "/meghivo"},
		{http.MethodPost, "/csoport/" + created.ID + "/meghivo"},
		{http.MethodDelete, "/csoport/" + created.ID + "/meghivo/" + m.ID},
		{http.MethodGet, "/csoport/no-such-csoport"},
	} {
		rec := call(t, api, tc.method, tc.path, "", as("dani"))
		if rec.Code != http.StatusNotFound {
			t.Errorf("%s %s as a non-Tag: status = %d, want 404", tc.method, tc.path, rec.Code)
		}
	}
}

func TestMeghivoFlowOverHTTP(t *testing.T) {
	api := newAPI(t)
	login(t, api, "anna", "111", "Anna")
	login(t, api, "bela", "222", "Béla")
	created := decode[csoportResp](t, call(t, api, http.MethodPost, "/csoport", `{"name":"Péntek esték"}`, as("anna")))

	rec := call(t, api, http.MethodPost, "/csoport/"+created.ID+"/meghivo", "", as("anna"))
	expectStatus(t, rec, http.StatusCreated)
	m := decode[meghivoResp](t, rec)
	if len(m.Token) < 43 {
		t.Fatalf("token %q is too short", m.Token)
	}

	rec = call(t, api, http.MethodGet, "/csoport/"+created.ID+"/meghivo", "", as("anna"))
	expectStatus(t, rec, http.StatusOK)
	if list := decode[[]meghivoResp](t, rec); len(list) != 1 || list[0].Token != m.Token {
		t.Fatalf("meghivo list = %+v", list)
	}

	// The preview needs only the secret: the visitor hasn't logged in yet.
	rec = call(t, api, http.MethodGet, "/csoport-meghivo/"+m.Token, "", map[string]string{httpapi.HeaderSecret: secret})
	expectStatus(t, rec, http.StatusOK)
	if got := rec.Body.String(); got != "{\"name\":\"Péntek esték\"}\n" {
		t.Fatalf("preview body = %q, want only the name", got)
	}

	rec = call(t, api, http.MethodPost, "/csoport-meghivo/"+m.Token+"/join", "", as("bela"))
	expectStatus(t, rec, http.StatusOK)
	if j := decode[struct {
		ID     string
		Joined bool
	}](t, rec); j.ID != created.ID || !j.Joined {
		t.Fatalf("join = %+v, want Joined in %s", j, created.ID)
	}
	rec = call(t, api, http.MethodPost, "/csoport-meghivo/"+m.Token+"/join", "", as("bela"))
	expectStatus(t, rec, http.StatusOK)
	if j := decode[struct{ Joined bool }](t, rec); j.Joined {
		t.Fatalf("second join reported Joined")
	}

	rec = call(t, api, http.MethodGet, "/csoport/"+created.ID, "", as("bela"))
	expectStatus(t, rec, http.StatusOK)
	if d := decode[csoportResp](t, rec); d.Admin || len(d.Tags) != 2 {
		t.Fatalf("detail as Béla = %+v, want a non-Admin view of 2 Tags", d)
	}

	rec = call(t, api, http.MethodDelete, "/csoport/"+created.ID+"/meghivo/"+m.ID, "", as("bela"))
	expectStatus(t, rec, http.StatusNoContent)

	// A revoked link answers 410 Gone, on the preview and on joining.
	login(t, api, "cili", "333", "Cili")
	expectStatus(t, call(t, api, http.MethodGet, "/csoport-meghivo/"+m.Token, "", map[string]string{httpapi.HeaderSecret: secret}), http.StatusGone)
	expectStatus(t, call(t, api, http.MethodPost, "/csoport-meghivo/"+m.Token+"/join", "", as("cili")), http.StatusGone)

	// An existing Tag still gets in through a revoked link.
	rec = call(t, api, http.MethodGet, "/csoport-meghivo/"+m.Token+"/tag", "", as("bela"))
	expectStatus(t, rec, http.StatusOK)
	if got := decode[struct {
		CsoportID string `json:"csoportId"`
	}](t, rec); got.CsoportID != created.ID {
		t.Fatalf("tag lookup = %+v, want %s", got, created.ID)
	}
	expectStatus(t, call(t, api, http.MethodGet, "/csoport-meghivo/"+m.Token+"/tag", "", as("cili")), http.StatusNotFound)
	expectStatus(t, call(t, api, http.MethodPost, "/csoport-meghivo/"+m.Token+"/join", "", as("bela")), http.StatusOK)

	rec = call(t, api, http.MethodGet, "/csoport/"+created.ID+"/meghivo", "", as("anna"))
	if list := decode[[]meghivoResp](t, rec); len(list) != 0 {
		t.Fatalf("revoked link still listed: %+v", list)
	}
}

func TestUnknownTokenAnswers404(t *testing.T) {
	api := newAPI(t)
	login(t, api, "anna", "111", "Anna")
	expectStatus(t, call(t, api, http.MethodGet, "/csoport-meghivo/nope", "", map[string]string{httpapi.HeaderSecret: secret}), http.StatusNotFound)
	expectStatus(t, call(t, api, http.MethodPost, "/csoport-meghivo/nope/join", "", as("anna")), http.StatusNotFound)
}

func TestUnknownFelhasznaloIsNotMistakenForAnUnknownToken(t *testing.T) {
	api := newAPI(t)
	login(t, api, "anna", "111", "Anna")
	created := decode[csoportResp](t, call(t, api, http.MethodPost, "/csoport", `{"name":"X"}`, as("anna")))
	m := decode[meghivoResp](t, call(t, api, http.MethodPost, "/csoport/"+created.ID+"/meghivo", "", as("anna")))

	expectStatus(t, call(t, api, http.MethodPost, "/csoport-meghivo/"+m.Token+"/join", "", as("nobody")), http.StatusForbidden)
}
