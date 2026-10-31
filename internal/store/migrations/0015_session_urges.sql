-- Phase 9: urge surfing. Counts of distraction notices answered "Back to work"
-- (resisted) and "Open anyway (counted)" (given in). Written only by tock with
-- SET x = x + 1; trackd's UpdateSession/EndSession never touch these columns.
ALTER TABLE session ADD COLUMN urges_resisted INTEGER NOT NULL DEFAULT 0;
ALTER TABLE session ADD COLUMN urges_given INTEGER NOT NULL DEFAULT 0;
