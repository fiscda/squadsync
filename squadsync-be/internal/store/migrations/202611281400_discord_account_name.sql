-- The Discord name of each account, shown on the /fiok page and in the
-- message that refuses web login with a linked account (ADR-0002). It is a
-- snapshot from when the account was recorded, for display only.
ALTER TABLE discord_account ADD COLUMN name TEXT NOT NULL DEFAULT '';

-- Existing main accounts were recorded at first login, whose display name
-- became the Felhasznalo's name.
UPDATE discord_account
SET name = (SELECT f.name FROM felhasznalo f WHERE f.id = discord_account.felhasznalo_id)
WHERE is_main = 1;
