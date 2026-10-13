-- v2 tier 11: [privacy] keep_titles. A span whose title was deleted keeps
-- the label its rules gave it here, so deleting titles changes no labels.
CREATE TABLE kept (
  span_id  INTEGER PRIMARY KEY,
  category TEXT NOT NULL DEFAULT '',
  project  TEXT NOT NULL DEFAULT ''
);
