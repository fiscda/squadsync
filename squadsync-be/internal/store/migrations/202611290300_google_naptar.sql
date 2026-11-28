-- Google Calendar sync (ADR-0005). Go owns the Google tokens.
--
-- A Felhasznalo's connection to Google Calendar: at most one. The app writes
-- only to the one "Squad Sync" calendar it creates in their account (scope
-- calendar.app.created).
CREATE TABLE google_naptar (
    felhasznalo_id TEXT PRIMARY KEY REFERENCES felhasznalo (id) ON DELETE CASCADE,
    -- Random, new on every connect: a sync started under an older
    -- connection stores nothing.
    kapcsolat_id   TEXT NOT NULL,
    refresh_token  TEXT NOT NULL,
    -- The "Squad Sync" calendar's id; NULL until the first sync creates it.
    calendar_id    TEXT,
    -- When Google refused the token (revoked, invalid_grant); NULL while it
    -- works. A broken connection syncs nothing until it is connected again.
    megszakadt_at  TEXT,           -- RFC 3339, UTC
    created_at     TEXT NOT NULL   -- RFC 3339, UTC
) STRICT;

-- What the sync knows of one (Felhasznalo, Esemeny) pair: the event in their
-- calendar, and the latest request to sync it. A row exists while an event
-- exists or a sync is pending.
CREATE TABLE google_naptar_esemeny (
    felhasznalo_id TEXT NOT NULL REFERENCES felhasznalo (id) ON DELETE CASCADE,
    esemeny_id     TEXT NOT NULL REFERENCES esemeny (id) ON DELETE CASCADE,
    -- The Google event id; NULL while there is no event.
    event_id       TEXT,
    -- A hash of the event's content as last written ('' for none), so an
    -- unchanged event isn't written again.
    tartalom       TEXT NOT NULL DEFAULT '',
    -- Random, new on every request to sync the pair: a sync that finds it
    -- changed while it called Google schedules another one.
    kerelem        TEXT NOT NULL,
    PRIMARY KEY (felhasznalo_id, esemeny_id)
) STRICT;

CREATE INDEX google_naptar_esemeny_esemeny_id ON google_naptar_esemeny (esemeny_id);
