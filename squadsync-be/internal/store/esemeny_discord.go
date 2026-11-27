package store

import (
	"context"
	"fmt"

	"squadsync-be/internal/domain"
)

// InsertEsemenyDiscordMessage remembers a Discord message that shows an
// Esemeny.
func (q Queries) InsertEsemenyDiscordMessage(ctx context.Context, m domain.EsemenyDiscordMessage) error {
	_, err := q.db.ExecContext(ctx,
		`INSERT INTO esemeny_discord_message (esemeny_id, kind, channel_id, message_id, created_at)
		 VALUES (?, ?, ?, ?, ?)`,
		m.EsemenyID, string(m.Kind), m.ChannelID, m.MessageID, formatTime(m.CreatedAt),
	)
	if err != nil {
		return fmt.Errorf("insert esemeny discord message: %w", err)
	}
	return nil
}

// ListEsemenyDiscordMessages returns every Discord message that shows the
// Esemeny, the channel post first, then oldest first.
func (q Queries) ListEsemenyDiscordMessages(ctx context.Context, esemenyID string) ([]domain.EsemenyDiscordMessage, error) {
	rows, err := q.db.QueryContext(ctx,
		`SELECT esemeny_id, kind, channel_id, message_id, created_at FROM esemeny_discord_message
		 WHERE esemeny_id = ? ORDER BY kind = 'dm', created_at, channel_id, message_id`,
		esemenyID,
	)
	if err != nil {
		return nil, fmt.Errorf("list esemeny discord messages: %w", err)
	}
	defer rows.Close()

	var list []domain.EsemenyDiscordMessage
	for rows.Next() {
		var m domain.EsemenyDiscordMessage
		var kind, createdAt string
		if err := rows.Scan(&m.EsemenyID, &kind, &m.ChannelID, &m.MessageID, &createdAt); err != nil {
			return nil, fmt.Errorf("list esemeny discord messages: %w", err)
		}
		m.Kind = domain.EsemenyMessageKind(kind)
		if m.CreatedAt, err = parseTime(createdAt); err != nil {
			return nil, err
		}
		list = append(list, m)
	}
	return list, rows.Err()
}
