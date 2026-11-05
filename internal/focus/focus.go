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
	"tock/internal/recommend"
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
	// TitleStart is the start-of-day notice.
	TitleStart = "Start of day"
	// TitleMidday is the midday check-in notice.
	TitleMidday = "Midday check-in"
	// TitleOver is the overworking notice.
	TitleOver = "Overworking"
	// TitleBreakOver is the notice that the break the user took has ended.
	TitleBreakOver = "Break over"
	// TitleBreakScreen is the break screen's title.
	TitleBreakScreen = "Back to it?"
)

// Notification topics: what a note is about, so a notifier can offer the
// right buttons.
const (
	TopicFocus  = "focus"
	TopicDone   = "done"
	TopicBreak  = "break"
	TopicRecap  = "recap"
	TopicAway   = "away"
	TopicStart  = "start"
	TopicMidday = "midday"
	// TopicOver is the overworking notice's topic.
	TopicOver = "overwork"
	// TopicBreakOver is the Break over notice's topic; it has no buttons.
	TopicBreakOver = "breakover"
	// TopicBreakScreen is the full-screen notice of a break that ran over;
	// the menu bar app shows it over every screen instead of a banner.
	TopicBreakScreen = "breakscreen"
)

// AwayNotifyMS is how soon after coming back the away question is sent; an
// absence that ended earlier is left to the TUI and tock away.
const AwayNotifyMS = 2 * 60_000

// BreakOverLateMS is how long after a taken break's end the Break over notice
// is still worth sending; a trackd that first ticks later sends none.
const BreakOverLateMS = 5 * 60_000

// Session is one focus session. It has the same fields in the same order as
// store.Session, so the two convert directly; keep them in step.
type Session struct {
	ID            int64
	Kind          string
	StartMS       int64
	PlannedMS     int64 // 0 for auto sessions
	EndMS         int64 // 0 while running
	Interruptions int
	AlertedMS     int64 // alertable distraction total at the last alert
	DeepMS        int64
	DistractMS    int64
	LongestMS     int64
	Switches      int    // context switches in the session window; -1 = recorded before migration 0014
	Rating        int    // 1..5, 0 = unrated
	Note          string // the user's one line about the session
	RatedMS       int64  // when it was rated, 0 = unrated
	UrgesResisted int    // distraction notices answered Back to work
	UrgesGiven    int    // distraction notices answered Open anyway
	Wakes         int    // wake screens shown
	WakesBack     int    // wake screens followed by work within WakeBackMS
}

// State is the engine's memory between ticks, kept in daemon_state.
type State struct {
	BreakFrom int64  // start of the activity run the last break reminder belongs to
	BreakAt   int64  // when the last break reminder or session end fired
	RecapDay  string // YYYY-MM-DD of the last recap
	StartDay  string // YYYY-MM-DD of the last start-of-day notice
	MiddayDay string // YYYY-MM-DD of the last midday check-in
	OverDay   string // YYYY-MM-DD of the last overworking notice
	OverAt    int64  // when the last overworking notice was sent
	AwayAt    int64  // end of the last absence asked about
	// SnoozeUntil holds break reminders back until this time (ms).
	SnoozeUntil int64
	// DistractSnoozeUntil holds the running session's distraction notices
	// back until this time (ms); read from daemon_state, never saved by a
	// StateUpdate.
	DistractSnoozeUntil int64
	// BreakTakenAt and BreakUntil are the break the user took from a break
	// reminder: when they took it and when it ends (ms). Read from daemon_state
	// and never saved by a StateUpdate: tock owns them.
	BreakTakenAt, BreakUntil int64
	// BreakOverAt is the BreakUntil the Break over notice was sent (or given up)
	// for; trackd owns it.
	BreakOverAt int64
	// BreakScreenAt is the BreakUntil the break screen was shown (or given
	// up) for; trackd owns it.
	BreakScreenAt int64
	// NudgeDay, Nudges and NudgeAt count the nudges (see nudgeOK): the
	// YYYY-MM-DD Nudges were sent on, and when the last one was sent (ms).
	// They move only while [focus] notice_budget or notice_gap is set.
	NudgeDay string
	Nudges   int
	NudgeAt  int64
	// WakeRun is the start of the off-task run the wake notices below
	// belong to; WakeWarn and WakeAt are when its banner and its last wake
	// screen went out, and WakeCheck when a wake screen still waits to see
	// work come back (0 = none). trackd owns them.
	WakeRun, WakeWarn, WakeAt, WakeCheck int64
	// PaperSession is the session whose idle time never counts as off task
	// (the user works on paper); read from daemon_state, never saved by a
	// StateUpdate: tock owns it.
	PaperSession int64
}

// EventKind says what an Event asks the caller to do.
type EventKind int

const (
	Notify        EventKind = iota + 1 // send Title and Body
	SessionUpdate                      // write Sess's counters (running session)
	SessionEnd                         // close Sess at Sess.EndMS
	AutoSession                        // insert Sess as an auto session
	StateUpdate                        // save State
	// SlotDue is a slot notice; the caller marks Slot.Key and SlotKind sent
	// and sends it as a Notify only when the mark is new.
	SlotDue
	// RecDue is a recommendation notice; the caller writes the recommend row
	// and sends it as a Notify carrying RecID.
	RecDue
)

// Event is one thing a tick decided.
type Event struct {
	Kind        EventKind
	Title, Body string
	Topic       string            // a Notify's topic: TopicFocus, TopicDone, …
	OfSession   bool              // a Notify about the running session; drop it when that session was closed meanwhile
	Minutes     int               // TopicDone: the finished session's planned length
	Project     string            // TopicDone: the finished session's top project, "" = none
	Refile      bool              // TopicDone: a refile of the session's time to Project is offered
	Away        classify.Interval // TopicAway: the absence asked about
	Before      classify.Want     // TopicAway: the label of what it follows, if any
	Break       classify.Interval // TopicBreakScreen: the break taken, from its start to its planned end
	// OffFrom, Idle, Key and Target (TopicWake, TopicWakeWarn): the start of
	// the off-task run, whether it is idle in front, the front view's
	// classify.Key and the label This is work gives it ("" or zero = none).
	OffFrom int64
	Idle    bool
	Key     string
	Target  classify.Want
	// Category and RuleRef (TopicFocus): the category the notice offers to
	// mark quiet, and the rule that set it ("" = none).
	Category, RuleRef string
	Urge              bool // TopicFocus: the notice is an urge-surfing prompt
	Sess              Session
	State             State
	// Slot and SlotKind (SlotDue): the slot and the notice kind it is due for.
	Slot     Slot
	SlotKind string
	// AutoStart (SlotDue, starting notice of a plan slot): the caller starts
	// Sess once the slot is marked. Body then says so; AltBody is the body to
	// send instead when the start finds a session running.
	AutoStart bool
	AltBody   string
	// Silent (SlotDue): mark and start only, send nothing ([plan] notify = false).
	Silent bool
	// RecKind and RecLengthMS (RecDue): the recommendation's kind and the
	// session or break length it offers. RecID is the recommend row the
	// caller wrote for it.
	RecKind     string
	RecLengthMS int64
	RecID       int64
	// RecAtMS is the send time (ms) of that row.
	RecAtMS int64
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
	// Routine holds the [routine] switches; the app passes cfg.Routine.
	Routine config.Routine
	// Hours is the work schedule; the zero value makes every day a work day.
	Hours stats.Hours
	// StartBody is the start-of-day text; the caller fills it when StartDue.
	StartBody string
	// MiddayBody is the midday check-in text; the caller fills it when MiddayDue.
	MiddayBody string
	// OverBody is the overworking text; the caller fills it when OverDue.
	OverBody string
	// Absence is the latest absence today that has no answer yet, or nil.
	Absence *classify.Interval
	// AbsenceBefore is the label of what Absence follows; zero when none.
	AbsenceBefore classify.Want
	// Weights are the quality weights of the done notice; the zero value
	// means DefaultQualityWeights().
	Weights Weights
	// Plan holds the [plan] notice switches; the app passes cfg.Plan.
	Plan config.Plan
	// Slots are today's slots in start order.
	Slots []Slot
	// SlotSent holds the SlotSentKey of the slot notices already sent; the
	// caller fills it only when SlotsDue.
	SlotSent map[string]bool
	// OtherInputMS are the latest view ends of the other synced machines
	// (RemoteMachine.LastMS); the caller fills it only when SlotsDue. Empty
	// = no other machine, this one owns every slot notice.
	OtherInputMS []int64
	// Rec holds the [recommend] switches; the app passes cfg.Recommend.
	Rec config.Recommend
	// Profile is today's profile; the zero value suggests nothing.
	Profile recommend.Profile
	// Recs are the recommendation rows of the last recommend.ReplyDays and
	// RecsOK says they were read; the caller fills both only when RecsDue.
	Recs   []recommend.Rec
	RecsOK bool
	// RecSlots are today's slots for the free-moment rule of a focus
	// suggestion; the caller fills them only when RecommendDue, from the same
	// source as Slots but independent of the [plan] notice switches.
	RecSlots []Slot
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
	AlertMS    int64 // the part of DistractMS that may alert: Quiet labels left out
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
			if !labels[i].Quiet {
				st.AlertMS += e - s
			}
		}
	}
	return st
}
