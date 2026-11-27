-- A Tag can mute the DMs of its Csoport. Channel posts are unaffected.
ALTER TABLE tag ADD COLUMN muted INTEGER NOT NULL DEFAULT 0 CHECK (muted IN (0, 1));
