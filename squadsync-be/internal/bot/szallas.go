package bot

import (
	"context"
	"fmt"

	"squadsync-be/internal/gateway"
)

// Wording of the sleeper's DM when their Szoba is gone.
const szobaDeletedFormat = "A(z) %s szoba megszűnt, válassz új ágyat: **%s**"

// szallasDMText words the DM to a sleeper whose Szoba was deleted.
func szallasDMText(d gateway.SzallasDM) string {
	s := fmt.Sprintf(szobaDeletedFormat, escapeMarkdown(d.SzobaName), escapeMarkdown(d.Title))
	if d.Link != "" {
		s += fmt.Sprintf(linkFormat, d.Link)
	}
	return s
}

// SendSzallasDM implements gateway.DiscordMessenger.
func (m Messenger) SendSzallasDM(ctx context.Context, discordID string, d gateway.SzallasDM) error {
	return m.SendDM(ctx, discordID, szallasDMText(d))
}
