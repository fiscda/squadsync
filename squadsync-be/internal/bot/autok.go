package bot

import (
	"context"
	"fmt"

	"squadsync-be/internal/gateway"
)

// Wording of the passenger's DM when their Auto is gone.
const (
	autoDropOutFormat = "%s autója kiesett, keress másik helyet: **%s**"
	autoDeletedFormat = "%s autója megszűnt, keress másik helyet: **%s**"
)

// autoDMText words the DM to a passenger whose Auto is gone.
func autoDMText(d gateway.AutoDM) string {
	format := autoDropOutFormat
	if d.Deleted {
		format = autoDeletedFormat
	}
	s := fmt.Sprintf(format, escapeMarkdown(d.SoforName), escapeMarkdown(d.Title))
	if d.Link != "" {
		s += fmt.Sprintf(linkFormat, d.Link)
	}
	return s
}

// SendAutoDM implements gateway.DiscordMessenger.
func (m Messenger) SendAutoDM(ctx context.Context, discordID string, d gateway.AutoDM) error {
	return m.SendDM(ctx, discordID, autoDMText(d))
}
