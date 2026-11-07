-- trackd runs: one row per stretch of time trackd ran without a break in
-- one capture state. A row starts when trackd starts, when it notices a
-- gap in its own ticks (the computer slept), and when capture pauses or
-- resumes. last_ms is written once a minute, whatever is recorded, so the
-- time between rows (and in paused rows) is time nothing was recorded.
-- Local to this machine: never synced.
CREATE TABLE trackd_run (
    id       INTEGER PRIMARY KEY,
    start_ms INTEGER NOT NULL,
    last_ms  INTEGER NOT NULL,
    stop_ms  INTEGER,
    reason   TEXT,
    state    TEXT NOT NULL DEFAULT 'recording',
    boot_ms  INTEGER NOT NULL DEFAULT 0,
    pid      INTEGER NOT NULL DEFAULT 0,
    version  TEXT NOT NULL DEFAULT ''
);
CREATE INDEX trackd_run_start ON trackd_run(start_ms);
