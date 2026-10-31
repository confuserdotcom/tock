-- Phase 11 PLAN-04: one row per slot notice sent, so a slot notifies once
-- across trackd restarts. event.join_url holds the meeting link trackd found
-- in the calendar event, '' = none.
CREATE TABLE slot_notice (
    slot_key TEXT NOT NULL,
    kind     TEXT NOT NULL,
    at_ms    INTEGER NOT NULL,
    PRIMARY KEY (slot_key, kind)
);
ALTER TABLE event ADD COLUMN join_url TEXT NOT NULL DEFAULT '';
