-- The Idopont the Szervezo confirmed. It is set while the Esemeny is
-- Veglegesitve or Lezajlott, NULL in Egyeztetes (reopening clears it), and
-- kept when a Veglegesitve Esemeny becomes Elmaradt, as a record of what was
-- planned.
--
-- No ON DELETE action: removing the confirmed Idopont must fail rather than
-- leave a Veglegesitve Esemeny without one (a confirmed time is never edited;
-- reopen Egyeztetes instead).
ALTER TABLE esemeny ADD COLUMN confirmed_idopont_id TEXT REFERENCES idopont (id)
    CHECK (CASE allapot
        WHEN 'egyeztetes' THEN confirmed_idopont_id IS NULL
        WHEN 'elmaradt' THEN 1
        ELSE confirmed_idopont_id IS NOT NULL
    END);

CREATE INDEX esemeny_confirmed_idopont_id ON esemeny (confirmed_idopont_id);
