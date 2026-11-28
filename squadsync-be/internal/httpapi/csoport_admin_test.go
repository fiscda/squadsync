package httpapi_test

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"testing"

	"squadsync-be/internal/apptest"
	"squadsync-be/internal/domain"
	"squadsync-be/internal/httpapi"
)

type errorResp struct {
	Error string `json:"error"`
	Code  string `json:"code"`
}

type mutedResp struct {
	Muted bool `json:"muted"`
}

// adminCsoport has Anna create a Csoport with Béla and Cecil as Tags.
func adminCsoport(t *testing.T) (http.Handler, *apptest.Harness, string) {
	t.Helper()
	h := apptest.New(t)
	api := httpapi.New(h.App, secret, slog.New(slog.NewTextHandler(io.Discard, nil)))
	login(t, api, "anna", "111", "Anna")
	login(t, api, "bela", "222", "Béla")
	login(t, api, "cecil", "333", "Cecil")
	created := decode[csoportResp](t, call(t, api, http.MethodPost, "/csoport", `{"name":"Péntek esték"}`, as("anna")))
	m := decode[meghivoResp](t, call(t, api, http.MethodPost, "/csoport/"+created.ID+"/meghivo", "", as("anna")))
	for _, id := range []string{"bela", "cecil"} {
		expectStatus(t, call(t, api, http.MethodPost, "/csoport-meghivo/"+m.Token+"/join", "", as(id)), http.StatusOK)
	}
	return api, h, created.ID
}

func TestRenameCsoportRoute(t *testing.T) {
	api, _, id := adminCsoport(t)
	path := "/csoport/" + id + "/name"

	expectStatus(t, call(t, api, http.MethodPut, path, `{"name":"Új"}`, map[string]string{httpapi.HeaderSecret: secret}), http.StatusBadRequest)
	expectStatus(t, call(t, api, http.MethodPut, path, `nem json`, as("anna")), http.StatusBadRequest)
	expectStatus(t, call(t, api, http.MethodPut, path, `{"name":"  "}`, as("anna")), http.StatusBadRequest)
	expectStatus(t, call(t, api, http.MethodPut, path, `{"name":"Új"}`, as("bela")), http.StatusForbidden)
	expectStatus(t, call(t, api, http.MethodPut, "/csoport/nincs/name", `{"name":"Új"}`, as("anna")), http.StatusNotFound)

	rec := call(t, api, http.MethodPut, path, `{"name":" Szombat esték "}`, as("anna"))
	expectStatus(t, rec, http.StatusOK)
	if got := decode[csoportResp](t, rec); got.ID != id || got.Name != "Szombat esték" {
		t.Fatalf("response = %+v, want the renamed Csoport", got)
	}
	if d := decode[csoportResp](t, call(t, api, http.MethodGet, "/csoport/"+id, "", as("bela"))); d.Name != "Szombat esték" {
		t.Fatalf("name = %q after renaming", d.Name)
	}
}

func TestAdminRoleRoutesAndTheLastAdminRule(t *testing.T) {
	api, _, id := adminCsoport(t)
	bela := "/csoport/" + id + "/tag/bela/admin"
	anna := "/csoport/" + id + "/tag/anna/admin"

	expectStatus(t, call(t, api, http.MethodPut, bela, `{"admin":true}`, map[string]string{httpapi.HeaderSecret: secret}), http.StatusBadRequest)
	expectStatus(t, call(t, api, http.MethodPut, bela, `{"admin":true}`, as("cecil")), http.StatusForbidden)
	expectStatus(t, call(t, api, http.MethodPut, "/csoport/"+id+"/tag/nincs/admin", `{"admin":true}`, as("anna")), http.StatusNotFound)

	rec := call(t, api, http.MethodPut, anna, `{"admin":false}`, as("anna"))
	expectStatus(t, rec, http.StatusConflict)
	if e := decode[errorResp](t, rec); e.Code != "last_admin" {
		t.Fatalf("error = %+v, want code last_admin", e)
	}

	expectStatus(t, call(t, api, http.MethodPut, bela, `{"admin":true}`, as("anna")), http.StatusNoContent)
	expectStatus(t, call(t, api, http.MethodPut, anna, `{"admin":false}`, as("anna")), http.StatusNoContent)
	if d := decode[csoportResp](t, call(t, api, http.MethodGet, "/csoport/"+id, "", as("anna"))); d.Admin {
		t.Fatal("Anna is still Admin")
	}
}

func TestRemoveTagAndLeaveRoutes(t *testing.T) {
	api, h, id := adminCsoport(t)
	cecil := "/csoport/" + id + "/tag/cecil"

	expectStatus(t, call(t, api, http.MethodDelete, cecil, "", map[string]string{httpapi.HeaderSecret: secret}), http.StatusBadRequest)
	expectStatus(t, call(t, api, http.MethodDelete, cecil, "", as("bela")), http.StatusForbidden)
	expectStatus(t, call(t, api, http.MethodDelete, "/csoport/"+id+"/tag/nincs", "", as("anna")), http.StatusNotFound)

	// The Szervezo of an active Esemeny can't leave.
	err := h.Store.InsertEsemeny(context.Background(), domain.Esemeny{
		ID: "e1", CsoportID: id, Title: "Játék", SzervezoID: "cecil", Allapot: domain.Egyeztetes, CreatedAt: apptest.Start,
	})
	if err != nil {
		t.Fatalf("InsertEsemeny: %v", err)
	}
	rec := call(t, api, http.MethodPost, "/csoport/"+id+"/kilepes", "", as("cecil"))
	expectStatus(t, rec, http.StatusConflict)
	if e := decode[errorResp](t, rec); e.Code != "szervezo_of_active_esemeny" {
		t.Fatalf("error = %+v, want code szervezo_of_active_esemeny", e)
	}
	// The last Admin can't leave while others remain.
	rec = call(t, api, http.MethodPost, "/csoport/"+id+"/kilepes", "", as("anna"))
	expectStatus(t, rec, http.StatusConflict)
	if e := decode[errorResp](t, rec); e.Code != "last_admin" {
		t.Fatalf("error = %+v, want code last_admin", e)
	}

	expectStatus(t, call(t, api, http.MethodPost, "/csoport/"+id+"/kilepes", "", as("bela")), http.StatusNoContent)
	expectStatus(t, call(t, api, http.MethodGet, "/csoport/"+id, "", as("bela")), http.StatusNotFound)
	expectStatus(t, call(t, api, http.MethodPost, "/csoport/"+id+"/kilepes", "", as("bela")), http.StatusNotFound)
}

func TestMuteRoute(t *testing.T) {
	api, _, id := adminCsoport(t)
	path := "/csoport/" + id + "/nemitas"

	expectStatus(t, call(t, api, http.MethodPut, path, `{"muted":true}`, map[string]string{httpapi.HeaderSecret: secret}), http.StatusBadRequest)
	expectStatus(t, call(t, api, http.MethodPut, path, `nem json`, as("bela")), http.StatusBadRequest)

	expectStatus(t, call(t, api, http.MethodPut, path, `{"muted":true}`, as("bela")), http.StatusNoContent)
	if d := decode[mutedResp](t, call(t, api, http.MethodGet, "/csoport/"+id, "", as("bela"))); !d.Muted {
		t.Fatal("Béla isn't shown as muted")
	}
	if d := decode[mutedResp](t, call(t, api, http.MethodGet, "/csoport/"+id, "", as("cecil"))); d.Muted {
		t.Fatal("Cecil is shown as muted")
	}
	expectStatus(t, call(t, api, http.MethodPut, path, `{"muted":false}`, as("bela")), http.StatusNoContent)
	if d := decode[mutedResp](t, call(t, api, http.MethodGet, "/csoport/"+id, "", as("bela"))); d.Muted {
		t.Fatal("Béla is still muted")
	}

	login(t, api, "kivul", "444", "Kívül")
	expectStatus(t, call(t, api, http.MethodPut, path, `{"muted":true}`, as("kivul")), http.StatusNotFound)
}
