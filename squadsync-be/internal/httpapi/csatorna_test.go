package httpapi_test

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"testing"

	"squadsync-be/internal/app"
	"squadsync-be/internal/apptest"
	"squadsync-be/internal/httpapi"
)

type csatornaResp struct {
	Csatorna *struct {
		GuildID     string `json:"guildId"`
		ChannelID   string `json:"channelId"`
		ChannelName string `json:"channelName"`
	} `json:"csatorna"`
}

func TestCsatornaIsShownAndOnlyAdminsUnlinkIt(t *testing.T) {
	h := apptest.New(t)
	api := httpapi.New(h.App, secret, slog.New(slog.NewTextHandler(io.Discard, nil)))
	login(t, api, "anna", "111", "Anna")
	login(t, api, "bela", "222", "Béla")
	login(t, api, "cecil", "333", "Cecil")
	created := decode[csoportResp](t, call(t, api, http.MethodPost, "/csoport", `{"name":"Péntek esték"}`, as("anna")))
	m := decode[meghivoResp](t, call(t, api, http.MethodPost, "/csoport/"+created.ID+"/meghivo", "", as("anna")))
	expectStatus(t, call(t, api, http.MethodPost, "/csoport-meghivo/"+m.Token+"/join", "", as("bela")), http.StatusOK)

	// No channel yet: the field is null.
	if d := decode[csatornaResp](t, call(t, api, http.MethodGet, "/csoport/"+created.ID, "", as("bela"))); d.Csatorna != nil {
		t.Fatalf("csatorna = %+v, want null", d.Csatorna)
	}

	// Linking happens in Discord, through the bot.
	if _, err := h.App.LinkCsatorna(context.Background(), "anna", app.LinkCsatornaInput{
		CsoportID: created.ID, GuildID: "g1", ChannelID: "c1", ChannelName: "altalanos",
	}); err != nil {
		t.Fatalf("LinkCsatorna: %v", err)
	}
	d := decode[csatornaResp](t, call(t, api, http.MethodGet, "/csoport/"+created.ID, "", as("bela")))
	if d.Csatorna == nil || d.Csatorna.GuildID != "g1" || d.Csatorna.ChannelID != "c1" || d.Csatorna.ChannelName != "altalanos" {
		t.Fatalf("csatorna = %+v, want g1/c1 altalanos", d.Csatorna)
	}

	path := "/csoport/" + created.ID + "/csatorna"
	expectStatus(t, call(t, api, http.MethodDelete, path, "", map[string]string{httpapi.HeaderSecret: secret}), http.StatusBadRequest)
	expectStatus(t, call(t, api, http.MethodDelete, path, "", as("bela")), http.StatusForbidden)
	expectStatus(t, call(t, api, http.MethodDelete, path, "", as("cecil")), http.StatusNotFound)
	expectStatus(t, call(t, api, http.MethodDelete, path, "", as("anna")), http.StatusNoContent)

	if d := decode[csatornaResp](t, call(t, api, http.MethodGet, "/csoport/"+created.ID, "", as("anna"))); d.Csatorna != nil {
		t.Fatalf("after unlinking csatorna = %+v, want null", d.Csatorna)
	}
}
