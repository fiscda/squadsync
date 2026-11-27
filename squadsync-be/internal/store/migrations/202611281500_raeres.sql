-- A Felhasznalo's Raeres: per Budapest calendar day, szabad or foglalt.
-- An unmarked day is unknown and has no row. It is shared across all the
-- Felhasznalo's Csoportok, so it hangs off the Felhasznalo, not a Tag.
CREATE TABLE raeres (
    felhasznalo_id TEXT NOT NULL REFERENCES felhasznalo (id),
    day            TEXT NOT NULL
        CHECK (day GLOB '[0-9][0-9][0-9][0-9]-[0-9][0-9]-[0-9][0-9]'), -- a Budapest date, YYYY-MM-DD
    allapot        TEXT NOT NULL CHECK (allapot IN ('szabad', 'foglalt')),
    PRIMARY KEY (felhasznalo_id, day)
) STRICT, WITHOUT ROWID;
