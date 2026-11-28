package bot

import (
	"context"
	"fmt"

	"squadsync-be/internal/gateway"
)

// Wording of the DM to a Tag who became an Esemeny's Szervezo.
const szervezoFormat = "Mostantól te vagy a(z) **%s** szervezője (%s)"

// szervezoDMText words the DM to the new Szervezo of an Esemeny.
func szervezoDMText(d gateway.SzervezoDM) string {
	s := fmt.Sprintf(szervezoFormat, escapeMarkdown(d.Title), escapeMarkdown(d.CsoportName))
	if d.Link != "" {
		s += fmt.Sprintf(linkFormat, d.Link)
	}
	return s
}

// SendSzervezoDM implements gateway.DiscordMessenger.
func (m Messenger) SendSzervezoDM(ctx context.Context, discordID string, d gateway.SzervezoDM) error {
	return m.SendDM(ctx, discordID, szervezoDMText(d))
}
