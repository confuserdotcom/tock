-- Wake screen: how many wake screens a session showed, and how many of
-- them were followed by work within two minutes.
ALTER TABLE session ADD COLUMN wakes INTEGER NOT NULL DEFAULT 0;
ALTER TABLE session ADD COLUMN wakes_back INTEGER NOT NULL DEFAULT 0;
