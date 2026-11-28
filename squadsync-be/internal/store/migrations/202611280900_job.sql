-- Timed jobs: work that must happen at a moment, such as the Lezajlott
-- transition, Emlekezteto DMs and the Hatarido DM. A once-a-minute loop runs
-- every job whose run_at has passed and that isn't done, so jobs missed
-- while the server was down run late rather than never.
--
-- A job is done once it ran (even if it found nothing to do) or gave up
-- after too many failures. Cancelling a job deletes it while it's pending.
CREATE TABLE job (
    id         TEXT PRIMARY KEY,
    kind       TEXT NOT NULL,
    esemeny_id TEXT NOT NULL REFERENCES esemeny (id) ON DELETE CASCADE,
    -- Kind-specific JSON, e.g. the confirmed Idopont. Together with kind and
    -- esemeny_id it identifies the job, so scheduling the same job again
    -- (even one that already ran) is a no-op.
    payload    TEXT NOT NULL,
    run_at     TEXT NOT NULL, -- RFC 3339, UTC
    done_at    TEXT,          -- RFC 3339, UTC; NULL while pending
    attempts   INTEGER NOT NULL DEFAULT 0,
    last_error TEXT NOT NULL DEFAULT '',
    created_at TEXT NOT NULL  -- RFC 3339, UTC
) STRICT;

CREATE UNIQUE INDEX job_identity ON job (esemeny_id, kind, payload);
CREATE INDEX job_due ON job (run_at) WHERE done_at IS NULL;

-- The Csoport's Emlekezteto offsets, in minutes before the confirmed start,
-- as a comma-separated list ('' for none). The default is 24 hours and 1
-- hour.
ALTER TABLE csoport ADD COLUMN emlekezteto_percek TEXT NOT NULL DEFAULT '1440,60';

-- The Esemeny's optional voting Hatarido (RFC 3339, UTC), NULL for none.
ALTER TABLE esemeny ADD COLUMN hatarido TEXT;
