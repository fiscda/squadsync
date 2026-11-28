package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"squadsync-be/internal/domain"
)

// InsertElszamolasMegerosites records that the holder's Kiadasok are
// recorded, confirmed by megerositette. Confirming again changes nothing.
func (q Queries) InsertElszamolasMegerosites(ctx context.Context, esemenyID string, h domain.ClaimHolder, megerositette string, at time.Time) error {
	f, v := holderColumns(h)
	_, err := q.db.ExecContext(ctx,
		`INSERT INTO elszamolas_megerosites (esemeny_id, felhasznalo_id, vendeg_id, megerositette, created_at)
		 VALUES (?, ?, ?, ?, ?) ON CONFLICT DO NOTHING`,
		esemenyID, f, v, megerositette, formatTime(at))
	if err != nil {
		return fmt.Errorf("insert elszamolas megerosites: %w", err)
	}
	return nil
}

// DeleteElszamolasMegerosites removes the holder's confirmation, if any.
func (q Queries) DeleteElszamolasMegerosites(ctx context.Context, esemenyID string, h domain.ClaimHolder) error {
	col, id := "felhasznalo_id", h.FelhasznaloID
	if h.VendegID != "" {
		col, id = "vendeg_id", h.VendegID
	}
	if _, err := q.db.ExecContext(ctx,
		`DELETE FROM elszamolas_megerosites WHERE esemeny_id = ? AND `+col+` = ?`, esemenyID, id); err != nil {
		return fmt.Errorf("delete elszamolas megerosites: %w", err)
	}
	return nil
}

// DeleteElszamolasMegerositesek removes every confirmation of the Esemeny.
func (q Queries) DeleteElszamolasMegerositesek(ctx context.Context, esemenyID string) error {
	if _, err := q.db.ExecContext(ctx, `DELETE FROM elszamolas_megerosites WHERE esemeny_id = ?`, esemenyID); err != nil {
		return fmt.Errorf("delete elszamolas megerositesek: %w", err)
	}
	return nil
}

// ListElszamolasMegerositesek returns whose Kiadasok are confirmed as
// recorded for the Esemeny.
func (q Queries) ListElszamolasMegerositesek(ctx context.Context, esemenyID string) (map[domain.ClaimHolder]bool, error) {
	rows, err := q.db.QueryContext(ctx,
		`SELECT COALESCE(felhasznalo_id, ''), COALESCE(vendeg_id, '') FROM elszamolas_megerosites WHERE esemeny_id = ?`, esemenyID)
	if err != nil {
		return nil, fmt.Errorf("list elszamolas megerositesek: %w", err)
	}
	defer rows.Close()
	out := map[domain.ClaimHolder]bool{}
	for rows.Next() {
		var h domain.ClaimHolder
		if err := rows.Scan(&h.FelhasznaloID, &h.VendegID); err != nil {
			return nil, fmt.Errorf("list elszamolas megerositesek: %w", err)
		}
		out[h] = true
	}
	return out, rows.Err()
}

// InsertElszamolas stores a calculated Elszamolas (without its Utalasok:
// InsertUtalas).
func (q Queries) InsertElszamolas(ctx context.Context, e domain.Elszamolas) error {
	var lezarta any
	if e.LezartaID != "" {
		lezarta = e.LezartaID
	}
	if _, err := q.db.ExecContext(ctx,
		`INSERT INTO elszamolas (esemeny_id, kiszamolva_at, lezarta) VALUES (?, ?, ?)`,
		e.EsemenyID, formatTime(e.KiszamolvaAt), lezarta); err != nil {
		return fmt.Errorf("insert elszamolas: %w", err)
	}
	return nil
}

// FindElszamolas returns the Esemeny's calculated Elszamolas, or
// ErrNotFound if it has none.
func (q Queries) FindElszamolas(ctx context.Context, esemenyID string) (domain.Elszamolas, error) {
	var e domain.Elszamolas
	var at string
	err := q.db.QueryRowContext(ctx,
		`SELECT esemeny_id, kiszamolva_at, COALESCE(lezarta, '') FROM elszamolas WHERE esemeny_id = ?`, esemenyID,
	).Scan(&e.EsemenyID, &at, &e.LezartaID)
	if errors.Is(err, sql.ErrNoRows) {
		return domain.Elszamolas{}, ErrNotFound
	}
	if err != nil {
		return domain.Elszamolas{}, fmt.Errorf("find elszamolas: %w", err)
	}
	if e.KiszamolvaAt, err = parseTime(at); err != nil {
		return domain.Elszamolas{}, err
	}
	return e, nil
}

// HasElszamolas tells whether the Esemeny has a calculated Elszamolas.
func (q Queries) HasElszamolas(ctx context.Context, esemenyID string) (bool, error) {
	var n int
	if err := q.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM elszamolas WHERE esemeny_id = ?`, esemenyID).Scan(&n); err != nil {
		return false, fmt.Errorf("has elszamolas: %w", err)
	}
	return n > 0, nil
}

// DeleteElszamolas deletes the Esemeny's calculated Elszamolas with its
// Utalasok.
func (q Queries) DeleteElszamolas(ctx context.Context, esemenyID string) error {
	res, err := q.db.ExecContext(ctx, `DELETE FROM elszamolas WHERE esemeny_id = ?`, esemenyID)
	if err != nil {
		return fmt.Errorf("delete elszamolas: %w", err)
	}
	return expectOneRow(res, "delete elszamolas")
}

// InsertUtalas stores one Utalas of a calculated Elszamolas; sorszam is its
// place in the calculation's order.
func (q Queries) InsertUtalas(ctx context.Context, u domain.Utalas, sorszam int) error {
	if _, err := q.db.ExecContext(ctx,
		`INSERT INTO utalas (id, esemeny_id, from_id, to_id, amount, sorszam) VALUES (?, ?, ?, ?, ?, ?)`,
		u.ID, u.EsemenyID, u.FromID, u.ToID, u.Amount, sorszam); err != nil {
		return fmt.Errorf("insert utalas: %w", err)
	}
	return nil
}

const utalasSelect = `SELECT u.id, u.esemeny_id, u.from_id, u.to_id, COALESCE(ff.name, ''), COALESCE(ft.name, ''),
	u.amount, u.megvolt_at, COALESCE(u.megvolt_by, ''), COALESCE(fm.name, '')
	FROM utalas u
	LEFT JOIN felhasznalo ff ON ff.id = u.from_id
	LEFT JOIN felhasznalo ft ON ft.id = u.to_id
	LEFT JOIN felhasznalo fm ON fm.id = u.megvolt_by`

func scanUtalas(row rowScanner) (domain.Utalas, error) {
	var u domain.Utalas
	var megvolt sql.NullString
	if err := row.Scan(&u.ID, &u.EsemenyID, &u.FromID, &u.ToID, &u.FromName, &u.ToName,
		&u.Amount, &megvolt, &u.MegvoltBy, &u.MegvoltByName); err != nil {
		return domain.Utalas{}, err
	}
	if megvolt.Valid {
		at, err := parseTime(megvolt.String)
		if err != nil {
			return domain.Utalas{}, err
		}
		u.MegvoltAt = &at
	}
	return u, nil
}

// ListUtalasok returns the Utalasok of the Esemeny's calculated
// Elszamolas, in the calculation's order.
func (q Queries) ListUtalasok(ctx context.Context, esemenyID string) ([]domain.Utalas, error) {
	rows, err := q.db.QueryContext(ctx, utalasSelect+` WHERE u.esemeny_id = ? ORDER BY u.sorszam`, esemenyID)
	if err != nil {
		return nil, fmt.Errorf("list utalasok: %w", err)
	}
	defer rows.Close()
	var list []domain.Utalas
	for rows.Next() {
		u, err := scanUtalas(rows)
		if err != nil {
			return nil, fmt.Errorf("list utalasok: %w", err)
		}
		list = append(list, u)
	}
	return list, rows.Err()
}

// FindUtalas returns the Utalas with the given id, or ErrNotFound.
func (q Queries) FindUtalas(ctx context.Context, id string) (domain.Utalas, error) {
	u, err := scanUtalas(q.db.QueryRowContext(ctx, utalasSelect+` WHERE u.id = ?`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return domain.Utalas{}, ErrNotFound
	}
	if err != nil {
		return domain.Utalas{}, fmt.Errorf("find utalas: %w", err)
	}
	return u, nil
}

// SetUtalasMegvolt marks the Utalas done by felhasznaloID at at, or (with
// at nil) not done.
func (q Queries) SetUtalasMegvolt(ctx context.Context, id string, at *time.Time, felhasznaloID string) error {
	var when, by any
	if at != nil {
		when, by = formatTime(*at), felhasznaloID
	}
	res, err := q.db.ExecContext(ctx, `UPDATE utalas SET megvolt_at = ?, megvolt_by = ? WHERE id = ?`, when, by, id)
	if err != nil {
		return fmt.Errorf("set utalas megvolt: %w", err)
	}
	return expectOneRow(res, "set utalas megvolt")
}
