package focus

import (
	"fmt"
	"strings"
	"time"

	"tock/internal/classify"
	"tock/internal/recommend"
)

// Recommendation notice copy.
const (
	TitleRecommend = "Good time to focus"
	TopicRecommend = "recommend"
)

const (
	// RecGapMS is the least time between two focus suggestions.
	RecGapMS = 60 * 60_000
	// RecMinMS is the shortest session worth suggesting (CONTEXT: skip if
	// the length is under 15 min).
	RecMinMS = 15 * 60_000
	// RecActiveMS of the last RecActiveWindowMS must be this machine's own
	// activity for a suggestion: the user is at the keyboard, not away.
	RecActiveMS       = 5 * 60_000
	RecActiveWindowMS = 10 * 60_000
	// RecBreakGapMS is how long a focus suggestion keeps away from a break: after
	// a break reminder or the end-of-session break prompt, and after a break
	// the user took has ended.
	RecBreakGapMS = 15 * 60_000
	// recRoundMS is the unit a suggested length is floored to.
	recRoundMS = 5 * 60_000
)

// RecommendDue reports that a focus suggestion could be due now, from the
// cheap conditions only, so an idle tick scans no view and reads no row:
// recommendations and suggestions are on, [focus] notify is on, now is in
// the work window of a work day, the hour is a top hour of today's weekday
// and no manual session runs. The caller then reads what stepRecommend needs.
func RecommendDue(in Input) bool {
	if !in.Rec.Enabled || in.Rec.MaxPerDay <= 0 || !in.Cfg.Notify {
		return false
	}
	if !in.Hours.Set || in.Hours.Off(in.Day) {
		return false
	}
	if from, to := in.Hours.Window(in.Day); in.Now < from || in.Now >= to {
		return false
	}
	t := time.UnixMilli(in.Now).In(in.Day.Location())
	if in.Profile.Rank(t.Weekday(), t.Hour()) == 0 {
		return false
	}
	if in.Now < in.State.BreakUntil {
		return false // the user is on a break they took
	}
	return openSession(in) == nil
}

// RecsDue reports that the caller must read the recommendation rows (and,
// with them, whatever else Step needs) before calling Step: a focus suggestion
// could be due, or a break reminder is due and the rows tell whether its hour
// is muted.
func RecsDue(in Input) bool {
	if RecommendDue(in) {
		return true
	}
	if !in.Rec.Enabled || !in.Cfg.Notify || openSession(in) != nil {
		return false
	}
	_, due := breakDue(in, in.State)
	return due
}

// stepRecommend suggests a focus session at a top focus hour (REC-02). It is
// due, beyond RecommendDue, when all of these hold (12-CONTEXT "Focus
// suggestion"):
//   - the rows were read, fewer than [recommend] max_per_day focus suggestions
//     went out today and the last one was at least RecGapMS ago;
//   - no call or meeting view (inMeeting) and no distraction snooze;
//   - the user is active: the front view is this machine's own and not idle,
//     and at least RecActiveMS of the last RecActiveWindowMS is this machine's
//     own activity (the rule LocalInputEnd uses, so with two synced machines
//     only the one in use suggests);
//   - no plan item or calendar event is running (RecSlots);
//   - no break is due, none was prompted in the last RecBreakGapMS and none the
//     user took ended in that time (st is the state after this tick's break
//     steps), so a suggestion never lands beside a break notice;
//   - the replies allow the hour (recommend.FocusHour): not one always
//     answered Not now, and one rarely taken only when it ranks first.
//
// The length is the profile's typical session capped by the time to the next
// slot and to the [hours] end, floored to 5 min; it is skipped under RecMinMS.
func stepRecommend(in Input, st State) (Event, bool) {
	if !RecommendDue(in) || !in.RecsOK || nearBreak(in, st) {
		return Event{}, false
	}
	t := time.UnixMilli(in.Now).In(in.Day.Location())
	if !recommend.FocusHour(in.Profile, recommend.Learn(in.Recs, in.Now), t.Weekday(), t.Hour()) {
		return Event{}, false // an hour the replies muted, or one that is rarely taken and not the best
	}
	if recommend.CountSince(in.Recs, recommend.KindFocus, in.Day.UnixMilli()) >= in.Rec.MaxPerDay {
		return Event{}, false
	}
	if last := recommend.Last(in.Recs, recommend.KindFocus); last != 0 && in.Now-last < RecGapMS {
		return Event{}, false
	}
	if inMeeting(in) || in.Now < in.State.DistractSnoozeUntil {
		return Event{}, false
	}
	if cur := current(in); cur == nil || isIdle(*cur) || !ownView(*cur) {
		return Event{}, false
	}
	if ownActiveMS(in.Views, in.Now-RecActiveWindowMS, in.Now) < RecActiveMS {
		return Event{}, false
	}
	n := in.Profile.SessionMS
	for _, s := range in.RecSlots {
		switch {
		case s.StartMS <= in.Now && in.Now < s.EndMS:
			return Event{}, false
		case s.StartMS > in.Now:
			n = min(n, s.StartMS-in.Now)
		}
	}
	_, to := in.Hours.Window(in.Day)
	n = min(n, to-in.Now)
	n -= n % recRoundMS
	if n < RecMinMS {
		return Event{}, false
	}
	body := fmt.Sprintf("Your best focus hour · start a %d min session now?", n/60_000)
	ev := note(TopicRecommend, TitleRecommend, body, false)
	ev.Kind = RecDue
	ev.Minutes = int(n / 60_000)
	ev.RecKind, ev.RecLengthMS = recommend.KindFocus, n
	return ev, true
}

// nearBreak reports that a break reminder is due, that one (or the prompt at
// the end of a session) went out less than RecBreakGapMS ago, or that a break
// the user took ended less than RecBreakGapMS ago.
func nearBreak(in Input, st State) bool {
	if _, due := breakDue(in, st); due {
		return true
	}
	if st.BreakAt > 0 && in.Now-st.BreakAt < RecBreakGapMS {
		return true
	}
	return st.BreakUntil > 0 && in.Now >= st.BreakUntil && in.Now-st.BreakUntil < RecBreakGapMS
}

// ownView reports a view of this machine's own screen: a positive ID and a
// bundle that does not start with "tock." (the rule LocalInputEnd uses).
func ownView(v classify.SpanView) bool {
	return v.ID > 0 && !strings.HasPrefix(v.BundleID, "tock.")
}

// ownActiveMS is the time in [from, to) this machine's own non-idle views
// cover.
func ownActiveMS(views []classify.SpanView, from, to int64) int64 {
	var sum int64
	for _, v := range views {
		if !ownView(v) || isIdle(v) {
			continue
		}
		if e, s := min(v.End, to), max(v.Start, from); e > s {
			sum += e - s
		}
	}
	return sum
}
