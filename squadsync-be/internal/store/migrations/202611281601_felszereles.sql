-- Felszereles: the things to bring to an Esemeny.
CREATE TABLE tetel (
    id             TEXT PRIMARY KEY,
    esemeny_id     TEXT NOT NULL REFERENCES esemeny (id) ON DELETE CASCADE,
    name           TEXT NOT NULL,
    quantity       INTEGER NOT NULL DEFAULT 1 CHECK (quantity >= 1),
    -- A reminder for everyone, never claimed.
    mindenki_hozza INTEGER NOT NULL DEFAULT 0 CHECK (mindenki_hozza IN (0, 1)),
    -- Who added it: they and the Szervezo may edit or delete it.
    added_by       TEXT NOT NULL REFERENCES felhasznalo (id),
    created_at     TEXT NOT NULL -- RFC 3339, UTC
) STRICT;

CREATE INDEX tetel_esemeny_id ON tetel (esemeny_id);

-- A Vallalas: part of a Tetel's quantity claimed by a Resztvevo
-- (felhasznalo_id), or by a Tag for their name-only Vendeg (vendeg_id, #13).
-- Exactly one of the two is set; until #13, vendeg_id is always NULL.
--
-- vendeg_id has no REFERENCES clause yet, like visszajelzes.vendeg_id:
-- SQLite refuses writes to a table whose foreign key names a missing table.
-- #13 rebuilds this table to add REFERENCES vendeg (id).
CREATE TABLE tetel_vallalas (
    tetel_id       TEXT NOT NULL REFERENCES tetel (id) ON DELETE CASCADE,
    felhasznalo_id TEXT REFERENCES felhasznalo (id),
    vendeg_id      TEXT, -- #13: REFERENCES vendeg (id)
    quantity       INTEGER NOT NULL CHECK (quantity >= 1),
    created_at     TEXT NOT NULL, -- RFC 3339, UTC
    updated_at     TEXT NOT NULL, -- RFC 3339, UTC
    CHECK ((felhasznalo_id IS NULL) <> (vendeg_id IS NULL))
) STRICT;

CREATE INDEX tetel_vallalas_tetel_id ON tetel_vallalas (tetel_id);
CREATE UNIQUE INDEX tetel_vallalas_tetel_felhasznalo
    ON tetel_vallalas (tetel_id, felhasznalo_id) WHERE felhasznalo_id IS NOT NULL;
CREATE UNIQUE INDEX tetel_vallalas_tetel_vendeg
    ON tetel_vallalas (tetel_id, vendeg_id) WHERE vendeg_id IS NOT NULL;
