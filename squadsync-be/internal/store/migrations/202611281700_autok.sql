-- Autok: the cars offered for getting to an Esemeny.
--
-- An Auto is offered by a Resztvevo, its Sofor, with a number of free seats
-- (not counting the driver). A Resztvevo is the Sofor of at most one Auto
-- per Esemeny.
CREATE TABLE auto (
    id         TEXT PRIMARY KEY,
    esemeny_id TEXT NOT NULL REFERENCES esemeny (id) ON DELETE CASCADE,
    sofor_id   TEXT NOT NULL REFERENCES felhasznalo (id),
    seats      INTEGER NOT NULL CHECK (seats BETWEEN 1 AND 8),
    note       TEXT NOT NULL DEFAULT '',
    created_at TEXT NOT NULL -- RFC 3339, UTC
) STRICT;

CREATE UNIQUE INDEX auto_esemeny_sofor ON auto (esemeny_id, sofor_id);

-- A seat in an Auto, claimed by a Resztvevo (felhasznalo_id) or by a Tag for
-- their name-only Vendeg (vendeg_id, #13). Exactly one of the two is set;
-- until #13, vendeg_id is always NULL.
--
-- esemeny_id repeats the Auto's Esemeny so that the unique indexes below can
-- make "one seat per person per Esemeny" a rule of the database, not just of
-- the use cases.
--
-- vendeg_id has no REFERENCES clause yet, like tetel_vallalas.vendeg_id:
-- #13 rebuilds this table to add REFERENCES vendeg (id).
CREATE TABLE auto_utas (
    auto_id        TEXT NOT NULL REFERENCES auto (id) ON DELETE CASCADE,
    esemeny_id     TEXT NOT NULL REFERENCES esemeny (id) ON DELETE CASCADE,
    felhasznalo_id TEXT REFERENCES felhasznalo (id),
    vendeg_id      TEXT, -- #13: REFERENCES vendeg (id)
    created_at     TEXT NOT NULL, -- RFC 3339, UTC
    CHECK ((felhasznalo_id IS NULL) <> (vendeg_id IS NULL))
) STRICT;

CREATE INDEX auto_utas_auto_id ON auto_utas (auto_id);
CREATE UNIQUE INDEX auto_utas_esemeny_felhasznalo
    ON auto_utas (esemeny_id, felhasznalo_id) WHERE felhasznalo_id IS NOT NULL;
CREATE UNIQUE INDEX auto_utas_esemeny_vendeg
    ON auto_utas (esemeny_id, vendeg_id) WHERE vendeg_id IS NOT NULL;

-- "Saját úton": a Resztvevo (or, #13, a name-only Vendeg) who gets there on
-- their own and so needs no seat. Same holder columns as auto_utas.
CREATE TABLE sajat_uton (
    esemeny_id     TEXT NOT NULL REFERENCES esemeny (id) ON DELETE CASCADE,
    felhasznalo_id TEXT REFERENCES felhasznalo (id),
    vendeg_id      TEXT, -- #13: REFERENCES vendeg (id)
    created_at     TEXT NOT NULL, -- RFC 3339, UTC
    CHECK ((felhasznalo_id IS NULL) <> (vendeg_id IS NULL))
) STRICT;

CREATE UNIQUE INDEX sajat_uton_esemeny_felhasznalo
    ON sajat_uton (esemeny_id, felhasznalo_id) WHERE felhasznalo_id IS NOT NULL;
CREATE UNIQUE INDEX sajat_uton_esemeny_vendeg
    ON sajat_uton (esemeny_id, vendeg_id) WHERE vendeg_id IS NOT NULL;
