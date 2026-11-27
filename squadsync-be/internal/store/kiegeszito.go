package store

import (
	"context"
	"fmt"
	"time"

	"squadsync-be/internal/domain"
)

// EnabledKiegeszitok returns the Kiegeszitok switched on for the Esemeny.
func (q Queries) EnabledKiegeszitok(ctx context.Context, esemenyID string) (map[domain.KiegeszitoKind]bool, error) {
	rows, err := q.db.QueryContext(ctx,
		`SELECT kind FROM kiegeszito WHERE esemeny_id = ? AND enabled = 1`, esemenyID)
	if err != nil {
		return nil, fmt.Errorf("list kiegeszitok: %w", err)
	}
	defer rows.Close()
	out := map[domain.KiegeszitoKind]bool{}
	for rows.Next() {
		var kind string
		if err := rows.Scan(&kind); err != nil {
			return nil, fmt.Errorf("list kiegeszitok: %w", err)
		}
		out[domain.KiegeszitoKind(kind)] = true
	}
	return out, rows.Err()
}

// SetKiegeszitoEnabled switches a Kiegeszito of the Esemeny on or off. It
// never touches the Kiegeszito's data.
func (q Queries) SetKiegeszitoEnabled(ctx context.Context, esemenyID string, kind domain.KiegeszitoKind, enabled bool, at time.Time) error {
	_, err := q.db.ExecContext(ctx,
		`INSERT INTO kiegeszito (esemeny_id, kind, enabled, updated_at) VALUES (?, ?, ?, ?)
		 ON CONFLICT (esemeny_id, kind) DO UPDATE SET enabled = excluded.enabled, updated_at = excluded.updated_at`,
		esemenyID, string(kind), enabled, formatTime(at),
	)
	if err != nil {
		return fmt.Errorf("set kiegeszito: %w", err)
	}
	return nil
}
