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
