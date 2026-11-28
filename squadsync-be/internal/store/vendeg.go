package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"squadsync-be/internal/domain"
)

// InsertVendeg stores a new Vendeg.
func (q Queries) InsertVendeg(ctx context.Context, v domain.Vendeg) error {
	var felhasznaloID, name, addedBy any
	if v.Kind == domain.VendegNev {
		name, addedBy = v.Name, v.AddedBy
	} else {
		felhasznaloID = v.FelhasznaloID
	}
	_, err := q.db.ExecContext(ctx,
		`INSERT INTO vendeg (id, esemeny_id, kind, felhasznalo_id, name, added_by, created_at) VALUES (?, ?, ?, ?, ?, ?, ?)`,
		v.ID, v.EsemenyID, string(v.Kind), felhasznaloID, name, addedBy, formatTime(v.CreatedAt),
	)
	if err != nil {
		return fmt.Errorf("insert vendeg: %w", err)
	}
	return nil
}

// vendegSelect reads Vendegek with the Felhasznalo Vendeg's name and the
// adding Tag's name filled in.
const vendegSelect = `SELECT v.id, v.esemeny_id, v.kind, COALESCE(v.felhasznalo_id, ''),
	COALESCE(v.name, f.name, ''), COALESCE(v.added_by, ''), COALESCE(a.name, ''), v.created_at
	FROM vendeg v
	LEFT JOIN felhasznalo f ON f.id = v.felhasznalo_id
	LEFT JOIN felhasznalo a ON a.id = v.added_by`

func scanVendeg(row rowScanner) (domain.Vendeg, error) {
	var v domain.Vendeg
	var kind, createdAt string
	if err := row.Scan(&v.ID, &v.EsemenyID, &kind, &v.FelhasznaloID, &v.Name, &v.AddedBy, &v.AddedByName, &createdAt); err != nil {
		return domain.Vendeg{}, err
	}
	v.Kind = domain.VendegKind(kind)
	var err error
	if v.CreatedAt, err = parseTime(createdAt); err != nil {
		return domain.Vendeg{}, err
	}
	return v, nil
}

func (q Queries) findVendeg(ctx context.Context, what, where string, args ...any) (domain.Vendeg, error) {
	v, err := scanVendeg(q.db.QueryRowContext(ctx, vendegSelect+` WHERE `+where, args...))
	if errors.Is(err, sql.ErrNoRows) {
		return domain.Vendeg{}, ErrNotFound
	}
	if err != nil {
		return domain.Vendeg{}, fmt.Errorf("%s: %w", what, err)
	}
	return v, nil
}

// FindVendeg returns the Vendeg with the given id, or ErrNotFound.
func (q Queries) FindVendeg(ctx context.Context, id string) (domain.Vendeg, error) {
	return q.findVendeg(ctx, "find vendeg", `v.id = ?`, id)
}

// FindVendegOfFelhasznalo returns the Felhasznalo's Vendeg of the Esemeny,
// or ErrNotFound if they aren't one.
func (q Queries) FindVendegOfFelhasznalo(ctx context.Context, esemenyID, felhasznaloID string) (domain.Vendeg, error) {
	return q.findVendeg(ctx, "find vendeg of felhasznalo", `v.esemeny_id = ? AND v.felhasznalo_id = ?`, esemenyID, felhasznaloID)
}

// ListVendegek returns the Esemeny's Vendegek, ordered by name.
func (q Queries) ListVendegek(ctx context.Context, esemenyID string) ([]domain.Vendeg, error) {
	rows, err := q.db.QueryContext(ctx,
		vendegSelect+` WHERE v.esemeny_id = ? ORDER BY COALESCE(v.name, f.name, ''), v.id`, esemenyID)
	if err != nil {
		return nil, fmt.Errorf("list vendegek: %w", err)
	}
	defer rows.Close()
	var list []domain.Vendeg
	for rows.Next() {
		v, err := scanVendeg(rows)
		if err != nil {
			return nil, fmt.Errorf("list vendegek: %w", err)
		}
		list = append(list, v)
	}
	return list, rows.Err()
}

// DeleteVendeg removes a Vendeg. A name-only Vendeg's Visszajelzesek,
// claims and confirmations go with them (ON DELETE CASCADE); a Kiadas share
// keeps them (the foreign key refuses the delete).
func (q Queries) DeleteVendeg(ctx context.Context, id string) error {
	if _, err := q.db.ExecContext(ctx, `DELETE FROM vendeg WHERE id = ?`, id); err != nil {
		return fmt.Errorf("delete vendeg: %w", err)
	}
	return nil
}

// DeleteVisszajelzesekOf removes the Felhasznalo's Visszajelzesek on every
// Idopont of the Esemeny.
func (q Queries) DeleteVisszajelzesekOf(ctx context.Context, esemenyID, felhasznaloID string) error {
	_, err := q.db.ExecContext(ctx,
		`DELETE FROM visszajelzes
		 WHERE felhasznalo_id = ? AND idopont_id IN (SELECT id FROM idopont WHERE esemeny_id = ?)`,
		felhasznaloID, esemenyID)
	if err != nil {
		return fmt.Errorf("delete visszajelzesek of: %w", err)
	}
	return nil
}

// HasKiadasOf tells whether the holder takes part in any Kiadas of the
// Esemeny: as a share in its split, or (a Felhasznalo) as its payer or
// creator.
func (q Queries) HasKiadasOf(ctx context.Context, esemenyID string, h domain.ClaimHolder) (bool, error) {
	f, v := holderColumns(h)
	var has bool
	err := q.db.QueryRowContext(ctx,
		`SELECT EXISTS (SELECT 1 FROM kiadas_resz r JOIN kiadas k ON k.id = r.kiadas_id
		                WHERE k.esemeny_id = ?1 AND r.felhasznalo_id IS ?2 AND r.vendeg_id IS ?3)
		     OR EXISTS (SELECT 1 FROM kiadas WHERE esemeny_id = ?1 AND (payer_id = ?2 OR created_by = ?2))`,
		esemenyID, f, v,
	).Scan(&has)
	if err != nil {
		return false, fmt.Errorf("has kiadas of: %w", err)
	}
	return has, nil
}

// InsertEsemenyMeghivo stores a new Esemeny-meghivo.
func (q Queries) InsertEsemenyMeghivo(ctx context.Context, m domain.EsemenyMeghivo) error {
	_, err := q.db.ExecContext(ctx,
		`INSERT INTO esemeny_meghivo (id, esemeny_id, token, created_by, created_at) VALUES (?, ?, ?, ?, ?)`,
		m.ID, m.EsemenyID, m.Token, m.CreatedBy, formatTime(m.CreatedAt),
	)
	if err != nil {
		return fmt.Errorf("insert esemeny-meghivo: %w", err)
	}
	return nil
}

const esemenyMeghivoColumns = `id, esemeny_id, token, created_by, created_at, revoked_at`

func scanEsemenyMeghivo(row rowScanner) (domain.EsemenyMeghivo, error) {
	var m domain.EsemenyMeghivo
	var createdAt string
	var revokedAt sql.NullString
	if err := row.Scan(&m.ID, &m.EsemenyID, &m.Token, &m.CreatedBy, &createdAt, &revokedAt); err != nil {
		return domain.EsemenyMeghivo{}, err
	}
	var err error
	if m.CreatedAt, err = parseTime(createdAt); err != nil {
		return domain.EsemenyMeghivo{}, err
	}
	if m.RevokedAt, err = parseNullTime(revokedAt); err != nil {
		return domain.EsemenyMeghivo{}, err
	}
	return m, nil
}

func (q Queries) findEsemenyMeghivo(ctx context.Context, column, value string) (domain.EsemenyMeghivo, error) {
	m, err := scanEsemenyMeghivo(q.db.QueryRowContext(ctx,
		`SELECT `+esemenyMeghivoColumns+` FROM esemeny_meghivo WHERE `+column+` = ?`, value))
	if errors.Is(err, sql.ErrNoRows) {
		return domain.EsemenyMeghivo{}, ErrNotFound
	}
	if err != nil {
		return domain.EsemenyMeghivo{}, fmt.Errorf("find esemeny-meghivo: %w", err)
	}
	return m, nil
}

// FindEsemenyMeghivo returns the Esemeny-meghivo with the given id, or
// ErrNotFound.
func (q Queries) FindEsemenyMeghivo(ctx context.Context, id string) (domain.EsemenyMeghivo, error) {
	return q.findEsemenyMeghivo(ctx, "id", id)
}

// FindEsemenyMeghivoByToken returns the Esemeny-meghivo with the given link
// token, revoked or not, or ErrNotFound.
func (q Queries) FindEsemenyMeghivoByToken(ctx context.Context, token string) (domain.EsemenyMeghivo, error) {
	return q.findEsemenyMeghivo(ctx, "token", token)
}

// ListActiveEsemenyMeghivok returns the Esemeny's Esemeny-meghivok that
// haven't been revoked, newest first.
func (q Queries) ListActiveEsemenyMeghivok(ctx context.Context, esemenyID string) ([]domain.EsemenyMeghivo, error) {
	rows, err := q.db.QueryContext(ctx,
		`SELECT `+esemenyMeghivoColumns+` FROM esemeny_meghivo
		 WHERE esemeny_id = ? AND revoked_at IS NULL
		 ORDER BY created_at DESC, id`,
		esemenyID,
	)
	if err != nil {
		return nil, fmt.Errorf("list esemeny-meghivok: %w", err)
	}
	defer rows.Close()
	var list []domain.EsemenyMeghivo
	for rows.Next() {
		m, err := scanEsemenyMeghivo(rows)
		if err != nil {
			return nil, fmt.Errorf("list esemeny-meghivok: %w", err)
		}
		list = append(list, m)
	}
	return list, rows.Err()
}

// RevokeEsemenyMeghivo marks the Esemeny-meghivo revoked at the given time.
// Revoking one that is already revoked keeps the first time.
func (q Queries) RevokeEsemenyMeghivo(ctx context.Context, id string, at time.Time) error {
	_, err := q.db.ExecContext(ctx,
		`UPDATE esemeny_meghivo SET revoked_at = ? WHERE id = ? AND revoked_at IS NULL`,
		formatTime(at), id,
	)
	if err != nil {
		return fmt.Errorf("revoke esemeny-meghivo: %w", err)
	}
	return nil
}
