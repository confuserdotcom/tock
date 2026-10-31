package bar

import (
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"

	"tock/internal/app"
	"tock/internal/classify"
	"tock/internal/focus"
	"tock/internal/report"
	"tock/internal/store"
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

// goneMsg answers a click on a session that was deleted since.
const goneMsg = "that session is gone"

// sessionMin is a finished session's length in whole minutes, rounded.
func sessionMin(s focus.Session) int64 { return (s.EndMS - s.StartMS + 30_000) / 60_000 }

// rateMsg turns the result of RateSession into the log line.
func rateMsg(s focus.Session, _ int, err error) (string, error) {
	switch {
	case errors.Is(err, store.ErrSessionGone):
		return goneMsg, nil
	case err != nil:
		return "", err
	}
	return fmt.Sprintf("rated the %d min session ★%d", sessionMin(s), s.Rating), nil
}

// refileMsg turns the result of RefileSession into the log line.
func refileMsg(s focus.Session, project string, err error) (string, error) {
	switch {
	case errors.Is(err, store.ErrSessionGone):
		return goneMsg, nil
	case err != nil:
		return "", err
	}
	return fmt.Sprintf("refiled %d min to %s", sessionMin(s), project), nil
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
	if v, ok := strings.CutPrefix(action, ActRate); ok {
		n, _ := strconv.Atoi(v) // Buttons only offers 1..5
		return rateMsg(a.RateSession(d.Session, n, nil))
	}
	if action == ActRefile {
		return refileMsg(a.RefileSession(d.Session, d.Project))
	}
	i, _ := awayIndex(action) // Buttons only offers valid indexes
	if err := a.Answer(classify.Interval{From: d.From, To: d.To}, d.Labels[i]); err != nil {
		return "", err
	}
	return "labelled away time", nil
}
