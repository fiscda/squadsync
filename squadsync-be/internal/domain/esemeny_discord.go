package domain

import "time"

// EsemenyMessageKind tells where an Esemeny's Discord message was posted.
type EsemenyMessageKind string

// The places an Esemeny's Discord message goes.
const (
	// InCsatorna is the post in the Csoport's linked channel.
	InCsatorna EsemenyMessageKind = "csatorna"
	// InDM is an announcement sent by DM, when the Csoport has no Csatorna.
	InDM EsemenyMessageKind = "dm"
)

// EsemenyDiscordMessage is one Discord message that shows an Esemeny and is
// edited in place when it changes.
type EsemenyDiscordMessage struct {
	EsemenyID string
	Kind      EsemenyMessageKind
	ChannelID string
	MessageID string
	CreatedAt time.Time
}
