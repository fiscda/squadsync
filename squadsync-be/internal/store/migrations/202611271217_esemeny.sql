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
    -- Who added it: the Szervezo may remove any Idopont, its adder only
    -- while nobody has answered it.
    added_by   TEXT NOT NULL REFERENCES felhasznalo (id),
    created_at TEXT NOT NULL  -- RFC 3339, UTC
) STRICT;

CREATE INDEX idopont_esemeny_id ON idopont (esemeny_id);

-- One answer to one Idopont. Today every row is a Felhasznalo's own answer.
-- felhasznalo_id is nullable so that name-only Vendegs (#13) can add a
-- vendeg_id column (ALTER TABLE ... ADD COLUMN vendeg_id TEXT REFERENCES
-- vendeg (id)) with its own partial unique index; the application core then
-- keeps exactly one of the two set.
CREATE TABLE visszajelzes (
    idopont_id     TEXT NOT NULL REFERENCES idopont (id) ON DELETE CASCADE,
    felhasznalo_id TEXT REFERENCES felhasznalo (id),
    valasz         TEXT NOT NULL CHECK (valasz IN ('igen', 'talan', 'nem')),
    created_at     TEXT NOT NULL, -- RFC 3339, UTC
    updated_at     TEXT NOT NULL  -- RFC 3339, UTC
) STRICT;

-- At most one Visszajelzes per Idopont and Felhasznalo.
CREATE UNIQUE INDEX visszajelzes_idopont_felhasznalo
    ON visszajelzes (idopont_id, felhasznalo_id) WHERE felhasznalo_id IS NOT NULL;
