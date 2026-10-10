-- v2 REV-01: the focus the user set for a week in tock review. week is the
-- Monday's date, YYYY-MM-DD.
CREATE TABLE plan (
  week  TEXT PRIMARY KEY,
  text  TEXT NOT NULL,
  at_ms INTEGER NOT NULL
);
