-- Phase 5: focus sessions. tock inserts and stops manual sessions; trackd
-- updates the running one's counters, closes it at planned_ms and inserts
-- auto sessions (always closed). Every update of a running row is guarded by
-- end_ms IS NULL, so tock and trackd never overwrite each other.
CREATE TABLE session (
  id            INTEGER PRIMARY KEY,
  kind          TEXT    NOT NULL CHECK (kind IN ('manual','auto')),
  start_ms      INTEGER NOT NULL,
  planned_ms    INTEGER,
  end_ms        INTEGER,
  interruptions INTEGER NOT NULL DEFAULT 0,
  alerted_ms    INTEGER NOT NULL DEFAULT 0,
  deep_ms       INTEGER NOT NULL DEFAULT 0,
  distract_ms   INTEGER NOT NULL DEFAULT 0,
  longest_ms    INTEGER NOT NULL DEFAULT 0,
  CHECK (end_ms IS NULL OR end_ms >= start_ms)
);

CREATE INDEX session_start ON session(start_ms);
CREATE UNIQUE INDEX session_auto_start ON session(start_ms) WHERE kind = 'auto';
CREATE UNIQUE INDEX session_one_open ON session(kind) WHERE end_ms IS NULL;
