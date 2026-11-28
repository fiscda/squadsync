-- Elszamolas (#22): settling an Esemeny's Kiadasok once it is Lezajlott.

-- "Rögzítettem a kiadásaimat": who has confirmed their Kiadasok are
-- recorded. Rows are cleared when the Szervezo reopens the Elszamolas, so
-- everyone confirms again.
--
-- Exactly one of felhasznalo_id and vendeg_id is set, as in tetel_vallalas:
-- a name-only Vendeg (#13) is confirmed by their Tag (megerositette).
-- vendeg_id has no REFERENCES clause yet: #13 rebuilds this table to add
-- REFERENCES vendeg (id).
CREATE TABLE elszamolas_megerosites (
    esemeny_id     TEXT NOT NULL REFERENCES esemeny (id) ON DELETE CASCADE,
    felhasznalo_id TEXT REFERENCES felhasznalo (id),
    vendeg_id      TEXT, -- #13: REFERENCES vendeg (id)
    -- Who pressed it: the Felhasznalo themselves, or a name-only Vendeg's Tag.
    megerositette  TEXT NOT NULL REFERENCES felhasznalo (id),
    created_at     TEXT NOT NULL, -- RFC 3339, UTC
    CHECK ((felhasznalo_id IS NULL) <> (vendeg_id IS NULL))
) STRICT;

CREATE INDEX elszamolas_megerosites_esemeny_id ON elszamolas_megerosites (esemeny_id);
CREATE UNIQUE INDEX elszamolas_megerosites_felhasznalo
    ON elszamolas_megerosites (esemeny_id, felhasznalo_id) WHERE felhasznalo_id IS NOT NULL;
CREATE UNIQUE INDEX elszamolas_megerosites_vendeg
    ON elszamolas_megerosites (esemeny_id, vendeg_id) WHERE vendeg_id IS NOT NULL;

-- A calculated Elszamolas: at most one per Esemeny. While the row exists
-- the Kiadasok are locked; reopening deletes it (and its utalasok).
CREATE TABLE elszamolas (
    esemeny_id    TEXT PRIMARY KEY REFERENCES esemeny (id) ON DELETE CASCADE,
    kiszamolva_at TEXT NOT NULL, -- RFC 3339, UTC
    -- The Szervezo who closed it without every confirmation; NULL when the
    -- last confirmation calculated it.
    lezarta       TEXT REFERENCES felhasznalo (id)
) STRICT;

-- The transfers of a calculated Elszamolas: from_id pays to_id amount.
-- megvolt_at and megvolt_by are set together once either of them (or the
-- Szervezo) marks it done.
CREATE TABLE utalas (
    id          TEXT PRIMARY KEY,
    esemeny_id  TEXT NOT NULL REFERENCES elszamolas (esemeny_id) ON DELETE CASCADE,
    from_id     TEXT NOT NULL REFERENCES felhasznalo (id),
    to_id       TEXT NOT NULL REFERENCES felhasznalo (id),
    amount      INTEGER NOT NULL CHECK (amount >= 1), -- whole HUF
    -- The order the calculation produced them in.
    sorszam     INTEGER NOT NULL,
    megvolt_at  TEXT, -- RFC 3339, UTC
    megvolt_by  TEXT REFERENCES felhasznalo (id),
    CHECK (from_id <> to_id),
    CHECK ((megvolt_at IS NULL) = (megvolt_by IS NULL))
) STRICT;

CREATE INDEX utalas_esemeny_id ON utalas (esemeny_id);
