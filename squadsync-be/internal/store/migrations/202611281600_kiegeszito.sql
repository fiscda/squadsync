-- Which Kiegeszitok an Esemeny has. A row appears the first time the
-- Szervezo switches one on. Switching it off sets enabled = 0 and keeps the
-- row and every row of its data, so switching it back on restores them
-- unchanged. No row means off.
CREATE TABLE kiegeszito (
    esemeny_id TEXT NOT NULL REFERENCES esemeny (id) ON DELETE CASCADE,
    kind       TEXT NOT NULL
        CHECK (kind IN ('koltsegek', 'autok', 'szallas', 'felszereles', 'kaja', 'lejatszasi_lista')),
    enabled    INTEGER NOT NULL CHECK (enabled IN (0, 1)),
    updated_at TEXT NOT NULL, -- RFC 3339, UTC
    PRIMARY KEY (esemeny_id, kind)
) STRICT;
