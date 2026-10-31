package focus

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/x/ansi"

	"tock/internal/classify"
	"tock/internal/stats"
)

// Step runs the engine once. Store events come first and notifications
// last, so a caller that stops at the first failed write never sends a
// notification the next tick would send again.
func Step(in Input) []Event {
	var writes, notes []Event
	st := in.State
	stateChanged := false
	if s := openSession(in); s != nil {
		w, n, ended := stepSession(in, *s)
		writes, notes = append(writes, w...), append(notes, n...)
		if ended {
			// The session's end prompts a break, so this run's next reminder
			// waits one repeat interval.
			st.BreakFrom, _ = runStart(in)
			st.BreakAt = in.Now
			stateChanged = true
		}
	} else if n, ok := stepBreak(in, &st); ok {
		notes = append(notes, n)
		stateChanged = true
	}
	writes = append(writes, autoSessions(in)...)
	if n, ok := stepRecap(in, &st); ok {
		notes = append(notes, n)
		stateChanged = true
	}
	if n, ok := stepAway(in, &st); ok {
		notes = append(notes, n)
		stateChanged = true
	}
	if stateChanged {
		writes = append(writes, Event{Kind: StateUpdate, State: st})
	}
	return append(writes, notes...)
}

// openSession is the running manual session, or nil.
func openSession(in Input) *Session {
	for i := range in.Sessions {
		if s := &in.Sessions[i]; s.Kind == KindManual && s.EndMS == 0 {
			return s
		}
	}
	return nil
}

func note(topic, title, body string, ofSession bool) Event {
	return Event{Kind: Notify, Topic: topic, Title: title, Body: body, OfSession: ofSession}
}

// minutes rounds ms to whole minutes.
func minutes(ms int64) int64 { return (max(ms, 0) + 30_000) / 60_000 }

// runStart is the start of the activity run that holds the view open at
// Now: the end of the last idle view of at least IdleResetMS, or the start
// of the first view after a gap of at least IdleResetMS, else the first
// view's start. ok is false when no view is open at Now or it is idle.
func runStart(in Input) (from int64, ok bool) {
	cur := current(in)
	if cur == nil || isIdle(*cur) {
		return 0, false
	}
	var lastEnd int64
	for i, v := range in.Views {
		switch {
		case i == 0:
			from = v.Start
		case isIdle(v):
			if v.DurMS() >= IdleResetMS {
				from = v.End
			}
		case v.Start-lastEnd >= IdleResetMS:
			from = v.Start
		}
		lastEnd = max(lastEnd, v.End)
	}
	return from, true
}

// stepBreak reminds after BreakAfterMS of continuous activity, then every
// BreakRepeatMS while the run goes on. Calls defer it.
func stepBreak(in Input, st *State) (Event, bool) {
	from, ok := runStart(in)
	if !ok || in.Now < st.SnoozeUntil || inMeeting(in) || in.Now-from < in.Cfg.BreakAfterMS {
		return Event{}, false
	}
	if st.BreakFrom == from && in.Now-st.BreakAt < in.Cfg.BreakRepeatMS {
		return Event{}, false
	}
	st.BreakFrom, st.BreakAt = from, in.Now
	return note(TopicBreak, TitleBreak, fmt.Sprintf("%d min without a break. Stand up for 5 min.", minutes(in.Now-from)), false), true
}

// stepSession measures the running session, alerts on distraction and ends
// it at its planned end.
func stepSession(in Input, s Session) (writes, notes []Event, ended bool) {
	ws := WindowStats(in.Views, in.Labels, s.StartMS, min(in.Now, s.PlannedMS), in.Cfg)
	next := s
	next.DeepMS, next.DistractMS, next.LongestMS = ws.DeepMS, ws.DistractMS, ws.LongestMS
	if in.Now >= s.PlannedMS {
		next.EndMS = s.PlannedMS
		next.Switches = stats.WindowSwitches(in.Views, in.Labels, s.StartMS, s.PlannedMS)
		b := Window(in.Views, in.Labels, s.StartMS, s.PlannedMS, in.Cfg)
		done := note(TopicDone, TitleDone, doneBody(next, Quality(next, in.Weights), b), true)
		done.Minutes = int(minutes(s.PlannedMS - s.StartMS))
		done.Project, done.Refile = b.Project, b.Refile()
		done.Sess = next
		return []Event{{Kind: SessionEnd, Sess: next}}, []Event{done}, true
	}
	step := in.Cfg.DistractGraceMS
	if s.Interruptions > 0 {
		step = in.Cfg.DistractRepeatMS
	}
	if !inMeeting(in) && ws.DistractMS >= s.AlertedMS+step {
		next.Interruptions++
		next.AlertedMS = ws.DistractMS
		// A snooze mutes the notice only: the counters above advance as
		// without it, so the next notice comes at the next threshold.
		if in.Now >= in.State.DistractSnoozeUntil {
			n := note(TopicFocus, TitleFocus, distractBody(next, in.Now), true)
			n.Sess = next
			notes = append(notes, n)
		}
	}
	if next != s {
		writes = append(writes, Event{Kind: SessionUpdate, Sess: next})
	}
	return writes, notes, false
}

func distractBody(s Session, now int64) string {
	return fmt.Sprintf("%d min distracted in your %d min session, %d min left.",
		minutes(s.DistractMS), minutes(s.PlannedMS-s.StartMS), minutes(s.PlannedMS-now))
}

// doneNameCells is the widest a category name gets in the done notice.
const doneNameCells = 20

// doneBody is the done notice: length, quality, deep share, interruptions
// and the two top categories, left out when the window has none.
func doneBody(s Session, q QualityScore, b Breakdown) string {
	length := s.EndMS - s.StartMS
	pct := int64(0)
	if length > 0 {
		pct = (s.DeepMS*100 + length/2) / length
	}
	body := fmt.Sprintf("%d min · quality %d · %d%% deep · %s",
		minutes(length), q.Score, pct, InterruptionsText(s.Interruptions))
	var names []string
	for _, c := range b.Categories[:min(len(b.Categories), 2)] {
		names = append(names, ansi.Truncate(c.Name, doneNameCells, "…"))
	}
	if len(names) > 0 {
		body += " · top: " + strings.Join(names, ", ")
	}
	return body
}

// autoSessions records each ended focus block of at least AutoMinMS that no
// session overlaps (an earlier tick's auto row overlaps its own block, so
// each block is recorded once).
func autoSessions(in Input) []Event {
	var out []Event
	for _, b := range stats.Compute(in.Views, in.Labels).Blocks {
		if b.DeepMS < in.Cfg.AutoMinMS || b.End > in.Now-stats.ToleranceMS || overlaps(in.Sessions, b.Start, b.End, in.Now) {
			continue
		}
		ws := WindowStats(in.Views, in.Labels, b.Start, b.End, in.Cfg)
		out = append(out, Event{Kind: AutoSession, Sess: Session{Kind: KindAuto, StartMS: b.Start, EndMS: b.End,
			DeepMS: ws.DeepMS, DistractMS: ws.DistractMS, LongestMS: ws.LongestMS,
			Switches: stats.WindowSwitches(in.Views, in.Labels, b.Start, b.End)}})
	}
	return out
}

// overlaps reports a session overlapping [from, to); a running session ends
// at now.
func overlaps(ss []Session, from, to, now int64) bool {
	for _, s := range ss {
		end := s.EndMS
		if end == 0 {
			end = now
		}
		if s.StartMS < to && end > from {
			return true
		}
	}
	return false
}

func dayKey(day time.Time) string { return day.Format("2006-01-02") }

// RecapDue reports whether this tick should send the recap: it is on, its
// time of day has come, today's recap has not gone out and today has at
// least RecapMinActiveMS of activity.
func RecapDue(in Input) bool {
	if in.Cfg.RecapAt < 0 || in.State.RecapDay == dayKey(in.Day) {
		return false
	}
	t := time.UnixMilli(in.Now).In(in.Day.Location())
	if t.Hour()*60+t.Minute() < in.Cfg.RecapAt {
		return false
	}
	return activeSince(in, in.Day.UnixMilli()) >= RecapMinActiveMS
}

func stepRecap(in Input, st *State) (Event, bool) {
	if in.RecapBody == "" || !RecapDue(in) {
		return Event{}, false
	}
	st.RecapDay = dayKey(in.Day)
	return note(TopicRecap, TitleRecap, in.RecapBody, false), true
}

// stepAway asks once what an absence was, within AwayNotifyMS of the
// return.
func stepAway(in Input, st *State) (Event, bool) {
	a := in.Absence
	if a == nil || a.To <= st.AwayAt || in.Now-a.To > AwayNotifyMS {
		return Event{}, false
	}
	st.AwayAt = a.To
	clock := func(ms int64) string { return time.UnixMilli(ms).In(in.Day.Location()).Format("15:04") }
	ask := "What was it?"
	if b := in.AbsenceBefore; b.Category != "" {
		what := b.Category
		if b.Project != "" {
			what += " / " + b.Project
		}
		ask = "Still on " + what + "?"
	}
	n := note(TopicAway, TitleAway, fmt.Sprintf("Away %s (%s–%s). %s Run tock away or open tock.",
		classify.FormatDur(a.To-a.From), clock(a.From), clock(a.To), ask), false)
	n.Away, n.Before = *a, in.AbsenceBefore
	return n, true
}

// activeSince is the non-idle time of the views after from.
func activeSince(in Input, from int64) int64 {
	var ms int64
	for _, v := range in.Views {
		if !isIdle(v) {
			ms += max(v.End-max(v.Start, from), 0)
		}
	}
	return ms
}
