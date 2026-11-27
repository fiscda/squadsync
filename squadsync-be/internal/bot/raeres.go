package bot

import (
	"fmt"

	"squadsync-be/internal/gateway"
)

// szabadAznap shows how many Tags are free on the days an Idopont covers.
func szabadAznap(i gateway.IdopontLine) string {
	if i.NincsRaeres {
		return "🗓 szabad aznap: –"
	}
	return fmt.Sprintf("🗓 %d/%d szabad aznap", i.Szabad, i.Osszes)
}
