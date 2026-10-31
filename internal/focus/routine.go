package focus

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
