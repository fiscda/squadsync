-- Vendegek (#13): people who attend one Esemeny without being a Tag of its
-- Csoport.
--
-- A Vendeg is either a Felhasznalo who joined through an Esemeny-meghivo
-- (kind 'felhasznalo', felhasznalo_id set), or just a name added by a Tag
-- (kind 'nev', name and added_by set). A Felhasznalo Vendeg answers and
-- claims as themselves, with their felhasznalo_id, like a Tag; a name-only
-- Vendeg's Visszajelzesek and claims carry vendeg_id, and their Tag
-- (added_by) answers and claims for them.
CREATE TABLE vendeg (
    id             TEXT PRIMARY KEY,
    esemeny_id     TEXT NOT NULL REFERENCES esemeny (id) ON DELETE CASCADE,
    kind           TEXT NOT NULL CHECK (kind IN ('felhasznalo', 'nev')),
    felhasznalo_id TEXT REFERENCES felhasznalo (id),
    name           TEXT,
    added_by       TEXT REFERENCES felhasznalo (id),
    created_at     TEXT NOT NULL, -- RFC 3339, UTC
    CHECK ((kind = 'felhasznalo') = (felhasznalo_id IS NOT NULL)),
    CHECK ((kind = 'nev') = (name IS NOT NULL)),
    CHECK ((kind = 'nev') = (added_by IS NOT NULL))
) STRICT;

CREATE INDEX vendeg_esemeny_id ON vendeg (esemeny_id);
-- A Felhasznalo is a Vendeg of an Esemeny at most once.
CREATE UNIQUE INDEX vendeg_esemeny_felhasznalo
    ON vendeg (esemeny_id, felhasznalo_id) WHERE felhasznalo_id IS NOT NULL;

-- An Esemeny-meghivo: a reusable link that makes whoever opens it and logs
-- in a Vendeg of one Esemeny. Any Tag creates or revokes one; it also stops
-- working once the Esemeny is Lezajlott or Elmaradt (checked by the core).
CREATE TABLE esemeny_meghivo (
    id         TEXT PRIMARY KEY,
    esemeny_id TEXT NOT NULL REFERENCES esemeny (id) ON DELETE CASCADE,
    token      TEXT NOT NULL UNIQUE,
    created_by TEXT NOT NULL REFERENCES felhasznalo (id),
    created_at TEXT NOT NULL, -- RFC 3339, UTC
    revoked_at TEXT           -- RFC 3339, UTC; NULL while it works
) STRICT;

CREATE INDEX esemeny_meghivo_esemeny_id ON esemeny_meghivo (esemeny_id);
