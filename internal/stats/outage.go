package stats

import "sort"

// Run states and stop reasons, as trackd stores them in trackd_run (the
// store's Run* constants are these).
const (
	StateRecording = "recording"
	StatePaused    = "paused"    // config.toml did not load: nothing recorded
	StateNoAccess  = "no_access" // Accessibility is off: apps are recorded, window titles are not

	ReasonSignal  = "signal"   // stopped by a signal the OS did not say was a session ending
	ReasonSession = "session"  // the OS said the user's session was ending: logout, restart or shutdown
	ReasonAsleep  = "asleep"   // trackd's own ticks stopped: the computer slept
	ReasonState   = "state"    // capture paused or resumed; the next row follows at once
	ReasonClock   = "clock"    // the wall clock went back
	ReasonAXStale = "ax_stale" // the Accessibility grant went stale; restarting
	ReasonUpdate  = "update"   // replaced by a new build
	ReasonError   = "error"    // a source or write error; restarting
	ReasonCrash   = "crash"    // ended without saying why
)

// Gap kinds: why nothing was recorded in a gap.
const (
	GapCrash      = "crash"      // trackd ended without a clean stop and started again later
	GapDown       = "down"       // trackd is not running now; its last run ended without a clean stop
	GapStopped    = "stopped"    // trackd was stopped on purpose
	GapConfig     = "config"     // trackd ran but config.toml did not load
	GapPermission = "permission" // trackd restarted because Accessibility went stale
	GapUpdate     = "update"     // trackd restarted into a new build
	GapError      = "error"      // trackd stopped on an error and restarted
	GapNoAccess   = "no_access"  // Accessibility was off: apps recorded, window titles not
	GapAsleep     = "asleep"     // the computer slept (or trackd was suspended)
	GapOff        = "off"        // the computer was off or restarted
	GapLoggedOut  = "logged_out" // the user's session had ended (logout)
)

// LiveMS is how old the open run's last tick may be while trackd still
// counts as running: two missed minute ticks and some slack.
const LiveMS int64 = 150_000

// MinOutageMS is the shortest outage worth showing; a restart that takes
// seconds is not. Outages closer than this are joined.
const MinOutageMS int64 = 60_000

// rebootSlackMS is how far two readings of one boot time may differ.
const rebootSlackMS int64 = 2_000

// Stretch is one trackd_run row. Stop 0 = open: running, or a trackd that
// died and has not been followed by a new run yet.
type Stretch struct {
	Start, Last, Stop int64
	Reason, State     string
	Boot              int64 // the computer's boot time; 0 = unknown
}

// Gap is time in which nothing (or, Partial, less) was recorded, and why.
type Gap struct {
	From, To int64
	Kind     string
	// EndUnknown: trackd stopped and the computer started again before
	// trackd did. To is the new boot; the computer may have been off for
	// part of the gap, and nothing says when it went off.
	EndUnknown bool
}

// NotRecording says the gap is tock's to explain: trackd should have been
// recording and was not. Sleep, a computer that was off, a session that
// had ended and apps-only time are not.
func (g Gap) NotRecording() bool {
	switch g.Kind {
	case GapAsleep, GapOff, GapLoggedOut, GapNoAccess:
		return false
	}
	return true
}

// Partial says trackd recorded, but less than it should: apps without
// their window titles while Accessibility was off.
func (g Gap) Partial() bool { return g.Kind == GapNoAccess }

// Gaps returns, in order, the time in [from, min(to, now)) that no
// recording run covers and the time in paused runs, each with its kind.
// runs come from the store with one neighbour outside the range on each
// side; time before the first run is unknown and not a gap.
func Gaps(runs []Stretch, from, to, now int64) []Gap {
	runs = append([]Stretch(nil), runs...)
	sort.SliceStable(runs, func(i, j int) bool { return runs[i].Start < runs[j].Start })
	lim := min(to, now)
	var out []Gap
	put := func(a, b int64, kind string, unknownEnd bool) {
		a, b = max(a, from), min(b, lim)
		if b > a {
			out = append(out, Gap{From: a, To: b, Kind: kind, EndUnknown: unknownEnd})
		}
	}
	var prev *Stretch
	var covered int64
	for i := range runs {
		r := &runs[i]
		end := r.Stop
		if end == 0 {
			end = r.Last
			if i == len(runs)-1 && now-r.Last <= LiveMS {
				end = now
			}
		}
		if prev != nil && r.Start > covered {
			between(prev, r, covered, put)
		}
		switch r.State {
		case StatePaused:
			put(r.Start, end, GapConfig, false)
		case StateNoAccess:
			put(r.Start, end, GapNoAccess, false)
		}
		if prev == nil || end > covered {
			covered = end
		}
		prev = r
	}
	if prev != nil && covered < lim {
		put(covered, lim, gapTail(prev), false)
	}
	return out
}

// rebooted says the computer started again between run a and run b. Within
// one boot the boot time is the same; a later boot is after a's last tick.
func rebooted(a, b *Stretch) bool {
	return a.Boot > 0 && b.Boot > 0 && b.Boot-a.Boot > rebootSlackMS && b.Boot >= a.Last-rebootSlackMS
}

// between puts the gap from `from` to the start of run b, after run a.
//
// A new boot explains the gap only when the OS said, as trackd stopped,
// that the session was ending (ReasonSession), or when the computer had
// gone to sleep. Any other stop (a signal from a person or a script, a
// crash, an exit to update or restart) leaves trackd down while the
// computer may have stayed on for hours before it went off: up to the new
// boot that is an outage whose end is unknown; only boot to start is off.
func between(a, b *Stretch, from int64, put func(a, b int64, kind string, unknownEnd bool)) {
	switch {
	case a.Reason == ReasonSession || a.Reason == ReasonAsleep:
		kind := GapAsleep
		switch {
		case rebooted(a, b):
			kind = GapOff
		case a.Reason == ReasonSession:
			kind = GapLoggedOut
		}
		put(from, b.Start, kind, false)
	case rebooted(a, b):
		boot := min(max(b.Boot, from), b.Start)
		put(from, boot, stopKind(a), true)
		put(boot, b.Start, GapOff, false)
	default:
		put(from, b.Start, stopKind(a), false)
	}
}

// stopKind is the outage kind after run a stopped for its reason.
func stopKind(a *Stretch) string {
	switch a.Reason {
	case "", ReasonCrash:
		return GapCrash
	case ReasonAXStale:
		return GapPermission
	case ReasonUpdate:
		return GapUpdate
	case ReasonError:
		return GapError
	}
	return GapStopped
}

// gapTail is the kind of the gap after the last run, up to now.
func gapTail(a *Stretch) string {
	switch a.Reason {
	case ReasonSignal:
		return GapStopped
	case ReasonAsleep:
		return GapAsleep
	}
	return GapDown
}

// Outages are the gaps that are not recording (see NotRecording) and the
// apps-only ones (Partial), with gaps of the same sort less than
// MinOutageMS apart joined (a crash loop is one outage, of the longer
// part's kind) and those shorter than MinOutageMS dropped. Apps-only time
// is never joined with time nothing was recorded in.
func Outages(gaps []Gap) []Gap {
	var out []Gap
	for _, g := range gaps {
		if !g.NotRecording() && !g.Partial() {
			continue
		}
		if n := len(out); n > 0 && g.From-out[n-1].To < MinOutageMS && g.Partial() == out[n-1].Partial() {
			last := &out[n-1]
			if g.To-g.From > last.To-last.From {
				last.Kind = g.Kind
			}
			last.To = max(last.To, g.To)
			last.EndUnknown = last.EndUnknown || g.EndUnknown
			continue
		}
		out = append(out, g)
	}
	kept := out[:0]
	for _, g := range out {
		if g.To-g.From >= MinOutageMS {
			kept = append(kept, g)
		}
	}
	return kept
}
