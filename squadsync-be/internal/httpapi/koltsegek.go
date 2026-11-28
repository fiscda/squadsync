package httpapi

import (
	"errors"
	"net/http"
	"time"

	"squadsync-be/internal/app"
	"squadsync-be/internal/domain"
)

func (s *server) registerKoltsegek(mux *http.ServeMux) {
	mux.HandleFunc("GET /esemeny/{id}/koltsegek", withFelhasznalo(s.getKoltsegek))
	mux.HandleFunc("POST /esemeny/{id}/koltsegek/kiadas", withFelhasznalo(s.addKiadas))
	mux.HandleFunc("PUT /esemeny/{id}/koltsegek/kiadas/{kiadasID}", withFelhasznalo(s.updateKiadas))
	mux.HandleFunc("DELETE /esemeny/{id}/koltsegek/kiadas/{kiadasID}", withFelhasznalo(s.deleteKiadas))
	mux.HandleFunc("POST /esemeny/{id}/koltsegek/elszamolas/megerosites", withFelhasznalo(s.confirmKiadasokRecorded))
	mux.HandleFunc("DELETE /esemeny/{id}/koltsegek/elszamolas/megerosites", withFelhasznalo(s.unconfirmKiadasokRecorded))
	mux.HandleFunc("POST /esemeny/{id}/koltsegek/elszamolas/lezaras", withFelhasznalo(s.closeElszamolas))
	mux.HandleFunc("POST /esemeny/{id}/koltsegek/elszamolas/ujranyitas", withFelhasznalo(s.reopenElszamolas))
	mux.HandleFunc("PUT /esemeny/{id}/koltsegek/elszamolas/utalas/{utalasID}/megvolt", withFelhasznalo(s.setUtalasMegvolt))
}

// The wire forms below are kept in sync by hand with
// squadsync-fe/src/lib/kiegeszito/koltsegek.ts.

type kiadasJSON struct {
	ID          string    `json:"id"`
	Description string    `json:"description"`
	Amount      int       `json:"amount"`
	PayerID     string    `json:"payerId"`
	PayerName   string    `json:"payerName"`
	Allapot     string    `json:"allapot"`
	Egyenlo     bool      `json:"egyenlo"`
	CreatedBy   string    `json:"createdBy"`
	CreatedAt   time.Time `json:"createdAt"`
	UpdatedAt   time.Time `json:"updatedAt"`
}

type kiadasReszJSON struct {
	FelhasznaloID string `json:"felhasznaloId"`
	// VendegID is a name-only Vendeg's id, "" otherwise.
	VendegID string `json:"vendegId"`
	Name     string `json:"name"`
	Amount   int    `json:"amount"`
}

type kiadasAllasJSON struct {
	kiadasJSON
	Reszek  []kiadasReszJSON `json:"reszek"`
	CanEdit bool             `json:"canEdit"`
}

type egyenlegJSON struct {
	FelhasznaloID string `json:"felhasznaloId"`
	Name          string `json:"name"`
	Fizetett      int    `json:"fizetett"`
	Resz          int    `json:"resz"`
	Netto         int    `json:"netto"`
}

type koltsegekJSON struct {
	Kiadasok   []kiadasAllasJSON `json:"kiadasok"`
	Egyenlegek []egyenlegJSON    `json:"egyenlegek"`
	Tervezett  int               `json:"tervezett"`
	CanAdd     bool              `json:"canAdd"`
	// Elszamolas is null while the Esemeny has none.
	Elszamolas *elszamolasJSON `json:"elszamolas"`
}

type elszamolasSzemelyJSON struct {
	FelhasznaloID string `json:"felhasznaloId"`
	// VendegID is a name-only Vendeg's id, "" otherwise.
	VendegID string `json:"vendegId"`
	Name     string `json:"name"`
}

type utalasJSON struct {
	ID       string `json:"id"`
	FromID   string `json:"fromId"`
	FromName string `json:"fromName"`
	ToID     string `json:"toId"`
	ToName   string `json:"toName"`
	Amount   int    `json:"amount"`
	// MegvoltAt is null while it isn't megvolt.
	MegvoltAt     *time.Time `json:"megvoltAt"`
	MegvoltByName string     `json:"megvoltByName"`
	CanMark       bool       `json:"canMark"`
}

type elszamolasJSON struct {
	// Allapot is lezajlas_elott, megerosites, kiszamolva or elszamolva.
	Allapot         string                  `json:"allapot"`
	Megerositettek  []elszamolasSzemelyJSON `json:"megerositettek"`
	Hianyzok        []elszamolasSzemelyJSON `json:"hianyzok"`
	Megerositette   bool                    `json:"megerositette"`
	CanConfirm      bool                    `json:"canConfirm"`
	TervezettFizeto []string                `json:"tervezettFizeto"`
	CanUnconfirm    bool                    `json:"canUnconfirm"`
	CanClose        bool                    `json:"canClose"`
	CanReopen       bool                    `json:"canReopen"`
	// KiszamolvaAt is null before the calculation.
	KiszamolvaAt *time.Time   `json:"kiszamolvaAt"`
	LezartaName  string       `json:"lezartaName"`
	Utalasok     []utalasJSON `json:"utalasok"`
}

func toElszamolasJSON(e *app.Elszamolas) *elszamolasJSON {
	if e == nil {
		return nil
	}
	szemelyek := func(list []app.ElszamolasSzemely) []elszamolasSzemelyJSON {
		out := make([]elszamolasSzemelyJSON, 0, len(list))
		for _, p := range list {
			out = append(out, elszamolasSzemelyJSON{FelhasznaloID: p.Holder.FelhasznaloID, VendegID: p.Holder.VendegID, Name: p.Name})
		}
		return out
	}
	out := &elszamolasJSON{
		Allapot:         string(e.Allapot),
		Megerositettek:  szemelyek(e.Megerositettek),
		Hianyzok:        szemelyek(e.Hianyzok),
		Megerositette:   e.Megerositette,
		CanConfirm:      e.CanConfirm,
		TervezettFizeto: append([]string{}, e.TervezettFizeto...),
		CanUnconfirm:    e.CanUnconfirm,
		CanClose:        e.CanClose,
		CanReopen:       e.CanReopen,
		KiszamolvaAt:    e.KiszamolvaAt,
		LezartaName:     e.LezartaName,
		Utalasok:        make([]utalasJSON, 0, len(e.Utalasok)),
	}
	for _, u := range e.Utalasok {
		out.Utalasok = append(out.Utalasok, utalasJSON{
			ID: u.ID, FromID: u.FromID, FromName: u.FromName, ToID: u.ToID, ToName: u.ToName, Amount: u.Amount,
			MegvoltAt: u.MegvoltAt, MegvoltByName: u.MegvoltByName, CanMark: u.CanMark,
		})
	}
	return out
}

type kiadasInputJSON struct {
	// Description "" keeps the current one (editing only).
	Description string `json:"description"`
	Amount      int    `json:"amount"`
	// PayerID "" means the caller (adding) or the current payer (editing).
	PayerID string `json:"payerId"`
	// Allapot "" means kifizetve (adding) or the current one (editing).
	Allapot string `json:"allapot"`
	// Split null means every Resztvevo equally (adding) or the current
	// split (editing).
	Split *struct {
		Egyenlo bool `json:"egyenlo"`
		// Reszek lists who it is split among; amount is read only when
		// egyenlo is false.
		Reszek []struct {
			FelhasznaloID string `json:"felhasznaloId"`
			VendegID      string `json:"vendegId"`
			Amount        int    `json:"amount"`
		} `json:"reszek"`
	} `json:"split"`
}

func (in kiadasInputJSON) input() app.KiadasInput {
	out := app.KiadasInput{
		Description: in.Description,
		Amount:      in.Amount,
		PayerID:     in.PayerID,
		Allapot:     domain.KiadasAllapot(in.Allapot),
	}
	if in.Split != nil {
		out.Split = &app.SplitInput{Holders: []domain.ClaimHolder{}}
		if !in.Split.Egyenlo {
			out.Split.Custom = []int{}
		}
		for _, r := range in.Split.Reszek {
			out.Split.Holders = append(out.Split.Holders, domain.ClaimHolder{FelhasznaloID: r.FelhasznaloID, VendegID: r.VendegID})
			if !in.Split.Egyenlo {
				out.Split.Custom = append(out.Split.Custom, r.Amount)
			}
		}
	}
	return out
}

func toKiadasJSON(k domain.Kiadas) kiadasJSON {
	return kiadasJSON{
		ID: k.ID, Description: k.Description, Amount: k.Amount, PayerID: k.PayerID, PayerName: k.PayerName,
		Allapot: string(k.Allapot), Egyenlo: k.Egyenlo, CreatedBy: k.CreatedBy, CreatedAt: k.CreatedAt, UpdatedAt: k.UpdatedAt,
	}
}

func (s *server) getKoltsegek(w http.ResponseWriter, r *http.Request, felhasznaloID string) {
	k, err := s.app.Koltsegek(r.Context(), felhasznaloID, r.PathValue("id"))
	if err != nil {
		s.writeKoltsegekError(w, r, err)
		return
	}
	out := koltsegekJSON{
		Kiadasok:   make([]kiadasAllasJSON, 0, len(k.Kiadasok)),
		Egyenlegek: make([]egyenlegJSON, 0, len(k.Egyenlegek)),
		Tervezett:  k.Tervezett,
		CanAdd:     k.CanAdd,
		Elszamolas: toElszamolasJSON(k.Elszamolas),
	}
	for _, kiadas := range k.Kiadasok {
		allas := kiadasAllasJSON{kiadasJSON: toKiadasJSON(kiadas.Kiadas), Reszek: make([]kiadasReszJSON, 0, len(kiadas.Reszek)), CanEdit: kiadas.CanEdit}
		for _, resz := range kiadas.Reszek {
			allas.Reszek = append(allas.Reszek, kiadasReszJSON{
				FelhasznaloID: resz.Holder.FelhasznaloID, VendegID: resz.Holder.VendegID, Name: resz.Name, Amount: resz.Amount,
			})
		}
		out.Kiadasok = append(out.Kiadasok, allas)
	}
	for _, e := range k.Egyenlegek {
		out.Egyenlegek = append(out.Egyenlegek, egyenlegJSON{
			FelhasznaloID: e.FelhasznaloID, Name: e.Name, Fizetett: e.Fizetett, Resz: e.Resz, Netto: e.Netto,
		})
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *server) addKiadas(w http.ResponseWriter, r *http.Request, felhasznaloID string) {
	var body kiadasInputJSON
	if !decodeJSON(w, r, &body) {
		return
	}
	k, err := s.app.AddKiadas(r.Context(), felhasznaloID, r.PathValue("id"), body.input())
	if err != nil {
		s.writeKoltsegekError(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, toKiadasJSON(k))
}

func (s *server) updateKiadas(w http.ResponseWriter, r *http.Request, felhasznaloID string) {
	var body kiadasInputJSON
	if !decodeJSON(w, r, &body) {
		return
	}
	if err := s.app.UpdateKiadas(r.Context(), felhasznaloID, r.PathValue("id"), r.PathValue("kiadasID"), body.input()); err != nil {
		s.writeKoltsegekError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *server) deleteKiadas(w http.ResponseWriter, r *http.Request, felhasznaloID string) {
	if err := s.app.DeleteKiadas(r.Context(), felhasznaloID, r.PathValue("id"), r.PathValue("kiadasID")); err != nil {
		s.writeKoltsegekError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *server) confirmKiadasokRecorded(w http.ResponseWriter, r *http.Request, felhasznaloID string) {
	s.koltsegekNoContent(w, r, s.app.ConfirmKiadasokRecorded(r.Context(), felhasznaloID, r.PathValue("id")))
}

func (s *server) unconfirmKiadasokRecorded(w http.ResponseWriter, r *http.Request, felhasznaloID string) {
	s.koltsegekNoContent(w, r, s.app.UnconfirmKiadasokRecorded(r.Context(), felhasznaloID, r.PathValue("id")))
}

func (s *server) closeElszamolas(w http.ResponseWriter, r *http.Request, felhasznaloID string) {
	s.koltsegekNoContent(w, r, s.app.CloseElszamolas(r.Context(), felhasznaloID, r.PathValue("id")))
}

func (s *server) reopenElszamolas(w http.ResponseWriter, r *http.Request, felhasznaloID string) {
	s.koltsegekNoContent(w, r, s.app.ReopenElszamolas(r.Context(), felhasznaloID, r.PathValue("id")))
}

func (s *server) setUtalasMegvolt(w http.ResponseWriter, r *http.Request, felhasznaloID string) {
	var body struct {
		Megvolt bool `json:"megvolt"`
	}
	if !decodeJSON(w, r, &body) {
		return
	}
	s.koltsegekNoContent(w, r, s.app.SetUtalasMegvolt(r.Context(), felhasznaloID, r.PathValue("id"), r.PathValue("utalasID"), body.Megvolt))
}

// koltsegekNoContent answers 204, or the Koltsegek error.
func (s *server) koltsegekNoContent(w http.ResponseWriter, r *http.Request, err error) {
	if err != nil {
		s.writeKoltsegekError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *server) writeKoltsegekError(w http.ResponseWriter, r *http.Request, err error) {
	for _, c := range []struct {
		err    error
		status int
		code   string
	}{
		{app.ErrElszamolasNotOpen, http.StatusConflict, "koltsegek_elszamolas_not_open"},
		{app.ErrElszamolasTervezett, http.StatusConflict, "koltsegek_elszamolas_tervezett"},
		{app.ErrElszamolasNotResztvevo, http.StatusForbidden, "koltsegek_elszamolas_not_resztvevo"},
		{app.ErrElszamolasKiszamolva, http.StatusConflict, "koltsegek_elszamolas_kiszamolva"},
		{app.ErrElszamolasNotKiszamolva, http.StatusConflict, "koltsegek_elszamolas_not_kiszamolva"},
		{app.ErrElszamolasMegvolt, http.StatusConflict, "koltsegek_elszamolas_megvolt"},
		{app.ErrElszamolva, http.StatusConflict, "koltsegek_elszamolva"},
		{app.ErrNotSzervezo, http.StatusForbidden, "koltsegek_not_szervezo"},
	} {
		if errors.Is(err, c.err) {
			writeCodedError(w, c.status, err, c.code)
			return
		}
	}
	switch {
	case errors.Is(err, app.ErrKoltsegekClosed):
		writeCodedError(w, http.StatusConflict, err, "koltsegek_closed")
	case errors.Is(err, app.ErrKiadasokLocked):
		writeCodedError(w, http.StatusConflict, err, "koltsegek_locked")
	case errors.Is(err, app.ErrKiadasNotResztvevo):
		writeCodedError(w, http.StatusForbidden, err, "koltsegek_not_resztvevo")
	case errors.Is(err, app.ErrKiadasSplitSum):
		// 409 rather than 400, so the web shows its own notice.
		writeCodedError(w, http.StatusConflict, err, "koltsegek_split_sum")
	default:
		s.writeKiegeszitoError(w, r, err)
	}
}
