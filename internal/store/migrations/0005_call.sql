-- v2 CAL-02: time with a camera or microphone in use. trackd extends the
-- latest row while a call goes on. Timestamps only.
CREATE TABLE call (
  id       INTEGER PRIMARY KEY,
  start_ms INTEGER NOT NULL,
  end_ms   INTEGER NOT NULL,
  CHECK (end_ms >= start_ms)
);

CREATE INDEX call_end ON call(end_ms);
