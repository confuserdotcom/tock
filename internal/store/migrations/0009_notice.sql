-- v2 NTF-01: notifications trackd hands to the tock menu bar app, which
-- shows them with buttons. shown_ms is set when the bar takes a row.
CREATE TABLE notice (
  id       INTEGER PRIMARY KEY AUTOINCREMENT,
  at_ms    INTEGER NOT NULL,
  topic    TEXT NOT NULL,
  title    TEXT NOT NULL,
  body     TEXT NOT NULL,
  data     TEXT NOT NULL DEFAULT '{}',
  shown_ms INTEGER
);

CREATE INDEX notice_shown ON notice(shown_ms);
