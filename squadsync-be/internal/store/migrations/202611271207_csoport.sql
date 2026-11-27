CREATE TABLE csoport (
    id         TEXT PRIMARY KEY,
    name       TEXT NOT NULL,
    created_at TEXT NOT NULL -- RFC 3339, UTC
) STRICT;

-- A Tag is one Felhasznalo's membership in one Csoport. It is its own table
-- so later tickets can add per-Tag settings (such as DM muting) as columns.
CREATE TABLE tag (
    csoport_id     TEXT NOT NULL REFERENCES csoport (id),
    felhasznalo_id TEXT NOT NULL REFERENCES felhasznalo (id),
    is_admin       INTEGER NOT NULL CHECK (is_admin IN (0, 1)),
    created_at     TEXT NOT NULL, -- RFC 3339, UTC
    PRIMARY KEY (csoport_id, felhasznalo_id)
) STRICT;

CREATE INDEX tag_felhasznalo_id ON tag (felhasznalo_id);

CREATE TABLE csoport_meghivo (
    id         TEXT PRIMARY KEY,
    csoport_id TEXT NOT NULL REFERENCES csoport (id),
    -- Unguessable random secret in the invite link.
    token      TEXT NOT NULL UNIQUE,
    created_by TEXT NOT NULL REFERENCES felhasznalo (id),
    created_at TEXT NOT NULL, -- RFC 3339, UTC
    -- NULL while the link works. There is no expiry.
    revoked_at TEXT
) STRICT;

CREATE INDEX csoport_meghivo_csoport_id ON csoport_meghivo (csoport_id);
