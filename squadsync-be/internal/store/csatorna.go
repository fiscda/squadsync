package store

import (
	"context"
	"fmt"

	"squadsync-be/internal/domain"
)

// InsertCsatorna links the channel to its Csoport. The Csoport must have no
// linked channel yet, and the channel no Csoport.
func (q Queries) InsertCsatorna(ctx context.Context, c domain.Csatorna) error {
	_, err := q.db.ExecContext(ctx,
		`INSERT INTO csatorna (csoport_id, guild_id, channel_id, channel_name, linked_by, linked_at)
		 VALUES (?, ?, ?, ?, ?, ?)`,
		c.CsoportID, c.GuildID, c.ChannelID, c.ChannelName, c.LinkedBy, formatTime(c.LinkedAt),
	)
	if err != nil {
		return fmt.Errorf("insert csatorna: %w", err)
	}
	return nil
}

// FindCsoportIDOfChannel returns the id of the Csoport the channel is linked
// to, or ErrNotFound.
func (q Queries) FindCsoportIDOfChannel(ctx context.Context, channelID string) (string, error) {
	var id string
	err := q.db.QueryRowContext(ctx, `SELECT csoport_id FROM csatorna WHERE channel_id = ?`, channelID).Scan(&id)
	if isNoRows(err) {
		return "", ErrNotFound
	}
	if err != nil {
		return "", fmt.Errorf("find csoport of channel: %w", err)
	}
	return id, nil
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
