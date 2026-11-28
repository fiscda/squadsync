package httpapi_test

import (
	"net/http"
	"testing"
)

type szervezoEsemenyResp struct {
	ID      string `json:"id"`
	Title   string `json:"title"`
	Allapot string `json:"allapot"`
	Utodok  []struct {
		FelhasznaloID string `json:"felhasznaloId"`
		Name          string `json:"name"`
	} `json:"utodok"`
}

type szervezoErrorResp struct {
	Error     string `json:"error"`
	Code      string `json:"code"`
	Esemeny   *struct{ ID, Title string }
	Esemenyek []struct {
		ID      string `json:"id"`
		Title   string `json:"title"`
		Allapot string `json:"allapot"`
	} `json:"esemenyek"`
}

// szervezoCsoport has Anna (Admin) create a Csoport with Béla and Cecil as
// Tags, and Béla propose an Esemeny that Anna answered (Cecil didn't).
func szervezoCsoport(t *testing.T) (api http.Handler, csoportID, esemenyID string) {
	t.Helper()
	api, _, csoportID = adminCsoport(t)
	e := decode[esemenyResp](t, call(t, api, http.MethodPost, "/csoport/"+csoportID+"/esemeny", proposal, as("bela")))
	d := decode[esemenyDetailResp](t, call(t, api, http.MethodGet, "/esemeny/"+e.ID, "", as("bela")))
	path := "/esemeny/" + e.ID + "/idopont/" + d.Idopontok[0].ID + "/visszajelzes"
	expectStatus(t, call(t, api, http.MethodPut, path, `{"valasz":"igen"}`, as("anna")), http.StatusNoContent)
	return api, csoportID, e.ID
}

func TestHandOverSzervezoRoute(t *testing.T) {
	api, _, id := szervezoCsoport(t)
	path := "/esemeny/" + id + "/szervezo-atadas"

	expectStatus(t, call(t, api, http.MethodPost, path, `{"felhasznaloId":"anna"}`, map[string]string{"X-Internal-Secret": secret}), http.StatusBadRequest)
	expectStatus(t, call(t, api, http.MethodPost, path, `nem json`, as("bela")), http.StatusBadRequest)
	expectStatus(t, call(t, api, http.MethodPost, path, `{"felhasznaloId":"anna"}`, as("cecil")), http.StatusForbidden)
	expectStatus(t, call(t, api, http.MethodPost, "/esemeny/nincs/szervezo-atadas", `{"felhasznaloId":"anna"}`, as("bela")), http.StatusNotFound)

	for _, to := range []string{"cecil", "bela", "nincs", ""} {
		rec := call(t, api, http.MethodPost, path, `{"felhasznaloId":"`+to+`"}`, as("bela"))
		expectStatus(t, rec, http.StatusConflict)
		if e := decode[errorResp](t, rec); e.Code != "not_eligible_szervezo" {
			t.Fatalf("handing to %q: error = %+v, want code not_eligible_szervezo", to, e)
		}
	}

	expectStatus(t, call(t, api, http.MethodPost, path, `{"felhasznaloId":"anna"}`, as("bela")), http.StatusNoContent)
	d := decode[esemenyDetailResp](t, call(t, api, http.MethodGet, "/esemeny/"+id, "", as("cecil")))
	if d.SzervezoID != "anna" || d.SzervezoName != "Anna" {
		t.Fatalf("szervezo = %s (%s), want anna", d.SzervezoID, d.SzervezoName)
	}
	// Béla is out of the role now.
	expectStatus(t, call(t, api, http.MethodPost, path, `{"felhasznaloId":"anna"}`, as("bela")), http.StatusForbidden)
	expectStatus(t, call(t, api, http.MethodPost, "/esemeny/"+id+"/lemondas", "", as("anna")), http.StatusNoContent)
	expectStatus(t, call(t, api, http.MethodPost, path, `{"felhasznaloId":"bela"}`, as("anna")), http.StatusConflict)
}

func TestSzervezoEsemenyekRoute(t *testing.T) {
	api, csoportID, id := szervezoCsoport(t)
	path := "/csoport/" + csoportID + "/tag/bela/szervezo-esemenyek"

	expectStatus(t, call(t, api, http.MethodGet, path, "", map[string]string{"X-Internal-Secret": secret}), http.StatusBadRequest)
	expectStatus(t, call(t, api, http.MethodGet, path, "", as("cecil")), http.StatusForbidden)
	expectStatus(t, call(t, api, http.MethodGet, "/csoport/"+csoportID+"/tag/nincs/szervezo-esemenyek", "", as("anna")), http.StatusNotFound)
	login(t, api, "kivul", "444", "Kívül")
	expectStatus(t, call(t, api, http.MethodGet, path, "", as("kivul")), http.StatusNotFound)

	for _, viewer := range []string{"bela", "anna"} {
		list := decode[[]szervezoEsemenyResp](t, call(t, api, http.MethodGet, path, "", as(viewer)))
		if len(list) != 1 || list[0].ID != id || list[0].Title != "Társasjáték" || list[0].Allapot != "egyeztetes" ||
			len(list[0].Utodok) != 1 || list[0].Utodok[0].FelhasznaloID != "anna" || list[0].Utodok[0].Name != "Anna" {
			t.Fatalf("list as %s = %+v, want the Esemeny with Anna as successor", viewer, list)
		}
	}
	list := decode[[]szervezoEsemenyResp](t, call(t, api, http.MethodGet, "/csoport/"+csoportID+"/tag/cecil/szervezo-esemenyek", "", as("cecil")))
	if list == nil || len(list) != 0 {
		t.Fatalf("list of a non-Szervezo = %+v, want an empty array", list)
	}
}

func TestRemoveTagRouteTakesANewSzervezoForEachEsemeny(t *testing.T) {
	api, csoportID, id := szervezoCsoport(t)
	bela := "/csoport/" + csoportID + "/tag/bela"

	// Without a pick, the leave and the removal name the Esemeny.
	rec := call(t, api, http.MethodDelete, bela, "", as("anna"))
	expectStatus(t, rec, http.StatusConflict)
	if e := decode[szervezoErrorResp](t, rec); e.Code != "szervezo_of_active_esemeny" || len(e.Esemenyek) != 1 || e.Esemenyek[0].ID != id {
		t.Fatalf("error = %+v, want the Esemeny named", e)
	}
	rec = call(t, api, http.MethodPost, "/csoport/"+csoportID+"/kilepes", "", as("bela"))
	expectStatus(t, rec, http.StatusConflict)
	if e := decode[szervezoErrorResp](t, rec); e.Code != "szervezo_of_active_esemeny" || len(e.Esemenyek) != 1 || e.Esemenyek[0].ID != id {
		t.Fatalf("error = %+v, want the Esemeny named", e)
	}

	expectStatus(t, call(t, api, http.MethodDelete, bela, `nem json`, as("anna")), http.StatusBadRequest)
	expectStatus(t, call(t, api, http.MethodDelete, bela, `{"szervezok":{"`+id+`":"anna"}}`, as("cecil")), http.StatusForbidden)
	rec = call(t, api, http.MethodDelete, bela, `{"szervezok":{"`+id+`":"cecil"}}`, as("anna"))
	expectStatus(t, rec, http.StatusConflict)
	if e := decode[errorResp](t, rec); e.Code != "not_eligible_szervezo" {
		t.Fatalf("error = %+v, want code not_eligible_szervezo", e)
	}
	expectStatus(t, call(t, api, http.MethodGet, "/csoport/"+csoportID, "", as("bela")), http.StatusOK)

	expectStatus(t, call(t, api, http.MethodDelete, bela, `{"szervezok":{"`+id+`":"anna"}}`, as("anna")), http.StatusNoContent)
	expectStatus(t, call(t, api, http.MethodGet, "/csoport/"+csoportID, "", as("bela")), http.StatusNotFound)
	if d := decode[esemenyDetailResp](t, call(t, api, http.MethodGet, "/esemeny/"+id, "", as("anna"))); d.SzervezoID != "anna" {
		t.Fatalf("szervezo = %s, want anna", d.SzervezoID)
	}
}

func TestRemoveTagRouteCancelsAnEsemenyNobodyElseAnswered(t *testing.T) {
	api, csoportID, id := szervezoCsoport(t)
	lonely := decode[esemenyResp](t, call(t, api, http.MethodPost, "/csoport/"+csoportID+"/esemeny", proposal, as("bela")))
	bela := "/csoport/" + csoportID + "/tag/bela"

	// Anna answered the first Esemeny, so it still needs a pick.
	rec := call(t, api, http.MethodDelete, bela, `{"szervezok":{}}`, as("anna"))
	expectStatus(t, rec, http.StatusConflict)
	if e := decode[szervezoErrorResp](t, rec); e.Code != "szervezo_of_active_esemeny" || len(e.Esemenyek) != 1 || e.Esemenyek[0].ID != id {
		t.Fatalf("error = %+v, want only the answered Esemeny named", e)
	}
	expectStatus(t, call(t, api, http.MethodGet, "/csoport/"+csoportID, "", as("bela")), http.StatusOK)

	expectStatus(t, call(t, api, http.MethodDelete, bela, `{"szervezok":{"`+id+`":"anna"}}`, as("anna")), http.StatusNoContent)
	if d := decode[esemenyDetailResp](t, call(t, api, http.MethodGet, "/esemeny/"+lonely.ID, "", as("anna"))); d.Allapot != "elmaradt" {
		t.Fatalf("allapot = %s, want elmaradt", d.Allapot)
	}
}

func TestRemoveTagRouteToleratesAnEmptyBody(t *testing.T) {
	api, _, id := adminCsoport(t)
	cecil := "/csoport/" + id + "/tag/cecil"

	// No body, a blank one and an empty object all mean no picks.
	expectStatus(t, call(t, api, http.MethodDelete, cecil, "", as("anna")), http.StatusNoContent)
	expectStatus(t, call(t, api, http.MethodDelete, "/csoport/"+id+"/tag/bela", " ", as("anna")), http.StatusNoContent)
}
