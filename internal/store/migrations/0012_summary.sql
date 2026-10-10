-- v2 SUM-01: Claude's summaries of a day (start_ms 0) and of its work
-- blocks (start_ms = the block's start). tock writes them; trackd never
-- reads them.
CREATE TABLE summary (
  day        TEXT    NOT NULL, -- YYYY-MM-DD
  start_ms   INTEGER NOT NULL,
  text       TEXT    NOT NULL,
  model      TEXT    NOT NULL,
  created_ms INTEGER NOT NULL,
  PRIMARY KEY (day, start_ms)
);
