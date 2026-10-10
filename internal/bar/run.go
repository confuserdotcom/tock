package bar

import (
	"errors"
	"fmt"
	"slices"

	"tock/internal/app"
	"tock/internal/classify"
	"tock/internal/report"
)

// ErrUnknown means a click named no known item or action.
var ErrUnknown = errors.New("unknown action")

// Menu runs the menu item id and returns what happened, for the log. Quit
// and Settings are the Cocoa side's to handle.
func Menu(a *app.App, id string) (string, error) {
	if m, ok := startMinutes(id); ok {
		s, err := a.StartSession(m)
		if err != nil {
			return "", err
		}
		return fmt.Sprintf("started %d min session", (s.PlannedMS-s.StartMS)/60_000), nil
	}
	if id == IDStop {
		s, err := a.StopSession()
		if err != nil {
			return "", err
		}
		return "stopped session after " + report.Dur(s.EndMS-s.StartMS), nil
	}
	return "", fmt.Errorf("menu %q: %w", id, ErrUnknown)
}

// Respond runs the button action of notice id. Dismissing a notification
// or clicking its body (an action that is not one of its buttons) does
// nothing.
func Respond(a *app.App, id int64, action string) (string, error) {
	n, d, err := a.Notice(id)
	if err != nil {
		return "", err
	}
	if !slices.ContainsFunc(Buttons(n, d), func(b Button) bool { return b.ID == action }) {
		return "", nil
	}
	switch action {
	case ActStop:
		return Menu(a, IDStop)
	case ActAgain:
		return Menu(a, fmt.Sprintf("%s%d", IDStart, d.Minutes))
	case ActSnooze:
		if err := a.SnoozeBreak(SnoozeFor); err != nil {
			return "", err
		}
		return fmt.Sprintf("break reminders snoozed for %d min", int(SnoozeFor.Minutes())), nil
	case ActIdle:
		return "left away time idle", a.Answer(classify.Interval{From: d.From, To: d.To}, classify.Want{})
	}
	i, _ := awayIndex(action) // Buttons only offers valid indexes
	if err := a.Answer(classify.Interval{From: d.From, To: d.To}, d.Labels[i]); err != nil {
		return "", err
	}
	return "labelled away time", nil
}
