package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"squadsync-be/internal/domain"
)

// GetSzallasNote returns the Esemeny's Szallas note, "" if it has none.
func (q Queries) GetSzallasNote(ctx context.Context, esemenyID string) (string, error) {
	var note string
	err := q.db.QueryRowContext(ctx, `SELECT note FROM szallas WHERE esemeny_id = ?`, esemenyID).Scan(&note)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("get szallas note: %w", err)
	}
	return note, nil
}

// SetSzallasNote stores the Esemeny's Szallas note.
func (q Queries) SetSzallasNote(ctx context.Context, esemenyID, note string) error {
	_, err := q.db.ExecContext(ctx,
		`INSERT INTO szallas (esemeny_id, note) VALUES (?, ?)
		 ON CONFLICT (esemeny_id) DO UPDATE SET note = excluded.note`,
		esemenyID, note,
	)
	if err != nil {
		return fmt.Errorf("set szallas note: %w", err)
	}
	return nil
}

const szobaColumns = `id, esemeny_id, name, position, created_at`

func scanSzoba(row rowScanner) (domain.Szoba, error) {
	var s domain.Szoba
	var createdAt string
	if err := row.Scan(&s.ID, &s.EsemenyID, &s.Name, &s.Position, &createdAt); err != nil {
		return domain.Szoba{}, err
	}
	var err error
	if s.CreatedAt, err = parseTime(createdAt); err != nil {
		return domain.Szoba{}, err
	}
	return s, nil
}

// InsertSzoba stores a new Szoba after the Esemeny's other Szobak and sets
// its Position.
func (q Queries) InsertSzoba(ctx context.Context, s *domain.Szoba) error {
	err := q.db.QueryRowContext(ctx,
		`INSERT INTO szoba (id, esemeny_id, name, position, created_at)
		 VALUES (?, ?, ?, (SELECT COALESCE(MAX(position), 0) + 1 FROM szoba WHERE esemeny_id = ?), ?)
		 RETURNING position`,
		s.ID, s.EsemenyID, s.Name, s.EsemenyID, formatTime(s.CreatedAt),
	).Scan(&s.Position)
	if err != nil {
		return fmt.Errorf("insert szoba: %w", err)
	}
	return nil
}

// FindSzoba returns the Szoba with the given id, or ErrNotFound.
func (q Queries) FindSzoba(ctx context.Context, id string) (domain.Szoba, error) {
	s, err := scanSzoba(q.db.QueryRowContext(ctx, `SELECT `+szobaColumns+` FROM szoba WHERE id = ?`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return domain.Szoba{}, ErrNotFound
	}
	if err != nil {
		return domain.Szoba{}, fmt.Errorf("find szoba: %w", err)
	}
	return s, nil
}

// ListSzobak returns the Esemeny's Szobak in order.
func (q Queries) ListSzobak(ctx context.Context, esemenyID string) ([]domain.Szoba, error) {
	rows, err := q.db.QueryContext(ctx,
		`SELECT `+szobaColumns+` FROM szoba WHERE esemeny_id = ? ORDER BY position, rowid`, esemenyID)
	if err != nil {
		return nil, fmt.Errorf("list szobak: %w", err)
	}
	defer rows.Close()
	var list []domain.Szoba
	for rows.Next() {
		s, err := scanSzoba(rows)
		if err != nil {
			return nil, fmt.Errorf("list szobak: %w", err)
		}
		list = append(list, s)
	}
	return list, rows.Err()
}

// RenameSzoba changes a Szoba's name.
func (q Queries) RenameSzoba(ctx context.Context, id, name string) error {
	res, err := q.db.ExecContext(ctx, `UPDATE szoba SET name = ? WHERE id = ?`, name, id)
	if err != nil {
		return fmt.Errorf("rename szoba: %w", err)
	}
	return expectOneRow(res, "rename szoba")
}

// DeleteSzoba deletes a Szoba with its Agyak and their spots.
func (q Queries) DeleteSzoba(ctx context.Context, id string) error {
	res, err := q.db.ExecContext(ctx, `DELETE FROM szoba WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("delete szoba: %w", err)
	}
	return expectOneRow(res, "delete szoba")
}

const agyColumns = `id, szoba_id, esemeny_id, capacity, label, position, created_at`

func scanAgy(row rowScanner) (domain.Agy, error) {
	var a domain.Agy
	var createdAt string
	if err := row.Scan(&a.ID, &a.SzobaID, &a.EsemenyID, &a.Capacity, &a.Label, &a.Position, &createdAt); err != nil {
		return domain.Agy{}, err
	}
	var err error
	if a.CreatedAt, err = parseTime(createdAt); err != nil {
		return domain.Agy{}, err
	}
	return a, nil
}

// InsertAgy stores a new Agy after the Szoba's other Agyak and sets its
// Position.
func (q Queries) InsertAgy(ctx context.Context, a *domain.Agy) error {
	err := q.db.QueryRowContext(ctx,
		`INSERT INTO agy (id, szoba_id, esemeny_id, capacity, label, position, created_at)
		 VALUES (?, ?, ?, ?, ?, (SELECT COALESCE(MAX(position), 0) + 1 FROM agy WHERE szoba_id = ?), ?)
		 RETURNING position`,
		a.ID, a.SzobaID, a.EsemenyID, a.Capacity, a.Label, a.SzobaID, formatTime(a.CreatedAt),
	).Scan(&a.Position)
	if err != nil {
		return fmt.Errorf("insert agy: %w", err)
	}
	return nil
}

// FindAgy returns the Agy with the given id, or ErrNotFound.
func (q Queries) FindAgy(ctx context.Context, id string) (domain.Agy, error) {
	a, err := scanAgy(q.db.QueryRowContext(ctx, `SELECT `+agyColumns+` FROM agy WHERE id = ?`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return domain.Agy{}, ErrNotFound
	}
	if err != nil {
		return domain.Agy{}, fmt.Errorf("find agy: %w", err)
	}
	return a, nil
}

// ListAgyak returns every Agy of the Esemeny, Szoba by Szoba in order.
func (q Queries) ListAgyak(ctx context.Context, esemenyID string) ([]domain.Agy, error) {
	rows, err := q.db.QueryContext(ctx,
		`SELECT a.id, a.szoba_id, a.esemeny_id, a.capacity, a.label, a.position, a.created_at
		 FROM agy a JOIN szoba s ON s.id = a.szoba_id
		 WHERE a.esemeny_id = ?
		 ORDER BY s.position, s.rowid, a.position, a.rowid`, esemenyID)
	if err != nil {
		return nil, fmt.Errorf("list agyak: %w", err)
	}
	defer rows.Close()
	var list []domain.Agy
	for rows.Next() {
		a, err := scanAgy(rows)
		if err != nil {
			return nil, fmt.Errorf("list agyak: %w", err)
		}
		list = append(list, a)
	}
	return list, rows.Err()
}

// UpdateAgy changes an Agy's femohely and label.
func (q Queries) UpdateAgy(ctx context.Context, id string, capacity int, label string) error {
	res, err := q.db.ExecContext(ctx, `UPDATE agy SET capacity = ?, label = ? WHERE id = ?`, capacity, label, id)
	if err != nil {
		return fmt.Errorf("update agy: %w", err)
	}
	return expectOneRow(res, "update agy")
}

// DeleteAgy deletes an Agy with its spots.
func (q Queries) DeleteAgy(ctx context.Context, id string) error {
	res, err := q.db.ExecContext(ctx, `DELETE FROM agy WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("delete agy: %w", err)
	}
	return expectOneRow(res, "delete agy")
}

// ListFoglalasok returns every spot taken in the Esemeny's Agyak, ordered
// by the sleeper's name. A name-only Vendeg's (#13) has an empty name until
// #13 joins vendeg here.
func (q Queries) ListFoglalasok(ctx context.Context, esemenyID string) ([]domain.Foglalas, error) {
	rows, err := q.db.QueryContext(ctx,
		`SELECT g.agy_id, COALESCE(g.felhasznalo_id, ''), COALESCE(g.vendeg_id, ''), COALESCE(f.name, '')
		 FROM agy_foglalas g
		 LEFT JOIN felhasznalo f ON f.id = g.felhasznalo_id
		 WHERE g.esemeny_id = ?
		 ORDER BY COALESCE(f.name, ''), g.felhasznalo_id, g.vendeg_id`,
		esemenyID,
	)
	if err != nil {
		return nil, fmt.Errorf("list foglalasok: %w", err)
	}
	defer rows.Close()
	var list []domain.Foglalas
	for rows.Next() {
		var g domain.Foglalas
		if err := rows.Scan(&g.AgyID, &g.Holder.FelhasznaloID, &g.Holder.VendegID, &g.Name); err != nil {
			return nil, fmt.Errorf("list foglalasok: %w", err)
		}
		list = append(list, g)
	}
	return list, rows.Err()
}

// SetFoglalas gives the holder a spot in the Agy, replacing any spot they
// hold in the Esemeny.
func (q Queries) SetFoglalas(ctx context.Context, esemenyID, agyID string, h domain.ClaimHolder, at time.Time) error {
	f, v := holderColumns(h)
	conflict := `(esemeny_id, felhasznalo_id) WHERE felhasznalo_id IS NOT NULL`
	if h.VendegID != "" {
		conflict = `(esemeny_id, vendeg_id) WHERE vendeg_id IS NOT NULL`
	}
	_, err := q.db.ExecContext(ctx,
		`INSERT INTO agy_foglalas (agy_id, esemeny_id, felhasznalo_id, vendeg_id, created_at)
		 VALUES (?, ?, ?, ?, ?)
		 ON CONFLICT `+conflict+` DO UPDATE SET agy_id = excluded.agy_id, created_at = excluded.created_at`,
		agyID, esemenyID, f, v, formatTime(at),
	)
	if err != nil {
		return fmt.Errorf("set foglalas: %w", err)
	}
	return nil
}

// DeleteFoglalas removes the holder's spot in the Esemeny, if they have one.
func (q Queries) DeleteFoglalas(ctx context.Context, esemenyID string, h domain.ClaimHolder) error {
	f, v := holderColumns(h)
	_, err := q.db.ExecContext(ctx,
		`DELETE FROM agy_foglalas WHERE esemeny_id = ? AND felhasznalo_id IS ? AND vendeg_id IS ?`, esemenyID, f, v)
	if err != nil {
		return fmt.Errorf("delete foglalas: %w", err)
	}
	return nil
}

// ListMasholAlszik returns everyone marked máshol alszik in the Esemeny,
// ordered by name.
func (q Queries) ListMasholAlszik(ctx context.Context, esemenyID string) ([]domain.MasholAlszik, error) {
	rows, err := q.db.QueryContext(ctx,
		`SELECT COALESCE(m.felhasznalo_id, ''), COALESCE(m.vendeg_id, ''), COALESCE(f.name, '')
		 FROM mashol_alszik m
		 LEFT JOIN felhasznalo f ON f.id = m.felhasznalo_id
		 WHERE m.esemeny_id = ?
		 ORDER BY COALESCE(f.name, ''), m.felhasznalo_id, m.vendeg_id`,
		esemenyID,
	)
	if err != nil {
		return nil, fmt.Errorf("list mashol alszik: %w", err)
	}
	defer rows.Close()
	var list []domain.MasholAlszik
	for rows.Next() {
		var m domain.MasholAlszik
		if err := rows.Scan(&m.Holder.FelhasznaloID, &m.Holder.VendegID, &m.Name); err != nil {
			return nil, fmt.Errorf("list mashol alszik: %w", err)
		}
		list = append(list, m)
	}
	return list, rows.Err()
}

// SetMasholAlszik marks the holder máshol alszik, or removes the mark.
func (q Queries) SetMasholAlszik(ctx context.Context, esemenyID string, h domain.ClaimHolder, on bool, at time.Time) error {
	f, v := holderColumns(h)
	if !on {
		_, err := q.db.ExecContext(ctx,
			`DELETE FROM mashol_alszik WHERE esemeny_id = ? AND felhasznalo_id IS ? AND vendeg_id IS ?`, esemenyID, f, v)
		if err != nil {
			return fmt.Errorf("delete mashol alszik: %w", err)
		}
		return nil
	}
	conflict := `(esemeny_id, felhasznalo_id) WHERE felhasznalo_id IS NOT NULL`
	if h.VendegID != "" {
		conflict = `(esemeny_id, vendeg_id) WHERE vendeg_id IS NOT NULL`
	}
	_, err := q.db.ExecContext(ctx,
		`INSERT INTO mashol_alszik (esemeny_id, felhasznalo_id, vendeg_id, created_at)
		 VALUES (?, ?, ?, ?) ON CONFLICT `+conflict+` DO NOTHING`,
		esemenyID, f, v, formatTime(at),
	)
	if err != nil {
		return fmt.Errorf("set mashol alszik: %w", err)
	}
	return nil
}
