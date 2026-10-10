package focus

import "fmt"

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

func note(title, body string, ofSession bool) Event {
	return Event{Kind: Notify, Title: title, Body: body, OfSession: ofSession}
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
	if !ok || inMeeting(in) || in.Now-from < in.Cfg.BreakAfterMS {
		return Event{}, false
	}
	if st.BreakFrom == from && in.Now-st.BreakAt < in.Cfg.BreakRepeatMS {
		return Event{}, false
	}
	st.BreakFrom, st.BreakAt = from, in.Now
	return note(TitleBreak, fmt.Sprintf("%d min without a break. Stand up for 5 min.", minutes(in.Now-from)), false), true
}

// stepSession measures the running session, alerts on distraction and ends
// it at its planned end.
func stepSession(in Input, s Session) (writes, notes []Event, ended bool) {
	ws := WindowStats(in.Views, in.Labels, s.StartMS, min(in.Now, s.PlannedMS), in.Cfg)
	next := s
	next.DeepMS, next.DistractMS, next.LongestMS = ws.DeepMS, ws.DistractMS, ws.LongestMS
	if in.Now >= s.PlannedMS {
		next.EndMS = s.PlannedMS
		return []Event{{Kind: SessionEnd, Sess: next}}, []Event{note(TitleDone, doneBody(next), true)}, true
	}
	step := in.Cfg.DistractGraceMS
	if s.Interruptions > 0 {
		step = in.Cfg.DistractRepeatMS
	}
	if !inMeeting(in) && ws.DistractMS >= s.AlertedMS+step {
		next.Interruptions++
		next.AlertedMS = ws.DistractMS
		notes = append(notes, note(TitleFocus, distractBody(next, in.Now), true))
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

func doneBody(s Session) string {
	length := s.EndMS - s.StartMS
	pct := int64(0)
	if length > 0 {
		pct = (s.DeepMS*100 + length/2) / length
	}
	word := "interruptions"
	if s.Interruptions == 1 {
		word = "interruption"
	}
	return fmt.Sprintf("%d min: %d%% deep, %d %s, longest %d min. Take a %d min break.",
		minutes(length), pct, s.Interruptions, word, minutes(s.LongestMS), breakMin(length))
}

// breakMin is the break suggested after a session of length ms.
func breakMin(length int64) int64 {
	switch {
	case length <= 30*60_000:
		return 5
	case length <= 60*60_000:
		return 10
	}
	return 15
}
