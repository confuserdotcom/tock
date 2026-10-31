package bar

import (
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"time"

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
	a.Reload() // a failure is kept in RulesErr / ConfigErr; the last good files stay
	if m, ok := startMinutes(id); ok {
		s, err := a.StartSession(m)
		if err != nil {
			return "", err
		}
		return fmt.Sprintf("started %d min session", (s.PlannedMS-s.StartMS)/60_000), nil
	}
	switch id {
	case IDSnoozeDistract:
		until, err := a.SnoozeDistract(0, time.Duration(a.Config().Focus.SnoozeMS)*time.Millisecond)
		return snoozedMsg(0, until, err)
	case IDSnoozeOff:
		if _, err := a.ResumeDistract(); err != nil {
			return "", err
		}
		return "distraction notices back on", nil
	}
	if id == IDStop {
		s, err := a.StopSession()
		if err != nil {
			return "", err
		}
		return "stopped session after " + report.Dur(s.EndMS-s.StartMS), nil
	}
	if v, ok := strings.CutPrefix(id, IDRate); ok {
		r, sid, ok := strings.Cut(v, ":")
		n, err := strconv.Atoi(r)
		sess, ok2 := sessionID(sid)
		if !ok || err != nil || n < 1 || n > 5 || !ok2 {
			return "", fmt.Errorf("menu %q: %w", id, ErrUnknown)
		}
		return rateMsg(a.RateSession(sess, n, nil))
	}
	if v, ok := strings.CutPrefix(id, IDRefile); ok {
		sess, ok := sessionID(v)
		if !ok {
			return "", fmt.Errorf("menu %q: %w", id, ErrUnknown)
		}
		d, err := a.SessionDetail(sess)
		switch {
		case errors.Is(err, store.ErrSessionGone):
			return goneMsg, nil
		case err != nil:
			return "", err
		case d.Top.Project == "":
			return "", errors.New("that session has no project to refile to")
		case !d.Top.Refile():
			return "nothing to refile: already filed to " + d.Top.Project, nil
		}
		return refileMsg(a.RefileSession(sess, d.Top.Project))
	}
	if v, ok := strings.CutPrefix(id, IDUnrefile); ok {
		sess, ok := sessionID(v)
		if !ok {
			return "", fmt.Errorf("menu %q: %w", id, ErrUnknown)
		}
		if _, err := a.UndoRefile(sess); err != nil {
			return "", err
		}
		return "undid the refile", nil
	}
	return "", fmt.Errorf("menu %q: %w", id, ErrUnknown)
}

// snoozedMsg turns the result of SnoozeDistract for session (0: whatever
// runs) into the log line.
func snoozedMsg(session int64, until time.Time, err error) (string, error) {
	switch {
	case errors.Is(err, store.ErrNoSession) && session > 0:
		return "that session is no longer running, nothing snoozed", nil
	case errors.Is(err, store.ErrNoSession):
		return "no session running, nothing snoozed", nil
	case err != nil:
		return "", err
	}
	return "distraction notices snoozed until " + until.Format("15:04"), nil
}

// sessionID parses the session id of a menu item id.
func sessionID(v string) (int64, bool) {
	n, err := strconv.ParseInt(v, 10, 64)
	return n, err == nil && n > 0
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
	a.Reload()
	n, d, err := a.Notice(id)
	if err != nil {
		return "", err
	}
	if !slices.ContainsFunc(Buttons(n, d), func(b Button) bool { return b.ID == action }) {
		return "", nil
	}
	switch action {
	case ActStop:
		if n.Topic == focus.TopicStarting {
			// Stop only the session this notice started, never a later one.
			switch running, err := a.RunningSessionID(); {
			case err != nil:
				return "", err
			case running != d.Session:
				return "that session is no longer running", nil
			}
		}
		return Menu(a, IDStop)
	case ActAgain:
		return Menu(a, fmt.Sprintf("%s%d", IDStart, d.Minutes))
	case ActSnooze:
		if err := a.SnoozeBreak(SnoozeFor); err != nil {
			return "", err
		}
		return fmt.Sprintf("break reminders snoozed for %d min", int(SnoozeFor.Minutes())), nil
	case ActSnoozeDistract:
		until, err := a.SnoozeDistract(d.Session, time.Duration(d.SnoozeMin)*time.Minute)
		return snoozedMsg(d.Session, until, err)
	case ActJoin:
		// The runner opens the link; it owns the UI.
		return "opened the meeting link", nil
	case ActIdle:
		return "left away time idle", a.Answer(classify.Interval{From: d.From, To: d.To}, classify.Want{})
	}
	switch {
	case strings.HasPrefix(action, ActUrgeResisted), strings.HasPrefix(action, ActUrgeGiven):
		// Only counts: nothing is opened, closed, focused or blocked.
		given := strings.HasPrefix(action, ActUrgeGiven)
		switch err := a.AddUrge(d.Session, given); {
		case errors.Is(err, store.ErrSessionGone):
			return goneMsg, nil
		case err != nil:
			return "", err
		case given:
			return "counted: gave in", nil
		}
		return "counted: resisted", nil
	case strings.HasPrefix(action, ActQuietCat):
		switch err := a.SetCategoryInterrupt(d.Category, false); {
		case errors.Is(err, app.ErrCategoryGone):
			return "that category is gone", nil
		case err != nil:
			return "", err
		}
		return d.Category + " will no longer interrupt a session", nil
	case strings.HasPrefix(action, ActQuietRule):
		switch err := a.MarkRuleQuiet(d.Rule, d.RuleFP, d.Category); {
		case errors.Is(err, app.ErrRuleChanged):
			return "rule changed, not marked (open tock to pick it again)", nil
		case err != nil:
			return "", err
		}
		return "that rule will no longer interrupt a session", nil
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
