package store

import (
	"context"
	"fmt"

	"squadsync-be/internal/domain"
)

// UpsertRaeres marks one day (a Budapest date, YYYY-MM-DD) for the
// Felhasznalo, replacing any earlier mark.
func (q Queries) UpsertRaeres(ctx context.Context, felhasznaloID, day string, allapot domain.RaeresAllapot) error {
	_, err := q.db.ExecContext(ctx,
		`INSERT INTO raeres (felhasznalo_id, day, allapot) VALUES (?, ?, ?)
		 ON CONFLICT (felhasznalo_id, day) DO UPDATE SET allapot = excluded.allapot`,
		felhasznaloID, day, string(allapot),
	)
	if err != nil {
		return fmt.Errorf("upsert raeres: %w", err)
	}
	return nil
}

// DeleteRaeresRange makes the days from..to (inclusive) unknown again.
func (q Queries) DeleteRaeresRange(ctx context.Context, felhasznaloID, from, to string) error {
	_, err := q.db.ExecContext(ctx,
		`DELETE FROM raeres WHERE felhasznalo_id = ? AND day BETWEEN ? AND ?`,
		felhasznaloID, from, to,
	)
	if err != nil {
		return fmt.Errorf("delete raeres: %w", err)
	}
	return nil
}

// ListRaeres returns the Felhasznalo's marked days from..to (inclusive),
// oldest first.
func (q Queries) ListRaeres(ctx context.Context, felhasznaloID, from, to string) ([]domain.RaeresDay, error) {
	rows, err := q.db.QueryContext(ctx,
		`SELECT day, allapot FROM raeres WHERE felhasznalo_id = ? AND day BETWEEN ? AND ? ORDER BY day`,
		felhasznaloID, from, to,
	)
	if err != nil {
		return nil, fmt.Errorf("list raeres: %w", err)
	}
	defer rows.Close()

	var days []domain.RaeresDay
	for rows.Next() {
		var d domain.RaeresDay
		var allapot string
		if err := rows.Scan(&d.Day, &allapot); err != nil {
			return nil, fmt.Errorf("list raeres: %w", err)
		}
		d.Allapot = domain.RaeresAllapot(allapot)
		days = append(days, d)
	}
	return days, rows.Err()
}

// CountTagsSzabadPerDay returns, for each day from..to (inclusive) on which
// at least one Tag of the Csoport is szabad, how many are. Days nobody is
// szabad on are missing.
func (q Queries) CountTagsSzabadPerDay(ctx context.Context, csoportID, from, to string) (map[string]int, error) {
	rows, err := q.db.QueryContext(ctx,
		`SELECT r.day, COUNT(*)
		 FROM raeres r JOIN tag t ON t.felhasznalo_id = r.felhasznalo_id
		 WHERE t.csoport_id = ? AND r.allapot = 'szabad' AND r.day BETWEEN ? AND ?
		 GROUP BY r.day`,
		csoportID, from, to,
	)
	if err != nil {
		return nil, fmt.Errorf("count szabad per day: %w", err)
	}
	defer rows.Close()

	counts := map[string]int{}
	for rows.Next() {
		var day string
		var n int
		if err := rows.Scan(&day, &n); err != nil {
			return nil, fmt.Errorf("count szabad per day: %w", err)
		}
		counts[day] = n
	}
	return counts, rows.Err()
}

// CountTagsSzabadOnEveryDay returns how many Tags of the Csoport are szabad
// on each of the days from..to (inclusive), which span exactly dayCount days.
func (q Queries) CountTagsSzabadOnEveryDay(ctx context.Context, csoportID, from, to string, dayCount int) (int, error) {
	var n int
	err := q.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM tag t
		 WHERE t.csoport_id = ?
		   AND (SELECT COUNT(*) FROM raeres r
		        WHERE r.felhasznalo_id = t.felhasznalo_id AND r.allapot = 'szabad' AND r.day BETWEEN ? AND ?) = ?`,
		csoportID, from, to, dayCount,
	).Scan(&n)
	if err != nil {
		return 0, fmt.Errorf("count szabad on every day: %w", err)
	}
	return n, nil
}
