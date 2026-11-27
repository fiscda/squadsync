-- Go-owned tables never use Better Auth's names (user, session, account,
-- verification), and Go never touches those tables (ADR-0004).

CREATE TABLE felhasznalo (
    -- Same value as Better Auth's user.id (ADR-0002).
    id         TEXT PRIMARY KEY,
    name       TEXT NOT NULL,
    created_at TEXT NOT NULL -- RFC 3339, UTC
) STRICT;

CREATE TABLE discord_account (
    discord_id     TEXT PRIMARY KEY,
    felhasznalo_id TEXT NOT NULL REFERENCES felhasznalo (id),
    is_main        INTEGER NOT NULL CHECK (is_main IN (0, 1)),
    created_at     TEXT NOT NULL -- RFC 3339, UTC
) STRICT;

CREATE INDEX discord_account_felhasznalo_id ON discord_account (felhasznalo_id);

-- At most one main Discord account per Felhasznalo.
CREATE UNIQUE INDEX discord_account_one_main
    ON discord_account (felhasznalo_id) WHERE is_main = 1;
