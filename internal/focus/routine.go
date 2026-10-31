package focus

import "time"

// StartDue reports whether this tick should send the start-of-day notice:
// it is on, today is a work day, today's notice has not gone out, no call is
// going on and today has at least RecapMinActiveMS of activity. The cheap
// conditions come first so an idle tick scans no views.
func StartDue(in Input) bool {
	if !in.Routine.Start || in.State.StartDay == dayKey(in.Day) || in.Hours.Off(in.Day) || inMeeting(in) {
		return false
	}
	return activeSince(in, in.Day.UnixMilli()) >= RecapMinActiveMS
}

// stepStart sends the start-of-day notice once a day. The caller fills
// StartBody only when StartDue.
func stepStart(in Input, st *State) (Event, bool) {
	if in.StartBody == "" || !StartDue(in) {
		return Event{}, false
	}
	st.StartDay = dayKey(in.Day)
	return note(TopicStart, TitleStart, in.StartBody, false), true
}

// MiddayDue reports whether this tick should send the midday check-in: it
// is on, today is a work day, today's check-in has not gone out, its time
// of day has come, the work day has not ended, no call is going on and today
// has at least RecapMinActiveMS of activity. The cheap conditions come first
// so an idle tick scans no views.
func MiddayDue(in Input) bool {
	if in.Routine.MiddayAt < 0 || in.State.MiddayDay == dayKey(in.Day) || in.Hours.Off(in.Day) {
		return false
	}
	t := time.UnixMilli(in.Now).In(in.Day.Location())
	if t.Hour()*60+t.Minute() < in.Routine.MiddayAt {
		return false
	}
	if in.Hours.Set {
		if _, to := in.Hours.Window(in.Day); in.Now >= to {
			return false
		}
	}
	if inMeeting(in) {
		return false
	}
	return activeSince(in, in.Day.UnixMilli()) >= RecapMinActiveMS
}

// stepMidday sends the midday check-in once a day. The caller fills
// MiddayBody only when MiddayDue.
func stepMidday(in Input, st *State) (Event, bool) {
	if in.MiddayBody == "" || !MiddayDue(in) {
		return Event{}, false
	}
	st.MiddayDay = dayKey(in.Day)
	return note(TopicMidday, TitleMidday, in.MiddayBody, false), true
}

// OverMS is the non-idle time after today's work end, up to Now; 0 when the
// schedule is unset, today is a day off or the work end has not come.
func OverMS(in Input) int64 {
	if !in.Hours.Set || in.Hours.Off(in.Day) {
		return 0
	}
	_, to := in.Hours.Window(in.Day)
	if in.Now <= to {
		return 0
	}
	return activeSince(in, to)
}

// OverDue reports whether this tick should send the overworking notice: it
// is on, today is a work day of a set schedule, the last one is at least
// Routine.OverRepeatMS old (or from another day), the grace has passed since
// the work end, the user is active now, no call is going on and the active
// time after the work end has reached Routine.OverGraceMS. The cheap
// conditions come first so no views are scanned before the grace could have
// passed.
func OverDue(in Input) bool {
	r := in.Routine
	if !r.Overwork || !in.Hours.Set || in.Hours.Off(in.Day) {
		return false
	}
	if in.State.OverDay == dayKey(in.Day) && in.Now-in.State.OverAt < r.OverRepeatMS {
		return false
	}
	_, to := in.Hours.Window(in.Day)
	if in.Now-to < r.OverGraceMS {
		return false
	}
	if c := current(in); c == nil || isIdle(*c) || inMeeting(in) {
		return false
	}
	return OverMS(in) >= r.OverGraceMS
}

// stepOver sends the overworking notice. The caller fills OverBody only when
// OverDue.
func stepOver(in Input, st *State) (Event, bool) {
	if in.OverBody == "" || !OverDue(in) {
		return Event{}, false
	}
	st.OverDay = dayKey(in.Day)
	st.OverAt = in.Now
	return note(TopicOver, TitleOver, in.OverBody, false), true
}
