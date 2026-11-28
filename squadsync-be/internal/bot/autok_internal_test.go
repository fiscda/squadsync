package bot

import (
	"strings"
	"testing"

	"squadsync-be/internal/gateway"
)

func TestAutoDMTextNamesTheSoforAndTheEsemeny(t *testing.T) {
	d := gateway.AutoDM{SoforName: "An*na", Title: "Balaton", Link: "https://x.example/esemenyek/1"}
	got := autoDMText(d)
	want := `An\*na autója kiesett, keress másik helyet: **Balaton**` + "\nhttps://x.example/esemenyek/1"
	if got != want {
		t.Fatalf("text = %q, want %q", got, want)
	}
	d.Deleted = true
	if got := autoDMText(d); !strings.Contains(got, "autója megszűnt, keress másik helyet") {
		t.Fatalf("deleted text = %q", got)
	}
}
