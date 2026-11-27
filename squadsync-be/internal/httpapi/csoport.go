package httpapi

import (
	"errors"
	"net/http"
	"time"

	"squadsync-be/internal/app"
	"squadsync-be/internal/domain"
)

func (s *server) registerCsoport(mux *http.ServeMux) {
	mux.HandleFunc("POST /csoport", withFelhasznalo(s.createCsoport))
	mux.HandleFunc("GET /csoport", withFelhasznalo(s.listCsoportok))
	mux.HandleFunc("GET /csoport/{id}", withFelhasznalo(s.getCsoport))
	mux.HandleFunc("POST /csoport/{id}/meghivo", withFelhasznalo(s.createCsoportMeghivo))
	mux.HandleFunc("GET /csoport/{id}/meghivo", withFelhasznalo(s.listCsoportMeghivok))
	mux.HandleFunc("DELETE /csoport/{id}/meghivo/{meghivoID}", withFelhasznalo(s.revokeCsoportMeghivo))
	mux.HandleFunc("PUT /csoport/{id}/name", withFelhasznalo(s.renameCsoport))
	mux.HandleFunc("PUT /csoport/{id}/tag/{felhasznaloID}/admin", withFelhasznalo(s.setAdmin))
	mux.HandleFunc("DELETE /csoport/{id}/tag/{felhasznaloID}", withFelhasznalo(s.removeTag))
	mux.HandleFunc("POST /csoport/{id}/kilepes", withFelhasznalo(s.leaveCsoport))
	mux.HandleFunc("PUT /csoport/{id}/nemitas", withFelhasznalo(s.setMuted))
	// Linking a channel happens in Discord (/csatorna); the web only unlinks.
	mux.HandleFunc("DELETE /csoport/{id}/csatorna", withFelhasznalo(s.unlinkCsatorna))

	// Opening a link comes first and needs no Felhasznalo: the join page
	// shows which Csoport the link is for before the visitor has logged in.
	mux.HandleFunc("GET /csoport-meghivo/{token}", s.previewCsoportMeghivo)
	mux.HandleFunc("GET /csoport-meghivo/{token}/tag", withFelhasznalo(s.tagOfCsoportMeghivo))
	mux.HandleFunc("POST /csoport-meghivo/{token}/join", withFelhasznalo(s.joinCsoport))
}

// The wire forms below are kept in sync by hand with the types in
// squadsync-fe/src/lib/backend.ts.

type csoportJSON struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	CreatedAt time.Time `json:"createdAt"`
}

type csoportSummaryJSON struct {
	csoportJSON
	Admin    bool `json:"admin"`
	TagCount int  `json:"tagCount"`
}

type tagJSON struct {
	FelhasznaloID string    `json:"felhasznaloId"`
	Name          string    `json:"name"`
	Admin         bool      `json:"admin"`
	CreatedAt     time.Time `json:"createdAt"`
}

type csoportDetailJSON struct {
	csoportJSON
	// Admin tells whether the caller is an Admin of the Csoport.
	Admin bool `json:"admin"`
	// Muted tells whether the caller muted the DMs of the Csoport.
	Muted bool      `json:"muted"`
	Tags  []tagJSON `json:"tags"`
	// Csatorna is the linked Discord channel, or null.
	Csatorna *csatornaJSON `json:"csatorna"`
}

type csatornaJSON struct {
	GuildID   string `json:"guildId"`
	ChannelID string `json:"channelId"`
	// ChannelName is empty when the bot didn't know the channel's name.
	ChannelName string    `json:"channelName"`
	LinkedAt    time.Time `json:"linkedAt"`
}

type csoportMeghivoJSON struct {
	ID        string    `json:"id"`
	Token     string    `json:"token"`
	CreatedAt time.Time `json:"createdAt"`
}

// meghivoPreviewJSON is all that the holder of a link may learn before
// joining: the name of the Csoport.
type meghivoPreviewJSON struct {
	Name string `json:"name"`
}

func toCsoportJSON(c domain.Csoport) csoportJSON {
	return csoportJSON{ID: c.ID, Name: c.Name, CreatedAt: c.CreatedAt}
}

func toCsoportMeghivoJSON(m domain.CsoportMeghivo) csoportMeghivoJSON {
	return csoportMeghivoJSON{ID: m.ID, Token: m.Token, CreatedAt: m.CreatedAt}
}

func (s *server) createCsoport(w http.ResponseWriter, r *http.Request, felhasznaloID string) {
	var body struct {
		Name string `json:"name"`
	}
	if !decodeJSON(w, r, &body) {
		return
	}
	c, err := s.app.CreateCsoport(r.Context(), felhasznaloID, body.Name)
	if err != nil {
		s.writeCsoportError(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, toCsoportJSON(c))
}

func (s *server) listCsoportok(w http.ResponseWriter, r *http.Request, felhasznaloID string) {
	list, err := s.app.CsoportokOf(r.Context(), felhasznaloID)
	if err != nil {
		s.writeCsoportError(w, r, err)
		return
	}
	out := make([]csoportSummaryJSON, 0, len(list))
	for _, c := range list {
		out = append(out, csoportSummaryJSON{csoportJSON: toCsoportJSON(c.Csoport), Admin: c.Admin, TagCount: c.TagCount})
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *server) getCsoport(w http.ResponseWriter, r *http.Request, felhasznaloID string) {
	d, err := s.app.Csoport(r.Context(), felhasznaloID, r.PathValue("id"))
	if err != nil {
		s.writeCsoportError(w, r, err)
		return
	}
	tags := make([]tagJSON, 0, len(d.Tags))
	for _, t := range d.Tags {
		tags = append(tags, tagJSON{FelhasznaloID: t.FelhasznaloID, Name: t.FelhasznaloName, Admin: t.Admin, CreatedAt: t.CreatedAt})
	}
	out := csoportDetailJSON{csoportJSON: toCsoportJSON(d.Csoport), Admin: d.Admin, Muted: d.Muted, Tags: tags}
	if c := d.Csatorna; c != nil {
		out.Csatorna = &csatornaJSON{GuildID: c.GuildID, ChannelID: c.ChannelID, ChannelName: c.ChannelName, LinkedAt: c.LinkedAt}
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *server) renameCsoport(w http.ResponseWriter, r *http.Request, felhasznaloID string) {
	var body struct {
		Name string `json:"name"`
	}
	if !decodeJSON(w, r, &body) {
		return
	}
	c, err := s.app.RenameCsoport(r.Context(), felhasznaloID, r.PathValue("id"), body.Name)
	if err != nil {
		s.writeCsoportError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, toCsoportJSON(c))
}

func (s *server) setAdmin(w http.ResponseWriter, r *http.Request, felhasznaloID string) {
	var body struct {
		Admin bool `json:"admin"`
	}
	if !decodeJSON(w, r, &body) {
		return
	}
	err := s.app.SetAdmin(r.Context(), felhasznaloID, r.PathValue("id"), r.PathValue("felhasznaloID"), body.Admin)
	if err != nil {
		s.writeCsoportError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *server) removeTag(w http.ResponseWriter, r *http.Request, felhasznaloID string) {
	err := s.app.RemoveTag(r.Context(), felhasznaloID, r.PathValue("id"), r.PathValue("felhasznaloID"))
	if err != nil {
		s.writeCsoportError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *server) leaveCsoport(w http.ResponseWriter, r *http.Request, felhasznaloID string) {
	if err := s.app.LeaveCsoport(r.Context(), felhasznaloID, r.PathValue("id")); err != nil {
		s.writeCsoportError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *server) setMuted(w http.ResponseWriter, r *http.Request, felhasznaloID string) {
	var body struct {
		Muted bool `json:"muted"`
	}
	if !decodeJSON(w, r, &body) {
		return
	}
	if err := s.app.SetMuted(r.Context(), felhasznaloID, r.PathValue("id"), body.Muted); err != nil {
		s.writeCsoportError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *server) unlinkCsatorna(w http.ResponseWriter, r *http.Request, felhasznaloID string) {
	if err := s.app.UnlinkCsatorna(r.Context(), felhasznaloID, r.PathValue("id")); err != nil {
		s.writeCsoportError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *server) createCsoportMeghivo(w http.ResponseWriter, r *http.Request, felhasznaloID string) {
	m, err := s.app.CreateCsoportMeghivo(r.Context(), felhasznaloID, r.PathValue("id"))
	if err != nil {
		s.writeCsoportError(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, toCsoportMeghivoJSON(m))
}

func (s *server) listCsoportMeghivok(w http.ResponseWriter, r *http.Request, felhasznaloID string) {
	list, err := s.app.CsoportMeghivok(r.Context(), felhasznaloID, r.PathValue("id"))
	if err != nil {
		s.writeCsoportError(w, r, err)
		return
	}
	out := make([]csoportMeghivoJSON, 0, len(list))
	for _, m := range list {
		out = append(out, toCsoportMeghivoJSON(m))
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *server) revokeCsoportMeghivo(w http.ResponseWriter, r *http.Request, felhasznaloID string) {
	err := s.app.RevokeCsoportMeghivo(r.Context(), felhasznaloID, r.PathValue("id"), r.PathValue("meghivoID"))
	if err != nil {
		s.writeCsoportError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *server) previewCsoportMeghivo(w http.ResponseWriter, r *http.Request) {
	c, err := s.app.CsoportMeghivoPreview(r.Context(), r.PathValue("token"))
	if err != nil {
		s.writeCsoportError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, meghivoPreviewJSON{Name: c.Name})
}

func (s *server) tagOfCsoportMeghivo(w http.ResponseWriter, r *http.Request, felhasznaloID string) {
	id, err := s.app.TagCsoportOfMeghivo(r.Context(), felhasznaloID, r.PathValue("token"))
	if err != nil {
		s.writeCsoportError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, struct {
		CsoportID string `json:"csoportId"`
	}{id})
}

func (s *server) joinCsoport(w http.ResponseWriter, r *http.Request, felhasznaloID string) {
	res, err := s.app.JoinCsoport(r.Context(), felhasznaloID, r.PathValue("token"))
	if err != nil {
		s.writeCsoportError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, struct {
		csoportJSON
		Joined bool `json:"joined"`
	}{toCsoportJSON(res.Csoport), res.Joined})
}

func (s *server) writeCsoportError(w http.ResponseWriter, r *http.Request, err error) {
	if errors.Is(err, app.ErrCsoportMeghivoRevoked) {
		writeError(w, http.StatusGone, err.Error())
		return
	}
	// The code lets the web tell the two 409s apart.
	if errors.Is(err, app.ErrLastAdmin) {
		writeJSON(w, http.StatusConflict, map[string]string{"error": err.Error(), "code": "last_admin"})
		return
	}
	if errors.Is(err, app.ErrSzervezoOfActiveEsemeny) {
		writeJSON(w, http.StatusConflict, map[string]string{"error": err.Error(), "code": "szervezo_of_active_esemeny"})
		return
	}
	if errors.Is(err, app.ErrFelhasznaloUnknown) || errors.Is(err, app.ErrNotAdmin) {
		writeError(w, http.StatusForbidden, err.Error())
		return
	}
	s.writeAppError(w, r, err)
}
