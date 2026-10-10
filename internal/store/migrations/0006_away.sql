-- v2 AWY-01: what the time away was. tock writes a row when the user labels
-- an absence (idle time and gaps between two activities); a row with an
-- empty category and project marks an absence the user chose to leave idle.
CREATE TABLE away (
  id       INTEGER PRIMARY KEY,
  start_ms INTEGER NOT NULL,
  end_ms   INTEGER NOT NULL,
  category TEXT NOT NULL DEFAULT '',
  project  TEXT NOT NULL DEFAULT '',
  at_ms    INTEGER NOT NULL,
  CHECK (end_ms > start_ms)
);

CREATE INDEX away_end ON away(end_ms);
