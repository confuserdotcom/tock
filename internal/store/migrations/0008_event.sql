-- v2 CAL-01: calendar events trackd reads through EventKit. trackd replaces
-- the rows of its sync window on every change; older rows are history.
CREATE TABLE event (
  id       INTEGER PRIMARY KEY AUTOINCREMENT, -- never reused: EventStamp keys on it
  start_ms INTEGER NOT NULL,
  end_ms   INTEGER NOT NULL,
  title    TEXT NOT NULL DEFAULT '',
  calendar TEXT NOT NULL DEFAULT '',
  CHECK (end_ms > start_ms)
);

CREATE INDEX event_start ON event(start_ms);
CREATE INDEX event_end ON event(end_ms);
