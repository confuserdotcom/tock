-- v2 EDT-01: timeline edits tock writes over the recorded spans, which stay
-- trackd's. kind add: an entry replaces the range; cut: the range is
-- removed; label: the range takes the label. Later rows win.
CREATE TABLE edit (
  id       INTEGER PRIMARY KEY AUTOINCREMENT, -- never reused: EditStamp keys on it
  kind     TEXT NOT NULL CHECK (kind IN ('add','cut','label')),
  start_ms INTEGER NOT NULL,
  end_ms   INTEGER NOT NULL,
  category TEXT NOT NULL DEFAULT '',
  project  TEXT NOT NULL DEFAULT '',
  note     TEXT NOT NULL DEFAULT '',
  at_ms    INTEGER NOT NULL,
  CHECK (end_ms > start_ms)
);

CREATE INDEX edit_start ON edit(start_ms);
CREATE INDEX edit_end ON edit(end_ms);

-- v2 UND-01: what tock undo reverses, newest last. data is JSON with the
-- state before the change.
CREATE TABLE undo (
  id    INTEGER PRIMARY KEY AUTOINCREMENT,
  at_ms INTEGER NOT NULL,
  what  TEXT NOT NULL,
  data  TEXT NOT NULL
);
