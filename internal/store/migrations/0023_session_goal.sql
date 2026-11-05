-- Focus goal: what the user said a session is for, from the start prompt.
ALTER TABLE session ADD COLUMN goal TEXT NOT NULL DEFAULT '';
