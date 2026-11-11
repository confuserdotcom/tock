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
	"tock/internal/config"
	"tock/internal/focus"
	"tock/internal/recommend"
	"tock/internal/report"
	"tock/internal/store"
)

// ErrUnknown means a click named no known item or action.
var ErrUnknown = errors.New("unknown action")

// Menu runs the menu item id and returns what happened, for the log. Quit,
// the System Settings panes and the dashboard are the Cocoa side's to handle.
func Menu(a *app.App, id string) (string, error) {
	a.Reload() // a failure is kept in RulesErr / ConfigErr; the last good files stay
	if m, ok := startMinutes(id); ok {
		s, err := a.StartSession(m)
		if err != nil {
			return "", err
		}
		return fmt.Sprintf("started %d min session", (s.PlannedMS-s.StartMS)/60_000), nil
	}
	if strings.HasPrefix(id, IDFocus) {
		n, goal, err := startGoal(a, id)
		if err != nil {
			return "", err
		}
		s, err := a.StartSessionGoal(n, goal)
		if err != nil {
			return "", err
		}
		msg := fmt.Sprintf("started %d min session", (s.PlannedMS-s.StartMS)/60_000)
		if s.Goal != "" {
			msg += " · " + s.Goal
		}
		return msg, nil
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
	if v, ok := strings.CutPrefix(id, IDFeature); ok {
		return toggleFeature(a, v)
	}
	for _, p := range []struct {
		prefix string
		hide   bool
	}{{IDHide, true}, {IDUnhide, false}} {
		if v, ok := strings.CutPrefix(id, p.prefix); ok {
			return setHidden(a, id, v, p.hide)
		}
	}
	if id == IDStop {
		s, err := a.StopSession()
		if err != nil {
			return "", err
		}
		return "stopped session after " + report.Dur(s.EndMS-s.StartMS), nil
	}
	for _, p := range []struct{ prefix, reply string }{{IDRecAccept, recommend.ReplyAccept}, {IDRecDismiss, recommend.ReplyDismiss}} {
		if v, ok := strings.CutPrefix(id, p.prefix); ok {
			rid, at, ok := recKey(v)
			if !ok {
				return "", fmt.Errorf("menu %q: %w", id, ErrUnknown)
			}
			return answerMsg(a.AnswerRecommendAt(rid, at, "", p.reply))
		}
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

// setHidden adds to or removes from the [privacy] hide lists the
// "app:<bundle id>" or "site:<host>" in v, and reloads.
func setHidden(a *app.App, id, v string, hide bool) (string, error) {
	kind, value, ok := strings.Cut(v, ":")
	if !ok || value == "" || (kind != config.HideApp && kind != config.HideSite) {
		return "", fmt.Errorf("menu %q: %w", id, ErrUnknown)
	}
	if err := config.SetHidden(a.ConfigPath(), kind, value, hide); err != nil {
		return "", fmt.Errorf("bar: %s: %w", id, err)
	}
	a.Reload()
	name := value
	if kind == config.HideApp {
		if n, err := a.AppName(value); err == nil && n != "" {
			name = n
		}
	}
	if hide {
		return name + " hidden", nil
	}
	return name + " shown again", nil
}

// toggleFeature flips one config.Features toggle in config.toml, the way
// tock choice does, and reloads so the change is live at once.
func toggleFeature(a *app.App, fid string) (string, error) {
	for _, f := range config.Features {
		if f.ID != fid {
			continue
		}
		on := !f.Set(a.Config())
		if err := config.SaveFeatures(a.ConfigPath(), map[string]bool{fid: on}); err != nil {
			return "", fmt.Errorf("bar: %s: %w", fid, err)
		}
		a.Reload()
		switch {
		case !on:
			return f.Name + " off", nil
		case fid == "ocr":
			return f.Name + " on · run tock ocr grant if trackd asks for Screen Recording", nil
		}
		return f.Name + " on", nil
	}
	return "", fmt.Errorf("menu %q: %w", IDFeature+fid, ErrUnknown)
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

// answerMsg turns the result of AnswerRecommend into the line a click shows. A
// suggestion that is no longer open, or a session already running, is an
// answer to the click, not a failure.
func answerMsg(res app.RecAnswer, err error) (string, error) {
	switch {
	case errors.Is(err, app.ErrNoRecommend):
		return "that suggestion is no longer open", nil
	case errors.Is(err, store.ErrSessionRunning):
		return "a session is already running, nothing started", nil
	case err != nil:
		return "", err
	case res.BreakUntilMS > 0:
		return "break until " + res.BreakUntil.Format("15:04"), nil
	case res.Session != nil:
		return fmt.Sprintf("started %d min session", (res.Session.PlannedMS-res.Session.StartMS)/60_000), nil
	}
	return "suggestion dismissed", nil
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
		if d.Rec > 0 {
			// The suggestion is closed without an answer; one already answered
			// or expired is fine.
			if err := a.SnoozeRecommend(d.Rec, d.RecAt); err != nil && !errors.Is(err, app.ErrNoRecommend) {
				return "", err
			}
		}
		return fmt.Sprintf("break reminders snoozed for %d min", int(SnoozeFor.Minutes())), nil
	case ActSnoozeDistract:
		until, err := a.SnoozeDistract(d.Session, time.Duration(d.SnoozeMin)*time.Minute)
		return snoozedMsg(d.Session, until, err)
	case ActBack:
		return "back to work", nil
	case ActMore:
		if _, err := a.ExtendBreak(BreakMore.Milliseconds()); err != nil {
			return "", err
		}
		return fmt.Sprintf("break %d min longer", int(BreakMore.Minutes())), nil
	case ActWakeWork:
		want := classify.Want{Category: d.Category, Project: d.Project}
		rule, err := a.WakeIsWork(d.Key, want)
		if err != nil {
			return "", err
		}
		return fmt.Sprintf("%s now files as %s (%s); tock undo takes it back", d.Key, WantText(want), rule), nil
	case ActPaper:
		switch err := a.SetPaper(d.Session); {
		case errors.Is(err, store.ErrNoSession):
			return goneMsg, nil
		case err != nil:
			return "", err
		}
		return "paper work: idle time no longer counts as off task this session", nil
	case ActJoin:
		// The runner opens the link; it owns the UI.
		return "opened the meeting link", nil
	case ActIdle:
		return "left away time idle", a.Answer(classify.Interval{From: d.From, To: d.To}, classify.Want{})
	case ActRecAccept, ActRecDismiss:
		// The kind follows from the topic; the session length and the open
		// check come from the recommend row, never from the notice data.
		reply := recommend.ReplyAccept
		if action == ActRecDismiss {
			reply = recommend.ReplyDismiss
		}
		kind := recommend.KindFocus
		if n.Topic == focus.TopicBreak {
			kind = recommend.KindBreak
		}
		// Row ids can be reused, so the send time the notice was queued with
		// must match the row's, as for the menu and the screen.
		return answerMsg(a.AnswerRecommendAt(d.Rec, d.RecAt, kind, reply))
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
