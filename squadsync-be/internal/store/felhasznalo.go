package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

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
		`SELECT discord_id, felhasznalo_id, is_main, created_at FROM discord_account WHERE discord_id = ?`,
		discordID,
	).Scan(&a.DiscordID, &a.FelhasznaloID, &a.IsMain, &createdAt)
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
		`SELECT discord_id, felhasznalo_id, is_main, created_at FROM discord_account
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
		if err := rows.Scan(&a.DiscordID, &a.FelhasznaloID, &a.IsMain, &createdAt); err != nil {
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
		`INSERT INTO discord_account (discord_id, felhasznalo_id, is_main, created_at) VALUES (?, ?, ?, ?)`,
		a.DiscordID, a.FelhasznaloID, a.IsMain, formatTime(a.CreatedAt),
	)
	if err != nil {
		return fmt.Errorf("insert discord account: %w", err)
	}
	return nil
}
