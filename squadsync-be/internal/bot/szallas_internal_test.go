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

func TestSzervezoDMTextNamesTheEsemenyAndTheCsoport(t *testing.T) {
	d := gateway.SzervezoDM{CsoportName: "Péntek_esték", Title: "Bal*aton", Link: "http://localhost:3000/esemenyek/1"}
	got := szervezoDMText(d)
	want := `Mostantól te vagy a(z) **Bal\*aton** szervezője (Péntek\_esték)` + "\nhttp://localhost:3000/esemenyek/1"
	if got != want {
		t.Fatalf("text = %q, want %q", got, want)
	}
}
