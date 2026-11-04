-- Phase 0: derived results of closed days (the focus target's past days),
-- each under a key over every input it was computed from. Derived only: it
-- can be dropped at any time and is rebuilt on the next read.
CREATE TABLE day_cache (
    day   TEXT PRIMARY KEY, -- local date, YYYY-MM-DD
    key   TEXT NOT NULL,
    data  BLOB NOT NULL,
    at_ms INTEGER NOT NULL
);
