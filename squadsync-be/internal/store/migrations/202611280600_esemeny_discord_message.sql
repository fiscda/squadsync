-- The Discord messages that show an Esemeny: its post in the Csoport's
-- Csatorna, or, without a Csatorna, the announcement DM sent to each Tag
-- (one per Discord account). Every one is edited in place when the Esemeny's
-- counts or Helyezes change.
CREATE TABLE esemeny_discord_message (
    esemeny_id TEXT NOT NULL REFERENCES esemeny (id) ON DELETE CASCADE,
    -- 'csatorna' for the post in the linked channel, 'dm' for an
    -- announcement sent by DM.
    kind       TEXT NOT NULL CHECK (kind IN ('csatorna', 'dm')),
    channel_id TEXT NOT NULL, -- a DM's channel for kind 'dm'
    message_id TEXT NOT NULL,
    created_at TEXT NOT NULL, -- RFC 3339, UTC
    PRIMARY KEY (channel_id, message_id)
) STRICT;

CREATE INDEX esemeny_discord_message_esemeny_id ON esemeny_discord_message (esemeny_id);
