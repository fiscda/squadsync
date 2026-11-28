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
	// VendegID is a name-only Vendeg's id (#13), "" otherwise.
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

func (s *server) writeKoltsegekError(w http.ResponseWriter, r *http.Request, err error) {
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
