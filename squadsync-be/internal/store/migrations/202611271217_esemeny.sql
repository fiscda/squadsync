CREATE TABLE esemeny (
    id               TEXT PRIMARY KEY,
    csoport_id       TEXT NOT NULL REFERENCES csoport (id),
    title            TEXT NOT NULL,
    description      TEXT NOT NULL, -- '' when none
    helyszin_name    TEXT NOT NULL, -- '' when none
    helyszin_url     TEXT NOT NULL, -- '' when none; an absolute http(s) URL
    szervezo_id      TEXT NOT NULL REFERENCES felhasznalo (id),
    allapot          TEXT NOT NULL
        CHECK (allapot IN ('egyeztetes', 'veglegesitve', 'elmaradt', 'lezajlott')),
    created_at       TEXT NOT NULL -- RFC 3339, UTC
) STRICT;

CREATE INDEX esemeny_csoport_id ON esemeny (csoport_id);

CREATE TABLE idopont (
    id         TEXT PRIMARY KEY,
    esemeny_id TEXT NOT NULL REFERENCES esemeny (id) ON DELETE CASCADE,
    start_at   TEXT NOT NULL, -- RFC 3339, UTC
    end_at     TEXT,          -- RFC 3339, UTC; NULL when it has only a start
    -- A whole-day Idopont runs from Budapest midnight of its first day
    -- (start_at) to Budapest midnight after its last day (end_at, never NULL).
    egesz_napos INTEGER NOT NULL DEFAULT 0 CHECK (egesz_napos IN (0, 1)),
    -- Who added it: the Szervezo may remove any Idopont, its adder only
    -- while nobody has answered it.
    added_by   TEXT NOT NULL REFERENCES felhasznalo (id),
    created_at TEXT NOT NULL  -- RFC 3339, UTC
) STRICT;

CREATE INDEX idopont_esemeny_id ON idopont (esemeny_id);

-- One answer to one Idopont, given either by a Felhasznalo for themselves
-- (felhasznalo_id) or by a Tag for a name-only Vendeg (vendeg_id, #13).
-- Exactly one of the two is set; until #13, vendeg_id is always NULL.
--
-- vendeg_id has no REFERENCES clause yet: SQLite refuses every write to a
-- table whose foreign key names a missing parent table, even with a NULL
-- value. #13 creates the vendeg table and rebuilds this one to add
-- REFERENCES vendeg (id) (PRAGMA defer_foreign_keys = ON, per CLAUDE.md).
CREATE TABLE visszajelzes (
    idopont_id     TEXT NOT NULL REFERENCES idopont (id) ON DELETE CASCADE,
    felhasznalo_id TEXT REFERENCES felhasznalo (id),
    vendeg_id      TEXT, -- #13: REFERENCES vendeg (id)
    valasz         TEXT NOT NULL CHECK (valasz IN ('igen', 'talan', 'nem')),
    created_at     TEXT NOT NULL, -- RFC 3339, UTC
    updated_at     TEXT NOT NULL, -- RFC 3339, UTC
    CHECK ((felhasznalo_id IS NULL) <> (vendeg_id IS NULL))
) STRICT;

CREATE INDEX visszajelzes_idopont_id ON visszajelzes (idopont_id);

-- At most one Visszajelzes per Idopont and Felhasznalo, and per Idopont and
-- Vendeg.
CREATE UNIQUE INDEX visszajelzes_idopont_felhasznalo
    ON visszajelzes (idopont_id, felhasznalo_id) WHERE felhasznalo_id IS NOT NULL;
CREATE UNIQUE INDEX visszajelzes_idopont_vendeg
    ON visszajelzes (idopont_id, vendeg_id) WHERE vendeg_id IS NOT NULL;
