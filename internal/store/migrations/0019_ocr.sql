-- Phase 13 OCR-04: the keywords read on screen for one span (space-joined,
-- at most 48), deleted with the span and when titles expire. Never synced,
-- exported or sent anywhere.
CREATE TABLE span_ocr (
    span_id INTEGER PRIMARY KEY,
    words   TEXT NOT NULL,
    at_ms   INTEGER NOT NULL
);
-- Phase 13 OCR-02/06: screen reads per local day and reason. Reason 'ok'
-- counts the reads done and sums their cost in cost_ms.
CREATE TABLE ocr_stat (
    day     TEXT NOT NULL,
    reason  TEXT NOT NULL,
    n       INTEGER NOT NULL DEFAULT 0,
    cost_ms INTEGER NOT NULL DEFAULT 0,
    PRIMARY KEY (day, reason)
);
