package bot

import (
	"fmt"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/bwmarrin/discordgo"

	"squadsync-be/internal/gateway"
)

// hataridoDM builds a Hatarido DM with n Idopontok, 32-character hex ids
// like the core's, and a label too long for a button.
func hataridoDM(n int, link string) gateway.HataridoDM {
	h := gateway.HataridoDM{
		EsemenyID:      strings.Repeat("e", 32),
		CsoportName:    "Péntek_esték",
		Title:          "Kör *1*",
		ConfirmButtons: n <= gateway.MaxAnswerRows,
		Link:           link,
	}
	for i := range n {
		h.Idopontok = append(h.Idopontok, gateway.IdopontLine{
			ID:       fmt.Sprintf("%032x", i+1),
			Label:    "dec. 5. (szombat) 20:00 – " + strings.Repeat("hosszú ", 15),
			Helyezes: i + 1,
			Igen:     5 - i,
		})
	}
	return h
}

func TestHataridoDMWithFiveIdopontokHasAConfirmButtonRowEach(t *testing.T) {
	h := hataridoDM(5, "http://localhost:3000/esemenyek/x")
	content, components, err := renderHatarido(h)
	if err != nil {
		t.Fatalf("renderHatarido: %v", err)
	}
	if !strings.Contains(content, `**Kör \*1\***`) || !strings.Contains(content, `(Péntek\_esték)`) {
		t.Fatalf("content doesn't escape the title and csoport: %q", content)
	}
	if !strings.Contains(content, "\n1 · dec. 5.") || !strings.Contains(content, "🏆 1. hely · ✅ 5 igen") {
		t.Fatalf("content doesn't rank the idopontok: %q", content)
	}
	if !strings.HasSuffix(content, "\nhttp://localhost:3000/esemenyek/x") {
		t.Fatalf("content lacks the web link as text: %q", content)
	}
	if len(components) != 5 {
		t.Fatalf("rows = %d, want 5", len(components))
	}
	for n, row := range components {
		buttons := row.(discordgo.ActionsRow).Components
		if len(buttons) != 1 {
			t.Fatalf("row %d has %d buttons, want 1", n, len(buttons))
		}
		b := buttons[0].(discordgo.Button)
		want := "vegl:" + h.EsemenyID + ":" + h.Idopontok[n].ID
		if b.CustomID != want || len(b.CustomID) > 100 {
			t.Fatalf("row %d custom id = %q (%d chars), want %q", n, b.CustomID, len(b.CustomID), want)
		}
		if utf8.RuneCountInString(b.Label) > 80 || !strings.HasSuffix(b.Label, "…") ||
			!strings.HasPrefix(b.Label, fmt.Sprintf("Véglegesítés: %d · dec. 5.", n+1)) {
			t.Fatalf("row %d label = %q, want it truncated to 80 characters", n, b.Label)
		}
	}
}

func TestHataridoDMWithSixIdopontokLinksTheWebInstead(t *testing.T) {
	content, components, err := renderHatarido(hataridoDM(6, "https://squadsync.example/esemenyek/x"))
	if err != nil {
		t.Fatalf("renderHatarido: %v", err)
	}
	if len(components) != 0 || components == nil {
		t.Fatalf("components = %v, want none (and not nil, so an edit removes buttons)", components)
	}
	if !strings.Contains(content, "\n6 · ") || !strings.HasSuffix(content, "véglegesíteni a weboldalon tudsz: https://squadsync.example/esemenyek/x") {
		t.Fatalf("content = %q, want all six and the web link", content)
	}

	content, _, _ = renderHatarido(hataridoDM(6, ""))
	if strings.Contains(content, "tudsz:") || !strings.HasSuffix(content, "a weboldalon tudsz.") {
		t.Fatalf("content without a link = %q, want no dangling colon", content)
	}
}
