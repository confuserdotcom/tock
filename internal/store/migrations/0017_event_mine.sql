-- event.mine is 1 when the owner accepted the event,
-- organizes it or it has no other attendees; only those events may open their
-- meeting link by themselves ([calendar] auto_open). Rows from before this
-- column read 0 until the next calendar sync.
ALTER TABLE event ADD COLUMN mine INTEGER NOT NULL DEFAULT 0;
