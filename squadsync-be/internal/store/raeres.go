package store

import (
	"context"
	"fmt"
	"time"

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

// ListEgyeztetesEsemenyIDsOnDays returns the ids of the Esemenyek in
// Egyeztetes, in the Csoportok the Felhasznalo is a Tag of, that have an
// Idopont overlapping from..to (to exclusive). An Idopont with an end
// covers [start, end); one without covers only its start.
func (q Queries) ListEgyeztetesEsemenyIDsOnDays(ctx context.Context, felhasznaloID string, from, to time.Time) ([]string, error) {
	rows, err := q.db.QueryContext(ctx,
		`SELECT DISTINCT e.id
		 FROM esemeny e
		 JOIN tag t ON t.csoport_id = e.csoport_id AND t.felhasznalo_id = ?
		 JOIN idopont i ON i.esemeny_id = e.id
		 WHERE e.allapot = 'egyeztetes'
		   AND i.start_at < ?
		   AND (CASE WHEN i.end_at IS NULL THEN i.start_at >= ? ELSE i.end_at > ? END)
		 ORDER BY e.id`,
		felhasznaloID, formatTime(to), formatTime(from), formatTime(from),
	)
	if err != nil {
		return nil, fmt.Errorf("list esemenyek on days: %w", err)
	}
	defer rows.Close()

	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("list esemenyek on days: %w", err)
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}
