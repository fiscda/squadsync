-- Koltsegek: the Kiadasok of an Esemeny.
CREATE TABLE kiadas (
    id          TEXT PRIMARY KEY,
    esemeny_id  TEXT NOT NULL REFERENCES esemeny (id) ON DELETE CASCADE,
    description TEXT NOT NULL,
    -- Whole HUF: the estimate while tervezett, the real amount once kifizetve.
    amount      INTEGER NOT NULL CHECK (amount >= 1),
    payer_id    TEXT NOT NULL REFERENCES felhasznalo (id),
    allapot     TEXT NOT NULL CHECK (allapot IN ('tervezett', 'kifizetve')),
    -- 1: split equally (the shares are recomputed when the amount or the
    -- split changes); 0: custom amounts per person.
    egyenlo     INTEGER NOT NULL CHECK (egyenlo IN (0, 1)),
    -- Who recorded it: they, the payer and the Szervezo may edit or delete it.
    created_by  TEXT NOT NULL REFERENCES felhasznalo (id),
    created_at  TEXT NOT NULL, -- RFC 3339, UTC
    updated_at  TEXT NOT NULL  -- RFC 3339, UTC
) STRICT;

CREATE INDEX kiadas_esemeny_id ON kiadas (esemeny_id);

-- A Kiadas's split: one row per person it is split among, with their share
-- in whole HUF. The shares of a Kiadas add up to its amount. The rows are
-- fixed when it is created and change only when someone edits the Kiadas:
-- drop-outs never touch them. The Elszamolas (#22) sums them.
--
-- Exactly one of felhasznalo_id and vendeg_id is set, as in tetel_vallalas.
-- A name-only Vendeg's (#13) share is charged to the Tag who added them.
-- vendeg_id has no REFERENCES clause yet: #13 rebuilds this table to add
-- REFERENCES vendeg (id).
CREATE TABLE kiadas_resz (
    kiadas_id      TEXT NOT NULL REFERENCES kiadas (id) ON DELETE CASCADE,
    felhasznalo_id TEXT REFERENCES felhasznalo (id),
    vendeg_id      TEXT, -- #13: REFERENCES vendeg (id)
    amount         INTEGER NOT NULL CHECK (amount >= 0),
    CHECK ((felhasznalo_id IS NULL) <> (vendeg_id IS NULL))
) STRICT;

CREATE INDEX kiadas_resz_kiadas_id ON kiadas_resz (kiadas_id);
CREATE UNIQUE INDEX kiadas_resz_kiadas_felhasznalo
    ON kiadas_resz (kiadas_id, felhasznalo_id) WHERE felhasznalo_id IS NOT NULL;
CREATE UNIQUE INDEX kiadas_resz_kiadas_vendeg
    ON kiadas_resz (kiadas_id, vendeg_id) WHERE vendeg_id IS NOT NULL;
