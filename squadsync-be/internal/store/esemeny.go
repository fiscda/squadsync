package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"squadsync-be/internal/domain"
)

// InsertEsemeny stores a new Esemeny (without its Idopontok).
func (q Queries) InsertEsemeny(ctx context.Context, e domain.Esemeny) error {
	_, err := q.db.ExecContext(ctx,
		`INSERT INTO esemeny (id, csoport_id, title, description, helyszin_name, helyszin_url, szervezo_id, allapot, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		e.ID, e.CsoportID, e.Title, e.Description, e.Helyszin.Name, e.Helyszin.URL, e.SzervezoID, string(e.Allapot), formatTime(e.CreatedAt),
	)
	if err != nil {
		return fmt.Errorf("insert esemeny: %w", err)
	}
	return nil
}

const esemenyColumns = `id, csoport_id, title, description, helyszin_name, helyszin_url, szervezo_id, allapot, created_at`

func scanEsemeny(row rowScanner, extra ...any) (domain.Esemeny, error) {
	var e domain.Esemeny
	var allapot, createdAt string
	dest := append([]any{&e.ID, &e.CsoportID, &e.Title, &e.Description, &e.Helyszin.Name, &e.Helyszin.URL, &e.SzervezoID, &allapot, &createdAt}, extra...)
	if err := row.Scan(dest...); err != nil {
		return domain.Esemeny{}, err
	}
	e.Allapot = domain.Allapot(allapot)
	var err error
	if e.CreatedAt, err = parseTime(createdAt); err != nil {
		return domain.Esemeny{}, err
	}
	return e, nil
}

// FindEsemeny returns the Esemeny with the given id, or ErrNotFound.
func (q Queries) FindEsemeny(ctx context.Context, id string) (domain.Esemeny, error) {
	e, err := scanEsemeny(q.db.QueryRowContext(ctx,
		`SELECT `+esemenyColumns+` FROM esemeny WHERE id = ?`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return domain.Esemeny{}, ErrNotFound
	}
	if err != nil {
		return domain.Esemeny{}, fmt.Errorf("find esemeny: %w", err)
	}
	return e, nil
}

// ListEsemenyekOfCsoport returns a Csoport's Esemenyek, newest first, each
// with its number of Idopontok and its earliest start.
func (q Queries) ListEsemenyekOfCsoport(ctx context.Context, csoportID string) ([]domain.EsemenySummary, error) {
	rows, err := q.db.QueryContext(ctx,
		`SELECT `+esemenyColumns+`,
		        (SELECT COUNT(*) FROM idopont WHERE esemeny_id = esemeny.id),
		        (SELECT MIN(start_at) FROM idopont WHERE esemeny_id = esemeny.id)
		 FROM esemeny
		 WHERE csoport_id = ?
		 ORDER BY created_at DESC, id`,
		csoportID,
	)
	if err != nil {
		return nil, fmt.Errorf("list esemenyek: %w", err)
	}
	defer rows.Close()

	var list []domain.EsemenySummary
	for rows.Next() {
		var s domain.EsemenySummary
		var firstStart sql.NullString
		s.Esemeny, err = scanEsemeny(rows, &s.IdopontCount, &firstStart)
		if err != nil {
			return nil, fmt.Errorf("list esemenyek: %w", err)
		}
		if s.FirstStart, err = parseNullTime(firstStart); err != nil {
			return nil, err
		}
		list = append(list, s)
	}
	return list, rows.Err()
}

// InsertIdopont stores a new Idopont.
func (q Queries) InsertIdopont(ctx context.Context, i domain.Idopont) error {
	var end any
	if i.End != nil {
		end = formatTime(*i.End)
	}
	_, err := q.db.ExecContext(ctx,
		`INSERT INTO idopont (id, esemeny_id, start_at, end_at, added_by, created_at) VALUES (?, ?, ?, ?, ?, ?)`,
		i.ID, i.EsemenyID, formatTime(i.Start), end, i.AddedBy, formatTime(i.CreatedAt),
	)
	if err != nil {
		return fmt.Errorf("insert idopont: %w", err)
	}
	return nil
}

const idopontColumns = `id, esemeny_id, start_at, end_at, added_by, created_at`

func scanIdopont(row rowScanner) (domain.Idopont, error) {
	var i domain.Idopont
	var start, createdAt string
	var end sql.NullString
	if err := row.Scan(&i.ID, &i.EsemenyID, &start, &end, &i.AddedBy, &createdAt); err != nil {
		return domain.Idopont{}, err
	}
	var err error
	if i.Start, err = parseTime(start); err != nil {
		return domain.Idopont{}, err
	}
	if i.End, err = parseNullTime(end); err != nil {
		return domain.Idopont{}, err
	}
	if i.CreatedAt, err = parseTime(createdAt); err != nil {
		return domain.Idopont{}, err
	}
	return i, nil
}

// FindIdopont returns the Idopont with the given id, or ErrNotFound.
func (q Queries) FindIdopont(ctx context.Context, id string) (domain.Idopont, error) {
	i, err := scanIdopont(q.db.QueryRowContext(ctx,
		`SELECT `+idopontColumns+` FROM idopont WHERE id = ?`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return domain.Idopont{}, ErrNotFound
	}
	if err != nil {
		return domain.Idopont{}, fmt.Errorf("find idopont: %w", err)
	}
	return i, nil
}

// ListIdopontok returns an Esemeny's Idopontok in chronological order.
func (q Queries) ListIdopontok(ctx context.Context, esemenyID string) ([]domain.Idopont, error) {
	rows, err := q.db.QueryContext(ctx,
		`SELECT `+idopontColumns+` FROM idopont WHERE esemeny_id = ?
		 ORDER BY start_at, end_at IS NOT NULL, end_at, created_at, id`,
		esemenyID,
	)
	if err != nil {
		return nil, fmt.Errorf("list idopontok: %w", err)
	}
	defer rows.Close()

	var list []domain.Idopont
	for rows.Next() {
		i, err := scanIdopont(rows)
		if err != nil {
			return nil, fmt.Errorf("list idopontok: %w", err)
		}
		list = append(list, i)
	}
	return list, rows.Err()
}

// UpsertVisszajelzes stores a Felhasznalo's Visszajelzes on an Idopont,
// replacing their earlier one.
func (q Queries) UpsertVisszajelzes(ctx context.Context, v domain.Visszajelzes) error {
	at := formatTime(v.UpdatedAt)
	_, err := q.db.ExecContext(ctx,
		`INSERT INTO visszajelzes (idopont_id, felhasznalo_id, valasz, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?)
		 ON CONFLICT (idopont_id, felhasznalo_id) WHERE felhasznalo_id IS NOT NULL
		 DO UPDATE SET valasz = excluded.valasz, updated_at = excluded.updated_at`,
		v.IdopontID, v.FelhasznaloID, string(v.Valasz), at, at,
	)
	if err != nil {
		return fmt.Errorf("upsert visszajelzes: %w", err)
	}
	return nil
}

// ListVisszajelzesekOfEsemeny returns every Felhasznalo's Visszajelzes on the
// Esemeny's Idopontok, with their name and whether they are a Tag of the
// Esemeny's Csoport, ordered by name.
func (q Queries) ListVisszajelzesekOfEsemeny(ctx context.Context, esemenyID string) ([]domain.VisszajelzesOfEsemeny, error) {
	rows, err := q.db.QueryContext(ctx,
		`SELECT v.idopont_id, v.felhasznalo_id, v.valasz, v.updated_at, f.name,
		        EXISTS (SELECT 1 FROM tag t WHERE t.csoport_id = e.csoport_id AND t.felhasznalo_id = v.felhasznalo_id)
		 FROM visszajelzes v
		 JOIN idopont i ON i.id = v.idopont_id
		 JOIN esemeny e ON e.id = i.esemeny_id
		 JOIN felhasznalo f ON f.id = v.felhasznalo_id
		 WHERE i.esemeny_id = ?
		 ORDER BY f.name, v.felhasznalo_id`,
		esemenyID,
	)
	if err != nil {
		return nil, fmt.Errorf("list visszajelzesek: %w", err)
	}
	defer rows.Close()

	var list []domain.VisszajelzesOfEsemeny
	for rows.Next() {
		var v domain.VisszajelzesOfEsemeny
		var valasz, updatedAt string
		if err := rows.Scan(&v.IdopontID, &v.FelhasznaloID, &valasz, &updatedAt, &v.FelhasznaloName, &v.IsTag); err != nil {
			return nil, fmt.Errorf("list visszajelzesek: %w", err)
		}
		v.Valasz = domain.Valasz(valasz)
		if v.UpdatedAt, err = parseTime(updatedAt); err != nil {
			return nil, err
		}
		list = append(list, v)
	}
	return list, rows.Err()
}

func parseNullTime(s sql.NullString) (*time.Time, error) {
	if !s.Valid {
		return nil, nil
	}
	t, err := parseTime(s.String)
	if err != nil {
		return nil, err
	}
	return &t, nil
}
