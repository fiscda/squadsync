package bot

import (
	"fmt"
	"strings"
	"testing"

	"github.com/bwmarrin/discordgo"

	"squadsync-be/internal/gateway"
)

func elszamolasDM(fizetendo, kapando int) gateway.ElszamolasDM {
	d := gateway.ElszamolasDM{
		EsemenyID:   strings.Repeat("e", 32),
		CsoportName: "Péntek_esték",
		Title:       "Kör *1*",
		Link:        "http://localhost:3000/esemenyek/x",
	}
	for i := range fizetendo {
		d.Fizetendo = append(d.Fizetendo, gateway.ElszamolasUtalas{UtalasID: fmt.Sprintf("%032x", i+1), Name: "Anna_" + fmt.Sprint(i), Amount: 12000 + i})
	}
	for i := range kapando {
		d.Kapando = append(d.Kapando, gateway.ElszamolasUtalas{UtalasID: fmt.Sprintf("%032x", 100+i), Name: "Béla", Amount: 1234567})
	}
	return d
}

func TestElszamolasDMListsWhatYouPayWithAMegvoltButtonEach(t *testing.T) {
	d := elszamolasDM(7, 1)
	content, components, err := renderElszamolas(d)
	if err != nil {
		t.Fatalf("renderElszamolas: %v", err)
	}
	for _, want := range []string{`**Kör \*1\*** (Péntek\_esték)`, "Ezt kell utalnod:\n• Anna\\_0: 12 000 Ft", "Ezt kapod:\n• Béla: 1 234 567 Ft", "\nhttp://localhost:3000/esemenyek/x"} {
		if !strings.Contains(content, want) {
			t.Fatalf("content = %q, want it to contain %q", content, want)
		}
	}
	if len(components) != 2 {
		t.Fatalf("rows = %d, want 2 (5 + 2 buttons)", len(components))
	}
	first := components[0].(discordgo.ActionsRow).Components
	if len(first) != 5 || len(components[1].(discordgo.ActionsRow).Components) != 2 {
		t.Fatalf("buttons per row = %d, %d", len(first), len(components[1].(discordgo.ActionsRow).Components))
	}
	b := first[0].(discordgo.Button)
	if b.CustomID != "megvolt:"+d.EsemenyID+":"+d.Fizetendo[0].UtalasID || len(b.CustomID) > 100 || b.Label != "Megvolt: Anna_0, 12 000 Ft" {
		t.Fatalf("button = %+v", b)
	}
}

func TestElszamolasDMWhenSquareOrWithTooManyUtalasok(t *testing.T) {
	content, components, err := renderElszamolas(elszamolasDM(0, 0))
	if err != nil {
		t.Fatalf("renderElszamolas: %v", err)
	}
	if !strings.Contains(content, "Rendben vagy") || len(components) != 0 || components == nil {
		t.Fatalf("square DM = %q, %v", content, components)
	}
	content, components, _ = renderElszamolas(elszamolasDM(gateway.MaxMegvoltButtons+1, 0))
	if len(components) != 0 || !strings.Contains(content, "a weboldalon jelölheted") {
		t.Fatalf("DM with too many = %q, %d rows", content, len(components))
	}
}
