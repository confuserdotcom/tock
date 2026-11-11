-- Results derived from the other tables, kept so a new process
-- need not compute them again: closed past days (name YYYY-MM-DD) and the
-- trained local model (name "model"). Each is stored under a key of every
-- input it was computed from and used only while the key still matches.
-- Derived only: the table can be emptied at any time.
CREATE TABLE derived (
    name  TEXT PRIMARY KEY,
    key   TEXT NOT NULL,
    data  BLOB NOT NULL,
    at_ms INTEGER NOT NULL
);
