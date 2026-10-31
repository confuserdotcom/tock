-- Phase 8: session quality. switches is the context-switch count of the
-- session window, written when the session ends. rating (1..5), note and
-- rated_ms are the user's close-the-loop answer. Sessions that existed before
-- this migration have no switch count: they are marked -1 so the quality
-- score can leave that part out instead of reading it as zero switches.
ALTER TABLE session ADD COLUMN switches INTEGER NOT NULL DEFAULT 0;
ALTER TABLE session ADD COLUMN rating INTEGER CHECK (rating IS NULL OR rating BETWEEN 1 AND 5);
ALTER TABLE session ADD COLUMN note TEXT;
ALTER TABLE session ADD COLUMN rated_ms INTEGER;
UPDATE session SET switches = -1;
