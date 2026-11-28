package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"squadsync-be/internal/domain"
)

const kiadasColumns = `k.id, k.esemeny_id, k.description, k.amount, k.payer_id, COALESCE(f.name, ''),
	k.allapot, k.egyenlo, k.created_by, k.created_at, k.updated_at`

const kiadasFrom = ` FROM kiadas k LEFT JOIN felhasznalo f ON f.id = k.payer_id`

func scanKiadas(row rowScanner) (domain.Kiadas, error) {
	var k domain.Kiadas
	var allapot, createdAt, updatedAt string
	if err := row.Scan(&k.ID, &k.EsemenyID, &k.Description, &k.Amount, &k.PayerID, &k.PayerName,
		&allapot, &k.Egyenlo, &k.CreatedBy, &createdAt, &updatedAt); err != nil {
		return domain.Kiadas{}, err
	}
	k.Allapot = domain.KiadasAllapot(allapot)
	var err error
	if k.CreatedAt, err = parseTime(createdAt); err != nil {
		return domain.Kiadas{}, err
	}
	if k.UpdatedAt, err = parseTime(updatedAt); err != nil {
		return domain.Kiadas{}, err
	}
	return k, nil
}

// InsertKiadas stores a new Kiadas (without its shares: SetKiadasReszek).
func (q Queries) InsertKiadas(ctx context.Context, k domain.Kiadas) error {
	_, err := q.db.ExecContext(ctx,
		`INSERT INTO kiadas (id, esemeny_id, description, amount, payer_id, allapot, egyenlo, created_by, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		k.ID, k.EsemenyID, k.Description, k.Amount, k.PayerID, string(k.Allapot), k.Egyenlo, k.CreatedBy,
		formatTime(k.CreatedAt), formatTime(k.UpdatedAt),
	)
	if err != nil {
		return fmt.Errorf("insert kiadas: %w", err)
	}
	return nil
}

// FindKiadas returns the Kiadas with the given id, or ErrNotFound.
func (q Queries) FindKiadas(ctx context.Context, id string) (domain.Kiadas, error) {
	k, err := scanKiadas(q.db.QueryRowContext(ctx, `SELECT `+kiadasColumns+kiadasFrom+` WHERE k.id = ?`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return domain.Kiadas{}, ErrNotFound
	}
	if err != nil {
		return domain.Kiadas{}, fmt.Errorf("find kiadas: %w", err)
	}
	return k, nil
}

// UpdateKiadas changes a Kiadas's description, amount, payer, state, split
// mode and updated_at.
func (q Queries) UpdateKiadas(ctx context.Context, k domain.Kiadas) error {
	res, err := q.db.ExecContext(ctx,
		`UPDATE kiadas SET description = ?, amount = ?, payer_id = ?, allapot = ?, egyenlo = ?, updated_at = ? WHERE id = ?`,
		k.Description, k.Amount, k.PayerID, string(k.Allapot), k.Egyenlo, formatTime(k.UpdatedAt), k.ID,
	)
	if err != nil {
		return fmt.Errorf("update kiadas: %w", err)
	}
	return expectOneRow(res, "update kiadas")
}

// DeleteKiadas deletes a Kiadas with its shares.
func (q Queries) DeleteKiadas(ctx context.Context, id string) error {
	res, err := q.db.ExecContext(ctx, `DELETE FROM kiadas WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("delete kiadas: %w", err)
	}
	return expectOneRow(res, "delete kiadas")
}

// ListKiadasok returns the Esemeny's Kiadasok, oldest first.
func (q Queries) ListKiadasok(ctx context.Context, esemenyID string) ([]domain.Kiadas, error) {
	rows, err := q.db.QueryContext(ctx,
		`SELECT `+kiadasColumns+kiadasFrom+` WHERE k.esemeny_id = ? ORDER BY k.created_at, k.rowid`, esemenyID)
	if err != nil {
		return nil, fmt.Errorf("list kiadasok: %w", err)
	}
	defer rows.Close()
	var list []domain.Kiadas
	for rows.Next() {
		k, err := scanKiadas(rows)
		if err != nil {
			return nil, fmt.Errorf("list kiadasok: %w", err)
		}
		list = append(list, k)
	}
	return list, rows.Err()
}

// CountKiadasok returns how many Kiadasok the Esemeny has.
func (q Queries) CountKiadasok(ctx context.Context, esemenyID string) (int, error) {
	var n int
	if err := q.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM kiadas WHERE esemeny_id = ?`, esemenyID).Scan(&n); err != nil {
		return 0, fmt.Errorf("count kiadasok: %w", err)
	}
	return n, nil
}

// SetKiadasReszek replaces the Kiadas's shares with reszek (their KiadasID
// and Name are ignored).
func (q Queries) SetKiadasReszek(ctx context.Context, kiadasID string, reszek []domain.KiadasResz) error {
	if _, err := q.db.ExecContext(ctx, `DELETE FROM kiadas_resz WHERE kiadas_id = ?`, kiadasID); err != nil {
		return fmt.Errorf("clear kiadas reszek: %w", err)
	}
	for _, r := range reszek {
		f, v := holderColumns(r.Holder)
		_, err := q.db.ExecContext(ctx,
			`INSERT INTO kiadas_resz (kiadas_id, felhasznalo_id, vendeg_id, amount) VALUES (?, ?, ?, ?)`,
			kiadasID, f, v, r.Amount)
		if err != nil {
			return fmt.Errorf("insert kiadas resz: %w", err)
		}
	}
	return nil
}

// ListKiadasReszekOfEsemeny returns the shares of every Kiadas of the
// Esemeny, in the stable id order of their holders (Felhasznalok by id,
// then name-only Vendegs by id). Each has the holder's name and whom it is
// charged to: the Felhasznalo themselves, or a name-only Vendeg's Tag.
func (q Queries) ListKiadasReszekOfEsemeny(ctx context.Context, esemenyID string) ([]domain.KiadasResz, error) {
	rows, err := q.db.QueryContext(ctx,
		`SELECT r.kiadas_id, COALESCE(r.felhasznalo_id, ''), COALESCE(r.vendeg_id, ''), COALESCE(f.name, vg.name, ''), r.amount,
		        COALESCE(r.felhasznalo_id, vg.added_by, ''), COALESCE(f.name, a.name, '')
		 FROM kiadas_resz r
		 JOIN kiadas k ON k.id = r.kiadas_id
		 LEFT JOIN felhasznalo f ON f.id = r.felhasznalo_id
		 LEFT JOIN vendeg vg ON vg.id = r.vendeg_id
		 LEFT JOIN felhasznalo a ON a.id = vg.added_by
		 WHERE k.esemeny_id = ?
		 ORDER BY r.kiadas_id, r.felhasznalo_id IS NULL, r.felhasznalo_id, r.vendeg_id`,
		esemenyID,
	)
	if err != nil {
		return nil, fmt.Errorf("list kiadas reszek: %w", err)
	}
	defer rows.Close()
	var list []domain.KiadasResz
	for rows.Next() {
		var r domain.KiadasResz
		if err := rows.Scan(&r.KiadasID, &r.Holder.FelhasznaloID, &r.Holder.VendegID, &r.Name, &r.Amount, &r.ChargedTo, &r.ChargedToName); err != nil {
			return nil, fmt.Errorf("list kiadas reszek: %w", err)
		}
		list = append(list, r)
	}
	return list, rows.Err()
}
