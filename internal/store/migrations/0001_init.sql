CREATE TABLE schema_version (
  version    INTEGER NOT NULL,
  applied_at INTEGER NOT NULL
);

CREATE TABLE span (
  id          INTEGER PRIMARY KEY,
  start       INTEGER NOT NULL,
  "end"       INTEGER,
  bundle_id   TEXT    NOT NULL,
  app         TEXT    NOT NULL,
  title       TEXT,
  url         TEXT,
  private     INTEGER NOT NULL DEFAULT 0 CHECK (private IN (0,1)),
  category_id INTEGER,
  project_id  INTEGER,
  source      TEXT    NOT NULL DEFAULT 'none' CHECK (source IN ('rule','manual','llm','none')),
  rule_id     INTEGER,
  CHECK ("end" IS NULL OR "end" >= start)
);

CREATE INDEX span_start ON span(start);
CREATE INDEX span_open ON span("end") WHERE "end" IS NULL;

CREATE TABLE daemon_state (
  key     TEXT PRIMARY KEY,
  value   TEXT NOT NULL,
  updated INTEGER NOT NULL
);
