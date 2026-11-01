-- Phase 12 REC-04: one row per recommendation notice trackd sends. kind is
-- 'focus' or 'break'; weekday (0 = Sunday) and hour are local at sending.
-- reply is '' until answered, then set once to 'accept' or 'dismiss' by tock
-- or tockbar; no reply within 30 minutes means ignored.
CREATE TABLE recommend (
    id        INTEGER PRIMARY KEY,
    kind      TEXT NOT NULL,
    at_ms     INTEGER NOT NULL,
    weekday   INTEGER NOT NULL,
    hour      INTEGER NOT NULL,
    length_ms INTEGER NOT NULL,
    reply     TEXT NOT NULL DEFAULT '',
    reply_ms  INTEGER NOT NULL DEFAULT 0
);
CREATE INDEX recommend_at ON recommend(at_ms);
