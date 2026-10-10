-- v2 SYN-01: activity of tock's other machines, read from their day files
-- in the shared sync folder. Each (machine, day) is replaced as a whole when
-- its file changes; remote_file remembers what was read.
CREATE TABLE remote_view (
  id       INTEGER PRIMARY KEY AUTOINCREMENT, -- never reused: RemoteStamp keys on it
  machine  TEXT NOT NULL,
  day      TEXT NOT NULL,
  start_ms INTEGER NOT NULL,
  end_ms   INTEGER NOT NULL,
  bundle   TEXT NOT NULL,
  app      TEXT NOT NULL,
  title    TEXT,
  url      TEXT,
  category TEXT NOT NULL DEFAULT '',
  project  TEXT NOT NULL DEFAULT '',
  CHECK (end_ms > start_ms)
);

CREATE INDEX remote_view_start ON remote_view(start_ms);
CREATE INDEX remote_view_end ON remote_view(end_ms);
CREATE INDEX remote_view_day ON remote_view(machine, day);

CREATE TABLE remote_file (
  machine  TEXT NOT NULL,
  day      TEXT NOT NULL,
  mtime_ms INTEGER NOT NULL,
  size     INTEGER NOT NULL,
  PRIMARY KEY (machine, day)
);
