package bot

import (
	"testing"

	"squadsync-be/internal/gateway"
)

func TestSzallasDMTextNamesTheSzobaAndTheEsemeny(t *testing.T) {
	d := gateway.SzallasDM{SzobaName: "Eme*let", Title: "Balaton", Link: "https://x.example/esemenyek/1"}
	got := szallasDMText(d)
	want := `A(z) Eme\*let szoba megszűnt, válassz új ágyat: **Balaton**` + "\nhttps://x.example/esemenyek/1"
	if got != want {
		t.Fatalf("text = %q, want %q", got, want)
	}
}
