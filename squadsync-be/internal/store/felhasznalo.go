package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"squadsync-be/internal/domain"
)

// FindFelhasznalo returns the Felhasznalo with the given id, or ErrNotFound.
func (q Queries) FindFelhasznalo(ctx context.Context, id string) (domain.Felhasznalo, error) {
	var f domain.Felhasznalo
	var createdAt string
	err := q.db.QueryRowContext(ctx,
		`SELECT id, name, created_at FROM felhasznalo WHERE id = ?`, id,
	).Scan(&f.ID, &f.Name, &createdAt)
	if errors.Is(err, sql.ErrNoRows) {
		return domain.Felhasznalo{}, ErrNotFound
	}
	if err != nil {
		return domain.Felhasznalo{}, fmt.Errorf("find felhasznalo: %w", err)
	}
	if f.CreatedAt, err = parseTime(createdAt); err != nil {
		return domain.Felhasznalo{}, err
	}
	return f, nil
}

// InsertFelhasznalo stores a new Felhasznalo.
func (q Queries) InsertFelhasznalo(ctx context.Context, f domain.Felhasznalo) error {
	_, err := q.db.ExecContext(ctx,
		`INSERT INTO felhasznalo (id, name, created_at) VALUES (?, ?, ?)`,
		f.ID, f.Name, formatTime(f.CreatedAt),
	)
	if err != nil {
		return fmt.Errorf("insert felhasznalo: %w", err)
	}
	return nil
}

// FindDiscordAccount returns the Discord account with the given Discord user
// id, or ErrNotFound.
func (q Queries) FindDiscordAccount(ctx context.Context, discordID string) (domain.DiscordAccount, error) {
	var a domain.DiscordAccount
	var createdAt string
	err := q.db.QueryRowContext(ctx,
		`SELECT discord_id, felhasznalo_id, is_main, name, created_at FROM discord_account WHERE discord_id = ?`,
		discordID,
	).Scan(&a.DiscordID, &a.FelhasznaloID, &a.IsMain, &a.Name, &createdAt)
	if errors.Is(err, sql.ErrNoRows) {
		return domain.DiscordAccount{}, ErrNotFound
	}
	if err != nil {
		return domain.DiscordAccount{}, fmt.Errorf("find discord account: %w", err)
	}
	if a.CreatedAt, err = parseTime(createdAt); err != nil {
		return domain.DiscordAccount{}, err
	}
	return a, nil
}

// ListDiscordAccounts returns every Discord account of a Felhasznalo, the main
// one first.
func (q Queries) ListDiscordAccounts(ctx context.Context, felhasznaloID string) ([]domain.DiscordAccount, error) {
	rows, err := q.db.QueryContext(ctx,
		`SELECT discord_id, felhasznalo_id, is_main, name, created_at FROM discord_account
		 WHERE felhasznalo_id = ? ORDER BY is_main DESC, created_at, discord_id`,
		felhasznaloID,
	)
	if err != nil {
		return nil, fmt.Errorf("list discord accounts: %w", err)
	}
	defer rows.Close()

	var accounts []domain.DiscordAccount
	for rows.Next() {
		var a domain.DiscordAccount
		var createdAt string
		if err := rows.Scan(&a.DiscordID, &a.FelhasznaloID, &a.IsMain, &a.Name, &createdAt); err != nil {
			return nil, fmt.Errorf("list discord accounts: %w", err)
		}
		if a.CreatedAt, err = parseTime(createdAt); err != nil {
			return nil, err
		}
		accounts = append(accounts, a)
	}
	return accounts, rows.Err()
}

// InsertDiscordAccount stores a Discord account of a Felhasznalo.
func (q Queries) InsertDiscordAccount(ctx context.Context, a domain.DiscordAccount) error {
	_, err := q.db.ExecContext(ctx,
		`INSERT INTO discord_account (discord_id, felhasznalo_id, is_main, name, created_at) VALUES (?, ?, ?, ?, ?)`,
		a.DiscordID, a.FelhasznaloID, a.IsMain, a.Name, formatTime(a.CreatedAt),
	)
	if err != nil {
		return fmt.Errorf("insert discord account: %w", err)
	}
	return nil
}

// MoveDiscordAccount makes an existing Discord account a linked (non-main)
// account of another Felhasznalo, with a fresh name and creation time.
func (q Queries) MoveDiscordAccount(ctx context.Context, a domain.DiscordAccount) error {
	res, err := q.db.ExecContext(ctx,
		`UPDATE discord_account SET felhasznalo_id = ?, is_main = ?, name = ?, created_at = ? WHERE discord_id = ?`,
		a.FelhasznaloID, a.IsMain, a.Name, formatTime(a.CreatedAt), a.DiscordID,
	)
	if err != nil {
		return fmt.Errorf("move discord account: %w", err)
	}
	return expectOneRow(res, "move discord account")
}

// DeleteDiscordAccount removes a Discord account.
func (q Queries) DeleteDiscordAccount(ctx context.Context, discordID string) error {
	res, err := q.db.ExecContext(ctx, `DELETE FROM discord_account WHERE discord_id = ?`, discordID)
	if err != nil {
		return fmt.Errorf("delete discord account: %w", err)
	}
	return expectOneRow(res, "delete discord account")
}

// DeleteFelhasznalo removes a Felhasznalo. It fails with a foreign key error
// while any row still refers to them, so delete their Discord accounts first.
func (q Queries) DeleteFelhasznalo(ctx context.Context, id string) error {
	res, err := q.db.ExecContext(ctx, `DELETE FROM felhasznalo WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("delete felhasznalo: %w", err)
	}
	return expectOneRow(res, "delete felhasznalo")
}

// FelhasznaloHasData reports whether any Go-owned row other than their
// Discord accounts refers to the Felhasznalo: a Tag, a Csoport-meghivo they
// made, anything at all.
//
// It finds the referring columns in the schema itself (every foreign key to
// felhasznalo), so tables added by later migrations count without changing
// this query. A new table that refers to a Felhasznalo must therefore declare
// its foreign key.
func (q Queries) FelhasznaloHasData(ctx context.Context, id string) (bool, error) {
	rows, err := q.db.QueryContext(ctx, `
		SELECT m.name, fk."from"
		FROM sqlite_schema AS m, pragma_foreign_key_list(m.name) AS fk
		WHERE m.type = 'table' AND fk."table" = 'felhasznalo' AND m.name <> 'discord_account'
		ORDER BY m.name, fk."from"`)
	if err != nil {
		return false, fmt.Errorf("list references to felhasznalo: %w", err)
	}
	type ref struct{ table, column string }
	var refs []ref
	for rows.Next() {
		var r ref
		if err := rows.Scan(&r.table, &r.column); err != nil {
			rows.Close()
			return false, fmt.Errorf("list references to felhasznalo: %w", err)
		}
		refs = append(refs, r)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return false, fmt.Errorf("list references to felhasznalo: %w", err)
	}

	for _, r := range refs {
		var found bool
		// The names come from the schema, not from input; quote them anyway.
		query := fmt.Sprintf(`SELECT EXISTS (SELECT 1 FROM %s WHERE %s = ?)`, quoteIdent(r.table), quoteIdent(r.column))
		if err := q.db.QueryRowContext(ctx, query, id).Scan(&found); err != nil {
			return false, fmt.Errorf("check %s.%s: %w", r.table, r.column, err)
		}
		if found {
			return true, nil
		}
	}
	return false, nil
}

func quoteIdent(name string) string {
	return `"` + strings.ReplaceAll(name, `"`, `""`) + `"`
}

func expectOneRow(res sql.Result, what string) error {
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("%s: %w", what, err)
	}
	if n == 0 {
		return ErrNotFound
	}
	return nil
}
