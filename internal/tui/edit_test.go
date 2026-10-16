package tui

import "testing"

func TestTodaySplitDeleteAddUndo(t *testing.T) {
	m, _ := seeded(t)
	// The cursor follows the last row (Mail 14:50–15:00). Split it at 14:55.
	m = press(t, m, "s")
	mustContain(t, m, "Split: label part of it", "14:55-15:00")
	m = press(t, m, "enter")
	m = typeText(t, m, "stu")
	m = press(t, m, "enter", "enter")
	mustContain(t, m, "labelled 14:55–15:00 (5m) → Study   u undo")
	m = press(t, m, "u")
	mustContain(t, m, "undid: label Sat 14:55–15:00 Study")

	// Delete the YouTube row (11:14–11:26): k from the end.
	m = press(t, m, "k", "k", "k", "k", "k")
	mustContain(t, m, "▸ 11:14–11:26")
	m = press(t, m, "d")
	mustContain(t, m, "deleted 11:14–11:26 (12m)   u undo")

	// Add time over the idle hour, typed by hand.
	m = press(t, m, "+")
	for i := 0; i < 11; i++ {
		m = press(t, m, "backspace")
	}
	m = typeText(t, m, "11:41-12:22")
	m = press(t, m, "enter")
	m = typeText(t, m, "com")
	m = press(t, m, "enter", "enter")
	mustContain(t, m, "added 11:41–12:22 (41m) → Communication   u undo", "Entry")

	// A range into the future is refused in the dialog.
	m = press(t, m, "+")
	for i := 0; i < 11; i++ {
		m = press(t, m, "backspace")
	}
	m = typeText(t, m, "14:00-16:00")
	m = press(t, m, "enter")
	mustContain(t, m, "the range reaches into the future")
	m = press(t, m, "esc")
}
