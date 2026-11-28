-- Every table with a vendeg_id column (#13) gets its REFERENCES vendeg (id),
-- now that the vendeg table exists. SQLite can't add a foreign key to an
-- existing table, so each one is rebuilt: create the new table, copy the
-- rows, drop the old one, rename, and recreate its indexes under the same
-- names. Removing a name-only Vendeg removes their Visszajelzesek, claims
-- and confirmations with them (ON DELETE CASCADE), but not their Kiadas
-- shares: the core refuses to remove a Vendeg who has one, so the shares
-- of a Kiadas keep adding up to its amount.
PRAGMA defer_foreign_keys = ON;

-- visszajelzes (202611271217_esemeny.sql)
CREATE TABLE visszajelzes_new (
    idopont_id     TEXT NOT NULL REFERENCES idopont (id) ON DELETE CASCADE,
    felhasznalo_id TEXT REFERENCES felhasznalo (id),
    vendeg_id      TEXT REFERENCES vendeg (id) ON DELETE CASCADE,
    valasz         TEXT NOT NULL CHECK (valasz IN ('igen', 'talan', 'nem')),
    created_at     TEXT NOT NULL, -- RFC 3339, UTC
    updated_at     TEXT NOT NULL, -- RFC 3339, UTC
    CHECK ((felhasznalo_id IS NULL) <> (vendeg_id IS NULL))
) STRICT;
INSERT INTO visszajelzes_new (idopont_id, felhasznalo_id, vendeg_id, valasz, created_at, updated_at)
    SELECT idopont_id, felhasznalo_id, vendeg_id, valasz, created_at, updated_at FROM visszajelzes;
DROP TABLE visszajelzes;
ALTER TABLE visszajelzes_new RENAME TO visszajelzes;
CREATE INDEX visszajelzes_idopont_id ON visszajelzes (idopont_id);
CREATE UNIQUE INDEX visszajelzes_idopont_felhasznalo
    ON visszajelzes (idopont_id, felhasznalo_id) WHERE felhasznalo_id IS NOT NULL;
CREATE UNIQUE INDEX visszajelzes_idopont_vendeg
    ON visszajelzes (idopont_id, vendeg_id) WHERE vendeg_id IS NOT NULL;

-- tetel_vallalas (202611281601_felszereles.sql)
CREATE TABLE tetel_vallalas_new (
    tetel_id       TEXT NOT NULL REFERENCES tetel (id) ON DELETE CASCADE,
    felhasznalo_id TEXT REFERENCES felhasznalo (id),
    vendeg_id      TEXT REFERENCES vendeg (id) ON DELETE CASCADE,
    quantity       INTEGER NOT NULL CHECK (quantity >= 1),
    created_at     TEXT NOT NULL, -- RFC 3339, UTC
    updated_at     TEXT NOT NULL, -- RFC 3339, UTC
    CHECK ((felhasznalo_id IS NULL) <> (vendeg_id IS NULL))
) STRICT;
INSERT INTO tetel_vallalas_new (tetel_id, felhasznalo_id, vendeg_id, quantity, created_at, updated_at)
    SELECT tetel_id, felhasznalo_id, vendeg_id, quantity, created_at, updated_at FROM tetel_vallalas;
DROP TABLE tetel_vallalas;
ALTER TABLE tetel_vallalas_new RENAME TO tetel_vallalas;
CREATE INDEX tetel_vallalas_tetel_id ON tetel_vallalas (tetel_id);
CREATE UNIQUE INDEX tetel_vallalas_tetel_felhasznalo
    ON tetel_vallalas (tetel_id, felhasznalo_id) WHERE felhasznalo_id IS NOT NULL;
CREATE UNIQUE INDEX tetel_vallalas_tetel_vendeg
    ON tetel_vallalas (tetel_id, vendeg_id) WHERE vendeg_id IS NOT NULL;

-- auto_utas (202611281700_autok.sql)
CREATE TABLE auto_utas_new (
    auto_id        TEXT NOT NULL REFERENCES auto (id) ON DELETE CASCADE,
    esemeny_id     TEXT NOT NULL REFERENCES esemeny (id) ON DELETE CASCADE,
    felhasznalo_id TEXT REFERENCES felhasznalo (id),
    vendeg_id      TEXT REFERENCES vendeg (id) ON DELETE CASCADE,
    created_at     TEXT NOT NULL, -- RFC 3339, UTC
    CHECK ((felhasznalo_id IS NULL) <> (vendeg_id IS NULL))
) STRICT;
INSERT INTO auto_utas_new (auto_id, esemeny_id, felhasznalo_id, vendeg_id, created_at)
    SELECT auto_id, esemeny_id, felhasznalo_id, vendeg_id, created_at FROM auto_utas;
DROP TABLE auto_utas;
ALTER TABLE auto_utas_new RENAME TO auto_utas;
CREATE INDEX auto_utas_auto_id ON auto_utas (auto_id);
CREATE UNIQUE INDEX auto_utas_esemeny_felhasznalo
    ON auto_utas (esemeny_id, felhasznalo_id) WHERE felhasznalo_id IS NOT NULL;
CREATE UNIQUE INDEX auto_utas_esemeny_vendeg
    ON auto_utas (esemeny_id, vendeg_id) WHERE vendeg_id IS NOT NULL;

-- sajat_uton (202611281700_autok.sql)
CREATE TABLE sajat_uton_new (
    esemeny_id     TEXT NOT NULL REFERENCES esemeny (id) ON DELETE CASCADE,
    felhasznalo_id TEXT REFERENCES felhasznalo (id),
    vendeg_id      TEXT REFERENCES vendeg (id) ON DELETE CASCADE,
    created_at     TEXT NOT NULL, -- RFC 3339, UTC
    CHECK ((felhasznalo_id IS NULL) <> (vendeg_id IS NULL))
) STRICT;
INSERT INTO sajat_uton_new (esemeny_id, felhasznalo_id, vendeg_id, created_at)
    SELECT esemeny_id, felhasznalo_id, vendeg_id, created_at FROM sajat_uton;
DROP TABLE sajat_uton;
ALTER TABLE sajat_uton_new RENAME TO sajat_uton;
CREATE UNIQUE INDEX sajat_uton_esemeny_felhasznalo
    ON sajat_uton (esemeny_id, felhasznalo_id) WHERE felhasznalo_id IS NOT NULL;
CREATE UNIQUE INDEX sajat_uton_esemeny_vendeg
    ON sajat_uton (esemeny_id, vendeg_id) WHERE vendeg_id IS NOT NULL;

-- agy_foglalas (202611281800_szallas.sql)
CREATE TABLE agy_foglalas_new (
    agy_id         TEXT NOT NULL REFERENCES agy (id) ON DELETE CASCADE,
    esemeny_id     TEXT NOT NULL REFERENCES esemeny (id) ON DELETE CASCADE,
    felhasznalo_id TEXT REFERENCES felhasznalo (id),
    vendeg_id      TEXT REFERENCES vendeg (id) ON DELETE CASCADE,
    created_at     TEXT NOT NULL, -- RFC 3339, UTC
    CHECK ((felhasznalo_id IS NULL) <> (vendeg_id IS NULL))
) STRICT;
INSERT INTO agy_foglalas_new (agy_id, esemeny_id, felhasznalo_id, vendeg_id, created_at)
    SELECT agy_id, esemeny_id, felhasznalo_id, vendeg_id, created_at FROM agy_foglalas;
DROP TABLE agy_foglalas;
ALTER TABLE agy_foglalas_new RENAME TO agy_foglalas;
CREATE INDEX agy_foglalas_agy_id ON agy_foglalas (agy_id);
CREATE UNIQUE INDEX agy_foglalas_esemeny_felhasznalo
    ON agy_foglalas (esemeny_id, felhasznalo_id) WHERE felhasznalo_id IS NOT NULL;
CREATE UNIQUE INDEX agy_foglalas_esemeny_vendeg
    ON agy_foglalas (esemeny_id, vendeg_id) WHERE vendeg_id IS NOT NULL;

-- mashol_alszik (202611281800_szallas.sql)
CREATE TABLE mashol_alszik_new (
    esemeny_id     TEXT NOT NULL REFERENCES esemeny (id) ON DELETE CASCADE,
    felhasznalo_id TEXT REFERENCES felhasznalo (id),
    vendeg_id      TEXT REFERENCES vendeg (id) ON DELETE CASCADE,
    created_at     TEXT NOT NULL, -- RFC 3339, UTC
    CHECK ((felhasznalo_id IS NULL) <> (vendeg_id IS NULL))
) STRICT;
INSERT INTO mashol_alszik_new (esemeny_id, felhasznalo_id, vendeg_id, created_at)
    SELECT esemeny_id, felhasznalo_id, vendeg_id, created_at FROM mashol_alszik;
DROP TABLE mashol_alszik;
ALTER TABLE mashol_alszik_new RENAME TO mashol_alszik;
CREATE UNIQUE INDEX mashol_alszik_esemeny_felhasznalo
    ON mashol_alszik (esemeny_id, felhasznalo_id) WHERE felhasznalo_id IS NOT NULL;
CREATE UNIQUE INDEX mashol_alszik_esemeny_vendeg
    ON mashol_alszik (esemeny_id, vendeg_id) WHERE vendeg_id IS NOT NULL;

-- kiadas_resz (202611282000_koltsegek.sql). No cascade: see above.
CREATE TABLE kiadas_resz_new (
    kiadas_id      TEXT NOT NULL REFERENCES kiadas (id) ON DELETE CASCADE,
    felhasznalo_id TEXT REFERENCES felhasznalo (id),
    vendeg_id      TEXT REFERENCES vendeg (id),
    amount         INTEGER NOT NULL CHECK (amount >= 0),
    CHECK ((felhasznalo_id IS NULL) <> (vendeg_id IS NULL))
) STRICT;
INSERT INTO kiadas_resz_new (kiadas_id, felhasznalo_id, vendeg_id, amount)
    SELECT kiadas_id, felhasznalo_id, vendeg_id, amount FROM kiadas_resz;
DROP TABLE kiadas_resz;
ALTER TABLE kiadas_resz_new RENAME TO kiadas_resz;
CREATE INDEX kiadas_resz_kiadas_id ON kiadas_resz (kiadas_id);
CREATE UNIQUE INDEX kiadas_resz_kiadas_felhasznalo
    ON kiadas_resz (kiadas_id, felhasznalo_id) WHERE felhasznalo_id IS NOT NULL;
CREATE UNIQUE INDEX kiadas_resz_kiadas_vendeg
    ON kiadas_resz (kiadas_id, vendeg_id) WHERE vendeg_id IS NOT NULL;

-- elszamolas_megerosites (202611282200_elszamolas.sql)
CREATE TABLE elszamolas_megerosites_new (
    esemeny_id     TEXT NOT NULL REFERENCES esemeny (id) ON DELETE CASCADE,
    felhasznalo_id TEXT REFERENCES felhasznalo (id),
    vendeg_id      TEXT REFERENCES vendeg (id) ON DELETE CASCADE,
    -- Who pressed it: the Felhasznalo themselves, or a name-only Vendeg's Tag.
    megerositette  TEXT NOT NULL REFERENCES felhasznalo (id),
    created_at     TEXT NOT NULL, -- RFC 3339, UTC
    CHECK ((felhasznalo_id IS NULL) <> (vendeg_id IS NULL))
) STRICT;
INSERT INTO elszamolas_megerosites_new (esemeny_id, felhasznalo_id, vendeg_id, megerositette, created_at)
    SELECT esemeny_id, felhasznalo_id, vendeg_id, megerositette, created_at FROM elszamolas_megerosites;
DROP TABLE elszamolas_megerosites;
ALTER TABLE elszamolas_megerosites_new RENAME TO elszamolas_megerosites;
CREATE INDEX elszamolas_megerosites_esemeny_id ON elszamolas_megerosites (esemeny_id);
CREATE UNIQUE INDEX elszamolas_megerosites_felhasznalo
    ON elszamolas_megerosites (esemeny_id, felhasznalo_id) WHERE felhasznalo_id IS NOT NULL;
CREATE UNIQUE INDEX elszamolas_megerosites_vendeg
    ON elszamolas_megerosites (esemeny_id, vendeg_id) WHERE vendeg_id IS NOT NULL;
