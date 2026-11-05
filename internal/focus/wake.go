package focus

import (
	"fmt"

	"tock/internal/classify"
)

const (
	// WakeWorkMS is how long work has to last to end an off-task run.
	WakeWorkMS = 60_000
	// WakeBackMS is how soon after the wake screen work has to come back for
	// it to count as woken.
	WakeBackMS = 2 * 60_000
	// WakeMax is how many wake screens a session gets at most.
	WakeMax = 2
)

// Wake notices.
const (
	TitleWake     = "Off task"
	TitleWakeWarn = "tock · off task"
	// TopicWake is the full-screen wake notice; the menu bar app shows it
	// over every screen.
	TopicWake = "wake"
	// TopicWakeWarn is the banner that comes before it.
	TopicWakeWarn = "wakewarn"
)

// OffRun is the off-task run of a session at Now.
type OffRun struct {
	From  int64 // start of the first off-task time after the last work; 0 = none
	OffMS int64 // off-task time counted since
	Front bool  // the view in front now is off task
	Idle  bool  // with Front: it is idle
}

// isWork reports deep or shallow time outside meetings.
func isWork(in Input, i int) bool {
	k := in.Labels[i].Kind
	return !isIdle(in.Views[i]) && !IsMeeting(in.Views[i], in.Cfg) && (k == classify.KindDeep || k == classify.KindShallow)
}

// offTask measures the off-task run of session s. Work of WakeWorkMS or
// more ends a run. Distraction that may alert, browsing and idle stretches
// of [focus] wake_idle or more (never on paper) count; anything else,
// meetings included, neither counts nor ends a run.
func offTask(in Input, s Session, paper bool) OffRun {
	var r OffRun
	var workRun, idleRun, idleFrom int64
	last := len(in.Views) - 1
	for i, v := range in.Views {
		from, to := max(v.Start, s.StartMS), min(v.End, in.Now)
		if to <= from {
			continue
		}
		d := to - from
		front := false
		switch {
		case isIdle(v):
			workRun = 0
			if idleRun == 0 {
				idleFrom = from
			}
			before := idleRun
			idleRun += d
			w := in.Cfg.WakeIdleMS
			if paper || idleRun < w {
				break
			}
			if before >= w {
				r.OffMS += d
			} else {
				if r.OffMS == 0 {
					r.From = idleFrom
				}
				r.OffMS += idleRun
			}
			front = true
		case IsMeeting(v, in.Cfg):
			workRun, idleRun = 0, 0
		case isWork(in, i):
			idleRun = 0
			workRun += d
			if workRun >= WakeWorkMS {
				r.From, r.OffMS = 0, 0
			}
		case in.Labels[i].Kind == classify.KindBrowse || in.Labels[i].Kind == classify.KindDistract && !in.Labels[i].Quiet:
			workRun, idleRun = 0, 0
			if r.OffMS == 0 {
				r.From = from
			}
			r.OffMS += d
			front = true
		default:
			workRun, idleRun = 0, 0
		}
		if i == last && current(in) != nil {
			r.Front, r.Idle = front, front && isIdle(v)
		}
	}
	return r
}

// wakeTarget is the label "This is work" files the front view under: the
// deep label with the most time in the session.
func wakeTarget(in Input, s Session) classify.Want {
	ms := map[classify.Want]int64{}
	var best classify.Want
	for i, v := range in.Views {
		l := in.Labels[i]
		if l.Kind != classify.KindDeep || l.Category == "" || isIdle(v) || IsMeeting(v, in.Cfg) {
			continue
		}
		w := classify.Want{Category: l.Category, Project: l.Project}
		ms[w] += max(min(v.End, in.Now)-max(v.Start, s.StartMS), 0)
		if ms[w] > ms[best] {
			best = w
		}
	}
	return best
}

// stepWake checks the wake screen's answer, then sends the off-task banner
// at [focus] wake_warn and the wake screen at wake_after, again every
// wake_after while the run goes on, WakeMax a session. Calls, a distraction
// snooze and a work view in front hold both back.
func stepWake(in Input, s Session, st *State) (Session, []Event) {
	if !in.Cfg.WakeScreen {
		return s, nil
	}
	n := len(in.Views)
	front := n > 0 && current(in) != nil
	if st.WakeCheck > 0 {
		switch {
		case in.Now-st.WakeCheck > WakeBackMS:
			st.WakeCheck = 0
		case front && isWork(in, n-1):
			s.WakesBack++
			st.WakeCheck = 0
		}
	}
	r := offTask(in, s, in.State.PaperSession == s.ID)
	if r.OffMS == 0 || !r.Front || inMeeting(in) || in.Now < in.State.DistractSnoozeUntil {
		return s, nil
	}
	if st.WakeRun != r.From {
		st.WakeRun, st.WakeWarn, st.WakeAt = r.From, 0, 0
	}
	left := minutes(s.PlannedMS - in.Now)
	mark := func(topic, title, body string) Event {
		e := note(topic, title, body, true)
		e.OffFrom, e.Idle, e.Sess = r.From, r.Idle, s
		if !r.Idle {
			e.Key = classify.Key(in.Views[n-1])
			e.Target = wakeTarget(in, s)
		}
		return e
	}
	switch {
	case r.OffMS >= in.Cfg.WakeAfterMS && s.Wakes < WakeMax && (st.WakeAt == 0 || in.Now-st.WakeAt >= in.Cfg.WakeAfterMS):
		st.WakeAt, st.WakeCheck, st.WakeWarn = in.Now, in.Now, max(st.WakeWarn, in.Now)
		s.Wakes++
		e := mark(TopicWake, TitleWake, fmt.Sprintf("%d min off task · %d min left of your %d min session.",
			minutes(r.OffMS), left, minutes(s.PlannedMS-s.StartMS)))
		e.Sess = s
		return s, []Event{e}
	case r.OffMS >= in.Cfg.WakeWarnMS && st.WakeWarn == 0 && st.WakeAt == 0:
		st.WakeWarn = in.Now
		if alertable(in, n-1) {
			return s, nil // the distraction notice covers it
		}
		return s, []Event{mark(TopicWakeWarn, TitleWakeWarn, fmt.Sprintf("%d min off task · %d min left.", minutes(r.OffMS), left))}
	}
	return s, nil
}
