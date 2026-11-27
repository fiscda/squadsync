package store

import (
	"context"
	"fmt"

	"squadsync-be/internal/domain"
)

// UpsertCsatorna links the channel to its Csoport, replacing any channel
// linked before.
func (q Queries) UpsertCsatorna(ctx context.Context, c domain.Csatorna) error {
	_, err := q.db.ExecContext(ctx,
		`INSERT INTO csatorna (csoport_id, guild_id, channel_id, channel_name, linked_by, linked_at)
		 VALUES (?, ?, ?, ?, ?, ?)
		 ON CONFLICT (csoport_id) DO UPDATE SET
		     guild_id = excluded.guild_id,
		     channel_id = excluded.channel_id,
		     channel_name = excluded.channel_name,
		     linked_by = excluded.linked_by,
		     linked_at = excluded.linked_at`,
		c.CsoportID, c.GuildID, c.ChannelID, c.ChannelName, c.LinkedBy, formatTime(c.LinkedAt),
	)
	if err != nil {
		return fmt.Errorf("upsert csatorna: %w", err)
	}
	return nil
}

// FindCsatorna returns the channel linked to the Csoport, or ErrNotFound.
func (q Queries) FindCsatorna(ctx context.Context, csoportID string) (domain.Csatorna, error) {
	c := domain.Csatorna{CsoportID: csoportID}
	var linkedAt string
	err := q.db.QueryRowContext(ctx,
		`SELECT guild_id, channel_id, channel_name, linked_by, linked_at FROM csatorna WHERE csoport_id = ?`,
		csoportID,
	).Scan(&c.GuildID, &c.ChannelID, &c.ChannelName, &c.LinkedBy, &linkedAt)
	if isNoRows(err) {
		return domain.Csatorna{}, ErrNotFound
	}
	if err != nil {
		return domain.Csatorna{}, fmt.Errorf("find csatorna: %w", err)
	}
	if c.LinkedAt, err = parseTime(linkedAt); err != nil {
		return domain.Csatorna{}, err
	}
	return c, nil
}

// DeleteCsatorna unlinks the Csoport's channel. Unlinking when none is
// linked is fine.
func (q Queries) DeleteCsatorna(ctx context.Context, csoportID string) error {
	if _, err := q.db.ExecContext(ctx, `DELETE FROM csatorna WHERE csoport_id = ?`, csoportID); err != nil {
		return fmt.Errorf("delete csatorna: %w", err)
	}
	return nil
}
