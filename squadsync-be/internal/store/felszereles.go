package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"squadsync-be/internal/domain"
)

const tetelColumns = `id, esemeny_id, name, quantity, mindenki_hozza, added_by, created_at`

func scanTetel(row rowScanner) (domain.Tetel, error) {
	var t domain.Tetel
	var createdAt string
	if err := row.Scan(&t.ID, &t.EsemenyID, &t.Name, &t.Quantity, &t.MindenkiHozza, &t.AddedBy, &createdAt); err != nil {
		return domain.Tetel{}, err
	}
	var err error
	if t.CreatedAt, err = parseTime(createdAt); err != nil {
		return domain.Tetel{}, err
	}
	return t, nil
}

// InsertTetel stores a new Tetel.
func (q Queries) InsertTetel(ctx context.Context, t domain.Tetel) error {
	_, err := q.db.ExecContext(ctx,
		`INSERT INTO tetel (`+tetelColumns+`) VALUES (?, ?, ?, ?, ?, ?, ?)`,
		t.ID, t.EsemenyID, t.Name, t.Quantity, t.MindenkiHozza, t.AddedBy, formatTime(t.CreatedAt),
	)
	if err != nil {
		return fmt.Errorf("insert tetel: %w", err)
	}
	return nil
}

// FindTetel returns the Tetel with the given id, or ErrNotFound.
func (q Queries) FindTetel(ctx context.Context, id string) (domain.Tetel, error) {
	t, err := scanTetel(q.db.QueryRowContext(ctx, `SELECT `+tetelColumns+` FROM tetel WHERE id = ?`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return domain.Tetel{}, ErrNotFound
	}
	if err != nil {
		return domain.Tetel{}, fmt.Errorf("find tetel: %w", err)
	}
	return t, nil
}

// UpdateTetel changes a Tetel's name, quantity and "mindenki hozza" mark.
func (q Queries) UpdateTetel(ctx context.Context, t domain.Tetel) error {
	res, err := q.db.ExecContext(ctx,
		`UPDATE tetel SET name = ?, quantity = ?, mindenki_hozza = ? WHERE id = ?`,
		t.Name, t.Quantity, t.MindenkiHozza, t.ID,
	)
	if err != nil {
		return fmt.Errorf("update tetel: %w", err)
	}
	return expectOneRow(res, "update tetel")
}

// DeleteTetel deletes a Tetel with its Vallalasok.
func (q Queries) DeleteTetel(ctx context.Context, id string) error {
	res, err := q.db.ExecContext(ctx, `DELETE FROM tetel WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("delete tetel: %w", err)
	}
	return expectOneRow(res, "delete tetel")
}

// ListTetelek returns the Esemeny's Tetelek, oldest first (in the order
// they were added, for the same time).
func (q Queries) ListTetelek(ctx context.Context, esemenyID string) ([]domain.Tetel, error) {
	rows, err := q.db.QueryContext(ctx,
		`SELECT `+tetelColumns+` FROM tetel WHERE esemeny_id = ? ORDER BY created_at, rowid`, esemenyID)
	if err != nil {
		return nil, fmt.Errorf("list tetelek: %w", err)
	}
	defer rows.Close()
	var list []domain.Tetel
	for rows.Next() {
		t, err := scanTetel(rows)
		if err != nil {
			return nil, fmt.Errorf("list tetelek: %w", err)
		}
		list = append(list, t)
	}
	return list, rows.Err()
}

// ListVallalasokOfEsemeny returns every Vallalas on the Esemeny's Tetelek,
// ordered by the holder's name (a name-only
// Vendeg's by their name too).
func (q Queries) ListVallalasokOfEsemeny(ctx context.Context, esemenyID string) ([]domain.Vallalas, error) {
	rows, err := q.db.QueryContext(ctx,
		`SELECT v.tetel_id, COALESCE(v.felhasznalo_id, ''), COALESCE(v.vendeg_id, ''), COALESCE(f.name, vg.name, ''), v.quantity
		 FROM tetel_vallalas v
		 JOIN tetel t ON t.id = v.tetel_id
		 LEFT JOIN felhasznalo f ON f.id = v.felhasznalo_id
		 LEFT JOIN vendeg vg ON vg.id = v.vendeg_id
		 WHERE t.esemeny_id = ?
		 ORDER BY COALESCE(f.name, vg.name, ''), v.felhasznalo_id, v.vendeg_id`,
		esemenyID,
	)
	if err != nil {
		return nil, fmt.Errorf("list vallalasok: %w", err)
	}
	defer rows.Close()
	var list []domain.Vallalas
	for rows.Next() {
		var v domain.Vallalas
		if err := rows.Scan(&v.TetelID, &v.Holder.FelhasznaloID, &v.Holder.VendegID, &v.Name, &v.Quantity); err != nil {
			return nil, fmt.Errorf("list vallalasok: %w", err)
		}
		list = append(list, v)
	}
	return list, rows.Err()
}

// holderColumns returns the felhasznalo_id and vendeg_id values of a
// ClaimHolder, NULL for the unset one.
func holderColumns(h domain.ClaimHolder) (felhasznaloID, vendegID any) {
	if h.VendegID != "" {
		return nil, h.VendegID
	}
	return h.FelhasznaloID, nil
}

// SetVallalas stores how many of the Tetel the holder claims, replacing
// their earlier Vallalas. A quantity of 0 removes it.
func (q Queries) SetVallalas(ctx context.Context, tetelID string, h domain.ClaimHolder, quantity int, at time.Time) error {
	f, v := holderColumns(h)
	if quantity == 0 {
		_, err := q.db.ExecContext(ctx,
			`DELETE FROM tetel_vallalas WHERE tetel_id = ? AND felhasznalo_id IS ? AND vendeg_id IS ?`, tetelID, f, v)
		if err != nil {
			return fmt.Errorf("delete vallalas: %w", err)
		}
		return nil
	}
	conflict := `(tetel_id, felhasznalo_id) WHERE felhasznalo_id IS NOT NULL`
	if h.VendegID != "" {
		conflict = `(tetel_id, vendeg_id) WHERE vendeg_id IS NOT NULL`
	}
	_, err := q.db.ExecContext(ctx,
		`INSERT INTO tetel_vallalas (tetel_id, felhasznalo_id, vendeg_id, quantity, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?)
		 ON CONFLICT `+conflict+` DO UPDATE SET quantity = excluded.quantity, updated_at = excluded.updated_at`,
		tetelID, f, v, quantity, formatTime(at), formatTime(at),
	)
	if err != nil {
		return fmt.Errorf("set vallalas: %w", err)
	}
	return nil
}
