// Package focus is the focus engine: distraction alerts in sessions, break
// reminders, auto sessions and the daily recap. It is pure: every function
// takes the time it runs at; app builds the input and applies the events.
package focus

import (
	"slices"
	"strings"
	"time"

	"tock/internal/classify"
	"tock/internal/config"
	"tock/internal/span"
	"tock/internal/stats"
)

// Session kinds.
const (
	KindManual = "manual"
	KindAuto   = "auto"
)

const (
	// IdleResetMS is how long idle, or a gap between spans, has to last to
	// end an activity run.
	IdleResetMS = 5 * 60_000
	// RecapMinActiveMS is the active time a day needs to get a recap.
	RecapMinActiveMS = 60_000
)

// Notification titles.
const (
	TitleFocus = "tock · focus"
	TitleDone  = "tock · session done"
	TitleBreak = "tock · break"
	TitleRecap = "tock · recap"
	TitleAway  = "tock · away"
)

// Notification topics: what a note is about, so a notifier can offer the
// right buttons.
const (
	TopicFocus = "focus"
	TopicDone  = "done"
	TopicBreak = "break"
	TopicRecap = "recap"
	TopicAway  = "away"
)

// AwayNotifyMS is how soon after coming back the away question is sent; an
// absence that ended earlier is left to the TUI and tock away.
const AwayNotifyMS = 2 * 60_000

// Session is one focus session. It has the same fields in the same order as
// store.Session, so the two convert directly; keep them in step.
type Session struct {
	ID            int64
	Kind          string
	StartMS       int64
	PlannedMS     int64 // 0 for auto sessions
	EndMS         int64 // 0 while running
	Interruptions int
	AlertedMS     int64 // distraction total at the last alert
	DeepMS        int64
	DistractMS    int64
	LongestMS     int64
}

// State is the engine's memory between ticks, kept in daemon_state.
type State struct {
	BreakFrom int64  // start of the activity run the last break reminder belongs to
	BreakAt   int64  // when the last break reminder or session end fired
	RecapDay  string // YYYY-MM-DD of the last recap
	AwayAt    int64  // end of the last absence asked about
	// SnoozeUntil holds break reminders back until this time (ms).
	SnoozeUntil int64
}

// EventKind says what an Event asks the caller to do.
type EventKind int

const (
	Notify        EventKind = iota + 1 // send Title and Body
	SessionUpdate                      // write Sess's counters (running session)
	SessionEnd                         // close Sess at Sess.EndMS
	AutoSession                        // insert Sess as an auto session
	StateUpdate                        // save State
)

// Event is one thing a tick decided.
type Event struct {
	Kind        EventKind
	Title, Body string
	Topic       string            // a Notify's topic: TopicFocus, TopicDone, …
	OfSession   bool              // a Notify about the running session; drop it when that session was closed meanwhile
	Minutes     int               // TopicDone: the finished session's planned length
	Away        classify.Interval // TopicAway: the absence asked about
	Before      classify.Want     // TopicAway: the label of what it follows, if any
	Sess        Session
	State       State
}

// Input is one tick's view of the world.
type Input struct {
	Now       int64
	Day       time.Time           // local midnight of Now's day; its Location is the user's zone
	Views     []classify.SpanView // start order, from Day (or the running session's start, if earlier) to Now; the open span ends at Now
	Labels    []classify.Label    // Labels[i] labels Views[i]
	Sessions  []Session           // sessions overlapping the Views' range, the running one included
	Call      bool                // a camera or microphone is in use now
	State     State
	Cfg       config.Focus
	RecapBody string // the recap text; the caller fills it when RecapDue
	// Absence is the latest absence today that has no answer yet, or nil.
	Absence *classify.Interval
	// AbsenceBefore is the label of what Absence follows; zero when none.
	AbsenceBefore classify.Want
}

// IsMeeting reports a meeting view: a meeting app, or a URL whose host is a
// meeting host or one of its subdomains.
func IsMeeting(v classify.SpanView, cfg config.Focus) bool {
	if slices.Contains(cfg.MeetingApps, v.BundleID) {
		return true
	}
	if v.URL == nil {
		return false
	}
	host, _, _ := strings.Cut(strings.ToLower(*v.URL), "/")
	for _, h := range cfg.MeetingHosts {
		h = strings.ToLower(h)
		if host == h || strings.HasSuffix(host, "."+h) {
			return true
		}
	}
	return false
}

// isIdle reports time away from the screen: idle, or labelled away (so
// labelling a break never sets off a break reminder).
func isIdle(v classify.SpanView) bool {
	return v.BundleID == span.IdleBundleID || v.BundleID == span.AwayBundleID
}

// current is the view open at Now, or nil when trackd recorded nothing up to
// Now.
func current(in Input) *classify.SpanView {
	if n := len(in.Views); n > 0 && in.Views[n-1].End >= in.Now {
		return &in.Views[n-1]
	}
	return nil
}

// inMeeting reports a call now: a camera or microphone in use, or a meeting
// view in front.
func inMeeting(in Input) bool {
	if in.Call {
		return true
	}
	c := current(in)
	return c != nil && IsMeeting(*c, in.Cfg)
}

// Stats is the focus time of a window.
type Stats struct {
	DeepMS     int64 // deep time outside meetings
	DistractMS int64 // distraction time outside meetings
	LongestMS  int64 // longest deep stretch; non-deep gaps under stats.ToleranceMS do not break it, idle does
}

// WindowStats measures the views clipped to [from, to). Meeting views count
// as neither deep nor distraction.
func WindowStats(views []classify.SpanView, labels []classify.Label, from, to int64, cfg config.Focus) Stats {
	var st Stats
	var run, runEnd int64
	inRun := false
	for i, v := range views {
		s, e := max(v.Start, from), min(v.End, to)
		if e <= s {
			continue
		}
		if isIdle(v) {
			inRun = false
			continue
		}
		if IsMeeting(v, cfg) {
			continue
		}
		switch labels[i].Kind {
		case classify.KindDeep:
			if inRun && s-runEnd >= stats.ToleranceMS {
				inRun = false
			}
			if !inRun {
				inRun, run = true, 0
			}
			run += e - s
			runEnd = e
			st.DeepMS += e - s
			st.LongestMS = max(st.LongestMS, run)
		case classify.KindDistract:
			st.DistractMS += e - s
		}
	}
	return st
}
