-- The Discord channel linked to a Csoport, where public moments are posted.
-- A Csoport has at most one, so linking again replaces the row.
CREATE TABLE csatorna (
    csoport_id   TEXT PRIMARY KEY REFERENCES csoport (id),
    guild_id     TEXT NOT NULL,
    channel_id   TEXT NOT NULL,
    -- The channel's name when it was linked, for display only. Empty if the
    -- bot didn't know it.
    channel_name TEXT NOT NULL,
    linked_by    TEXT NOT NULL REFERENCES felhasznalo (id),
    linked_at    TEXT NOT NULL -- RFC 3339, UTC
) STRICT;
