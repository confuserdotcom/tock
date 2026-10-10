-- Phase 2: manual labels and hidden inbox keys. Names are text because
-- rules.toml is the source of truth for categories and projects. Written by
-- the tock CLI; trackd stays the only writer of span.
CREATE TABLE override (
  span_id  INTEGER PRIMARY KEY REFERENCES span(id),
  category TEXT,
  project  TEXT,
  at       INTEGER NOT NULL
);

CREATE TABLE inbox_ignore (
  key TEXT PRIMARY KEY,
  at  INTEGER NOT NULL
);
