-- Szallas: where the Resztvevok of an Esemeny sleep.
--
-- There is one Szallas per Esemeny, and it is just the Kiegeszito itself:
-- its one optional row holds a note about the place. Its Szobak (rooms) hold
-- Agyak (beds), and each Agy sleeps one or two people.
CREATE TABLE szallas (
    esemeny_id TEXT PRIMARY KEY REFERENCES esemeny (id) ON DELETE CASCADE,
    note       TEXT NOT NULL DEFAULT ''
) STRICT;

-- position orders the Szobak of an Esemeny, oldest first.
CREATE TABLE szoba (
    id         TEXT PRIMARY KEY,
    esemeny_id TEXT NOT NULL REFERENCES esemeny (id) ON DELETE CASCADE,
    name       TEXT NOT NULL,
    position   INTEGER NOT NULL,
    created_at TEXT NOT NULL -- RFC 3339, UTC
) STRICT;

CREATE INDEX szoba_esemeny_id ON szoba (esemeny_id);

-- esemeny_id repeats the Szoba's Esemeny, like auto_utas.esemeny_id.
CREATE TABLE agy (
    id         TEXT PRIMARY KEY,
    szoba_id   TEXT NOT NULL REFERENCES szoba (id) ON DELETE CASCADE,
    esemeny_id TEXT NOT NULL REFERENCES esemeny (id) ON DELETE CASCADE,
    capacity   INTEGER NOT NULL CHECK (capacity BETWEEN 1 AND 2),
    label      TEXT NOT NULL DEFAULT '',
    position   INTEGER NOT NULL,
    created_at TEXT NOT NULL -- RFC 3339, UTC
) STRICT;

CREATE INDEX agy_szoba_id ON agy (szoba_id);
CREATE INDEX agy_esemeny_id ON agy (esemeny_id);

-- A spot in an Agy, claimed by a Resztvevo (felhasznalo_id) or by a Tag for
-- their name-only Vendeg (vendeg_id, #13). Exactly one of the two is set;
-- until #13, vendeg_id is always NULL. One spot per person per Esemeny.
--
-- vendeg_id has no REFERENCES clause yet, like auto_utas.vendeg_id: #13
-- rebuilds this table to add REFERENCES vendeg (id).
CREATE TABLE agy_foglalas (
    agy_id         TEXT NOT NULL REFERENCES agy (id) ON DELETE CASCADE,
    esemeny_id     TEXT NOT NULL REFERENCES esemeny (id) ON DELETE CASCADE,
    felhasznalo_id TEXT REFERENCES felhasznalo (id),
    vendeg_id      TEXT, -- #13: REFERENCES vendeg (id)
    created_at     TEXT NOT NULL, -- RFC 3339, UTC
    CHECK ((felhasznalo_id IS NULL) <> (vendeg_id IS NULL))
) STRICT;

CREATE INDEX agy_foglalas_agy_id ON agy_foglalas (agy_id);
CREATE UNIQUE INDEX agy_foglalas_esemeny_felhasznalo
    ON agy_foglalas (esemeny_id, felhasznalo_id) WHERE felhasznalo_id IS NOT NULL;
CREATE UNIQUE INDEX agy_foglalas_esemeny_vendeg
    ON agy_foglalas (esemeny_id, vendeg_id) WHERE vendeg_id IS NOT NULL;

-- "Máshol alszik": a Resztvevo (or, #13, a name-only Vendeg) who sleeps
-- somewhere else and so needs no Agy. Same holder columns as agy_foglalas.
CREATE TABLE mashol_alszik (
    esemeny_id     TEXT NOT NULL REFERENCES esemeny (id) ON DELETE CASCADE,
    felhasznalo_id TEXT REFERENCES felhasznalo (id),
    vendeg_id      TEXT, -- #13: REFERENCES vendeg (id)
    created_at     TEXT NOT NULL, -- RFC 3339, UTC
    CHECK ((felhasznalo_id IS NULL) <> (vendeg_id IS NULL))
) STRICT;

CREATE UNIQUE INDEX mashol_alszik_esemeny_felhasznalo
    ON mashol_alszik (esemeny_id, felhasznalo_id) WHERE felhasznalo_id IS NOT NULL;
CREATE UNIQUE INDEX mashol_alszik_esemeny_vendeg
    ON mashol_alszik (esemeny_id, vendeg_id) WHERE vendeg_id IS NOT NULL;
