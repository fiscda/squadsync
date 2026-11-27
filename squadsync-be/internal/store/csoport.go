package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"squadsync-be/internal/domain"
)

// InsertCsoport stores a new Csoport.
func (q Queries) InsertCsoport(ctx context.Context, c domain.Csoport) error {
	_, err := q.db.ExecContext(ctx,
		`INSERT INTO csoport (id, name, created_at) VALUES (?, ?, ?)`,
		c.ID, c.Name, formatTime(c.CreatedAt),
	)
	if err != nil {
		return fmt.Errorf("insert csoport: %w", err)
	}
	return nil
}

// FindCsoport returns the Csoport with the given id, or ErrNotFound.
func (q Queries) FindCsoport(ctx context.Context, id string) (domain.Csoport, error) {
	var c domain.Csoport
	var createdAt string
	err := q.db.QueryRowContext(ctx,
		`SELECT id, name, created_at FROM csoport WHERE id = ?`, id,
	).Scan(&c.ID, &c.Name, &createdAt)
	if errors.Is(err, sql.ErrNoRows) {
		return domain.Csoport{}, ErrNotFound
	}
	if err != nil {
		return domain.Csoport{}, fmt.Errorf("find csoport: %w", err)
	}
	if c.CreatedAt, err = parseTime(createdAt); err != nil {
		return domain.Csoport{}, err
	}
	return c, nil
}

// InsertTag stores a new Tag.
func (q Queries) InsertTag(ctx context.Context, t domain.Tag) error {
	_, err := q.db.ExecContext(ctx,
		`INSERT INTO tag (csoport_id, felhasznalo_id, is_admin, created_at) VALUES (?, ?, ?, ?)`,
		t.CsoportID, t.FelhasznaloID, t.Admin, formatTime(t.CreatedAt),
	)
	if err != nil {
		return fmt.Errorf("insert tag: %w", err)
	}
	return nil
}

// FindTag returns the Felhasznalo's Tag in the Csoport, or ErrNotFound.
func (q Queries) FindTag(ctx context.Context, csoportID, felhasznaloID string) (domain.Tag, error) {
	t := domain.Tag{CsoportID: csoportID, FelhasznaloID: felhasznaloID}
	var createdAt string
	err := q.db.QueryRowContext(ctx,
		`SELECT is_admin, created_at FROM tag WHERE csoport_id = ? AND felhasznalo_id = ?`,
		csoportID, felhasznaloID,
	).Scan(&t.Admin, &createdAt)
	if errors.Is(err, sql.ErrNoRows) {
		return domain.Tag{}, ErrNotFound
	}
	if err != nil {
		return domain.Tag{}, fmt.Errorf("find tag: %w", err)
	}
	if t.CreatedAt, err = parseTime(createdAt); err != nil {
		return domain.Tag{}, err
	}
	return t, nil
}

// ListTags returns the Tags of a Csoport with their Felhasznalo's name:
// Admins first, then in the order they joined.
func (q Queries) ListTags(ctx context.Context, csoportID string) ([]domain.TagWithName, error) {
	rows, err := q.db.QueryContext(ctx,
		`SELECT t.felhasznalo_id, f.name, t.is_admin, t.created_at
		 FROM tag t JOIN felhasznalo f ON f.id = t.felhasznalo_id
		 WHERE t.csoport_id = ?
		 ORDER BY t.is_admin DESC, t.created_at, f.name, t.felhasznalo_id`,
		csoportID,
	)
	if err != nil {
		return nil, fmt.Errorf("list tags: %w", err)
	}
	defer rows.Close()

	var tags []domain.TagWithName
	for rows.Next() {
		t := domain.TagWithName{Tag: domain.Tag{CsoportID: csoportID}}
		var createdAt string
		if err := rows.Scan(&t.FelhasznaloID, &t.FelhasznaloName, &t.Admin, &createdAt); err != nil {
			return nil, fmt.Errorf("list tags: %w", err)
		}
		if t.CreatedAt, err = parseTime(createdAt); err != nil {
			return nil, err
		}
		tags = append(tags, t)
	}
	return tags, rows.Err()
}

// ListCsoportokOf returns every Csoport the Felhasznalo is a Tag of, in the
// order they became a Tag of them.
func (q Queries) ListCsoportokOf(ctx context.Context, felhasznaloID string) ([]domain.CsoportSummary, error) {
	rows, err := q.db.QueryContext(ctx,
		`SELECT c.id, c.name, c.created_at, t.is_admin,
		        (SELECT COUNT(*) FROM tag WHERE csoport_id = c.id)
		 FROM tag t JOIN csoport c ON c.id = t.csoport_id
		 WHERE t.felhasznalo_id = ?
		 ORDER BY t.created_at, c.id`,
		felhasznaloID,
	)
	if err != nil {
		return nil, fmt.Errorf("list csoportok: %w", err)
	}
	defer rows.Close()

	var csoportok []domain.CsoportSummary
	for rows.Next() {
		var s domain.CsoportSummary
		var createdAt string
		if err := rows.Scan(&s.ID, &s.Name, &createdAt, &s.Admin, &s.TagCount); err != nil {
			return nil, fmt.Errorf("list csoportok: %w", err)
		}
		if s.CreatedAt, err = parseTime(createdAt); err != nil {
			return nil, err
		}
		csoportok = append(csoportok, s)
	}
	return csoportok, rows.Err()
}

// InsertCsoportMeghivo stores a new Csoport-meghivo.
func (q Queries) InsertCsoportMeghivo(ctx context.Context, m domain.CsoportMeghivo) error {
	_, err := q.db.ExecContext(ctx,
		`INSERT INTO csoport_meghivo (id, csoport_id, token, created_by, created_at) VALUES (?, ?, ?, ?, ?)`,
		m.ID, m.CsoportID, m.Token, m.CreatedBy, formatTime(m.CreatedAt),
	)
	if err != nil {
		return fmt.Errorf("insert csoport-meghivo: %w", err)
	}
	return nil
}

const csoportMeghivoColumns = `id, csoport_id, token, created_by, created_at, revoked_at`

type rowScanner interface{ Scan(dest ...any) error }

func scanCsoportMeghivo(row rowScanner) (domain.CsoportMeghivo, error) {
	var m domain.CsoportMeghivo
	var createdAt string
	var revokedAt sql.NullString
	if err := row.Scan(&m.ID, &m.CsoportID, &m.Token, &m.CreatedBy, &createdAt, &revokedAt); err != nil {
		return domain.CsoportMeghivo{}, err
	}
	var err error
	if m.CreatedAt, err = parseTime(createdAt); err != nil {
		return domain.CsoportMeghivo{}, err
	}
	if revokedAt.Valid {
		t, err := parseTime(revokedAt.String)
		if err != nil {
			return domain.CsoportMeghivo{}, err
		}
		m.RevokedAt = &t
	}
	return m, nil
}

// FindCsoportMeghivo returns the Csoport-meghivo with the given id, or
// ErrNotFound.
func (q Queries) FindCsoportMeghivo(ctx context.Context, id string) (domain.CsoportMeghivo, error) {
	m, err := scanCsoportMeghivo(q.db.QueryRowContext(ctx,
		`SELECT `+csoportMeghivoColumns+` FROM csoport_meghivo WHERE id = ?`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return domain.CsoportMeghivo{}, ErrNotFound
	}
	if err != nil {
		return domain.CsoportMeghivo{}, fmt.Errorf("find csoport-meghivo: %w", err)
	}
	return m, nil
}

// FindCsoportMeghivoByToken returns the Csoport-meghivo with the given link
// token, revoked or not, or ErrNotFound.
func (q Queries) FindCsoportMeghivoByToken(ctx context.Context, token string) (domain.CsoportMeghivo, error) {
	m, err := scanCsoportMeghivo(q.db.QueryRowContext(ctx,
		`SELECT `+csoportMeghivoColumns+` FROM csoport_meghivo WHERE token = ?`, token))
	if errors.Is(err, sql.ErrNoRows) {
		return domain.CsoportMeghivo{}, ErrNotFound
	}
	if err != nil {
		return domain.CsoportMeghivo{}, fmt.Errorf("find csoport-meghivo by token: %w", err)
	}
	return m, nil
}

// ListActiveCsoportMeghivok returns the Csoport's Csoport-meghivok that
// haven't been revoked, newest first.
func (q Queries) ListActiveCsoportMeghivok(ctx context.Context, csoportID string) ([]domain.CsoportMeghivo, error) {
	rows, err := q.db.QueryContext(ctx,
		`SELECT `+csoportMeghivoColumns+` FROM csoport_meghivo
		 WHERE csoport_id = ? AND revoked_at IS NULL
		 ORDER BY created_at DESC, id`,
		csoportID,
	)
	if err != nil {
		return nil, fmt.Errorf("list csoport-meghivok: %w", err)
	}
	defer rows.Close()

	var meghivok []domain.CsoportMeghivo
	for rows.Next() {
		m, err := scanCsoportMeghivo(rows)
		if err != nil {
			return nil, fmt.Errorf("list csoport-meghivok: %w", err)
		}
		meghivok = append(meghivok, m)
	}
	return meghivok, rows.Err()
}

// RevokeCsoportMeghivo marks the Csoport-meghivo revoked at the given time.
// Revoking one that is already revoked keeps the first time.
func (q Queries) RevokeCsoportMeghivo(ctx context.Context, id string, at time.Time) error {
	_, err := q.db.ExecContext(ctx,
		`UPDATE csoport_meghivo SET revoked_at = ? WHERE id = ? AND revoked_at IS NULL`,
		formatTime(at), id,
	)
	if err != nil {
		return fmt.Errorf("revoke csoport-meghivo: %w", err)
	}
	return nil
}
