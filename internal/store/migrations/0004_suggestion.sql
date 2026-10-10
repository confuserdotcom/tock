-- Phase 6: Claude suggestions for inbox keys. tock writes these rows; trackd
-- never reads them. A row with category and project both NULL means Claude
-- had no suggestion: the key is never sent again.
CREATE TABLE suggestion (
  key        TEXT    PRIMARY KEY,
  category   TEXT,
  project    TEXT,
  model      TEXT    NOT NULL,
  created_ms INTEGER NOT NULL
);
