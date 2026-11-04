-- v5 inbox: keys tock labelled on its own from a confident guess, with the
-- learned rule it wrote. A key is never auto-labelled twice, so a correction
-- by tock tag sticks.
CREATE TABLE auto_label (
    key      TEXT PRIMARY KEY,
    category TEXT NOT NULL,
    project  TEXT NOT NULL DEFAULT '',
    reason   TEXT NOT NULL,
    share    REAL NOT NULL,
    rule     TEXT NOT NULL,
    ms       INTEGER NOT NULL,
    at_ms    INTEGER NOT NULL
);
CREATE INDEX auto_label_at ON auto_label(at_ms);
