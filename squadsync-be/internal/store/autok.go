package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"squadsync-be/internal/domain"
)

const autoColumns = `a.id, a.esemeny_id, a.sofor_id, f.name, a.seats, a.note, a.created_at`

const autoFrom = ` FROM auto a JOIN felhasznalo f ON f.id = a.sofor_id`

func scanAuto(row rowScanner) (domain.Auto, error) {
	var a domain.Auto
	var createdAt string
	if err := row.Scan(&a.ID, &a.EsemenyID, &a.SoforID, &a.SoforName, &a.Seats, &a.Note, &createdAt); err != nil {
		return domain.Auto{}, err
	}
	var err error
	if a.CreatedAt, err = parseTime(createdAt); err != nil {
		return domain.Auto{}, err
	}
	return a, nil
}

// InsertAuto stores a new Auto.
func (q Queries) InsertAuto(ctx context.Context, a domain.Auto) error {
	_, err := q.db.ExecContext(ctx,
		`INSERT INTO auto (id, esemeny_id, sofor_id, seats, note, created_at) VALUES (?, ?, ?, ?, ?, ?)`,
		a.ID, a.EsemenyID, a.SoforID, a.Seats, a.Note, formatTime(a.CreatedAt),
	)
	if err != nil {
		return fmt.Errorf("insert auto: %w", err)
	}
	return nil
}

// FindAuto returns the Auto with the given id, or ErrNotFound.
func (q Queries) FindAuto(ctx context.Context, id string) (domain.Auto, error) {
	a, err := scanAuto(q.db.QueryRowContext(ctx, `SELECT `+autoColumns+autoFrom+` WHERE a.id = ?`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return domain.Auto{}, ErrNotFound
	}
	if err != nil {
		return domain.Auto{}, fmt.Errorf("find auto: %w", err)
	}
	return a, nil
}

// FindAutoOfSofor returns the Auto the Felhasznalo drives in the Esemeny,
// or ErrNotFound.
func (q Queries) FindAutoOfSofor(ctx context.Context, esemenyID, soforID string) (domain.Auto, error) {
	a, err := scanAuto(q.db.QueryRowContext(ctx,
		`SELECT `+autoColumns+autoFrom+` WHERE a.esemeny_id = ? AND a.sofor_id = ?`, esemenyID, soforID))
	if errors.Is(err, sql.ErrNoRows) {
		return domain.Auto{}, ErrNotFound
	}
	if err != nil {
		return domain.Auto{}, fmt.Errorf("find auto of sofor: %w", err)
	}
	return a, nil
}

// ListAutok returns the Esemeny's Autok, oldest first.
func (q Queries) ListAutok(ctx context.Context, esemenyID string) ([]domain.Auto, error) {
	rows, err := q.db.QueryContext(ctx,
		`SELECT `+autoColumns+autoFrom+` WHERE a.esemeny_id = ? ORDER BY a.created_at, a.rowid`, esemenyID)
	if err != nil {
		return nil, fmt.Errorf("list autok: %w", err)
	}
	defer rows.Close()
	var list []domain.Auto
	for rows.Next() {
		a, err := scanAuto(rows)
		if err != nil {
			return nil, fmt.Errorf("list autok: %w", err)
		}
		list = append(list, a)
	}
	return list, rows.Err()
}

// UpdateAuto changes an Auto's seats and note.
func (q Queries) UpdateAuto(ctx context.Context, id string, seats int, note string) error {
	res, err := q.db.ExecContext(ctx, `UPDATE auto SET seats = ?, note = ? WHERE id = ?`, seats, note, id)
	if err != nil {
		return fmt.Errorf("update auto: %w", err)
	}
	return expectOneRow(res, "update auto")
}

// DeleteAuto deletes an Auto with its passengers' seats.
func (q Queries) DeleteAuto(ctx context.Context, id string) error {
	res, err := q.db.ExecContext(ctx, `DELETE FROM auto WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("delete auto: %w", err)
	}
	return expectOneRow(res, "delete auto")
}

// ListUtasokOfEsemeny returns every seat taken in the Esemeny's Autok,
// ordered by the passenger's name. A name-only Vendeg's (#13) has an empty
// name until #13 joins vendeg here.
func (q Queries) ListUtasokOfEsemeny(ctx context.Context, esemenyID string) ([]domain.Utas, error) {
	rows, err := q.db.QueryContext(ctx,
		`SELECT u.auto_id, COALESCE(u.felhasznalo_id, ''), COALESCE(u.vendeg_id, ''), COALESCE(f.name, '')
		 FROM auto_utas u
		 LEFT JOIN felhasznalo f ON f.id = u.felhasznalo_id
		 WHERE u.esemeny_id = ?
		 ORDER BY COALESCE(f.name, ''), u.felhasznalo_id, u.vendeg_id`,
		esemenyID,
	)
	if err != nil {
		return nil, fmt.Errorf("list utasok: %w", err)
	}
	defer rows.Close()
	var list []domain.Utas
	for rows.Next() {
		var u domain.Utas
		if err := rows.Scan(&u.AutoID, &u.Holder.FelhasznaloID, &u.Holder.VendegID, &u.Name); err != nil {
			return nil, fmt.Errorf("list utasok: %w", err)
		}
		list = append(list, u)
	}
	return list, rows.Err()
}

// SetUles gives the holder a seat in the Auto, replacing any seat they hold
// in the Esemeny.
func (q Queries) SetUles(ctx context.Context, esemenyID, autoID string, h domain.ClaimHolder, at time.Time) error {
	f, v := holderColumns(h)
	conflict := `(esemeny_id, felhasznalo_id) WHERE felhasznalo_id IS NOT NULL`
	if h.VendegID != "" {
		conflict = `(esemeny_id, vendeg_id) WHERE vendeg_id IS NOT NULL`
	}
	_, err := q.db.ExecContext(ctx,
		`INSERT INTO auto_utas (auto_id, esemeny_id, felhasznalo_id, vendeg_id, created_at)
		 VALUES (?, ?, ?, ?, ?)
		 ON CONFLICT `+conflict+` DO UPDATE SET auto_id = excluded.auto_id, created_at = excluded.created_at`,
		autoID, esemenyID, f, v, formatTime(at),
	)
	if err != nil {
		return fmt.Errorf("set ules: %w", err)
	}
	return nil
}

// DeleteUles removes the holder's seat in the Esemeny, if they have one.
func (q Queries) DeleteUles(ctx context.Context, esemenyID string, h domain.ClaimHolder) error {
	f, v := holderColumns(h)
	_, err := q.db.ExecContext(ctx,
		`DELETE FROM auto_utas WHERE esemeny_id = ? AND felhasznalo_id IS ? AND vendeg_id IS ?`, esemenyID, f, v)
	if err != nil {
		return fmt.Errorf("delete ules: %w", err)
	}
	return nil
}

// ListSajatUton returns everyone marked saját úton in the Esemeny, ordered
// by name.
func (q Queries) ListSajatUton(ctx context.Context, esemenyID string) ([]domain.SajatUton, error) {
	rows, err := q.db.QueryContext(ctx,
		`SELECT COALESCE(s.felhasznalo_id, ''), COALESCE(s.vendeg_id, ''), COALESCE(f.name, '')
		 FROM sajat_uton s
		 LEFT JOIN felhasznalo f ON f.id = s.felhasznalo_id
		 WHERE s.esemeny_id = ?
		 ORDER BY COALESCE(f.name, ''), s.felhasznalo_id, s.vendeg_id`,
		esemenyID,
	)
	if err != nil {
		return nil, fmt.Errorf("list sajat uton: %w", err)
	}
	defer rows.Close()
	var list []domain.SajatUton
	for rows.Next() {
		var s domain.SajatUton
		if err := rows.Scan(&s.Holder.FelhasznaloID, &s.Holder.VendegID, &s.Name); err != nil {
			return nil, fmt.Errorf("list sajat uton: %w", err)
		}
		list = append(list, s)
	}
	return list, rows.Err()
}

// SetSajatUton marks the holder saját úton, or removes the mark.
func (q Queries) SetSajatUton(ctx context.Context, esemenyID string, h domain.ClaimHolder, on bool, at time.Time) error {
	f, v := holderColumns(h)
	if !on {
		_, err := q.db.ExecContext(ctx,
			`DELETE FROM sajat_uton WHERE esemeny_id = ? AND felhasznalo_id IS ? AND vendeg_id IS ?`, esemenyID, f, v)
		if err != nil {
			return fmt.Errorf("delete sajat uton: %w", err)
		}
		return nil
	}
	conflict := `(esemeny_id, felhasznalo_id) WHERE felhasznalo_id IS NOT NULL`
	if h.VendegID != "" {
		conflict = `(esemeny_id, vendeg_id) WHERE vendeg_id IS NOT NULL`
	}
	_, err := q.db.ExecContext(ctx,
		`INSERT INTO sajat_uton (esemeny_id, felhasznalo_id, vendeg_id, created_at)
		 VALUES (?, ?, ?, ?) ON CONFLICT `+conflict+` DO NOTHING`,
		esemenyID, f, v, formatTime(at),
	)
	if err != nil {
		return fmt.Errorf("set sajat uton: %w", err)
	}
	return nil
}
