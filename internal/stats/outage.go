package stats

import "sort"

// Run states and stop reasons, as trackd stores them in trackd_run (the
// store's Run* constants are these).
const (
	StateRecording = "recording"
	StatePaused    = "paused" // config.toml did not load: nothing recorded

	ReasonSignal  = "signal"   // stopped on purpose (SIGTERM/SIGINT)
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
	GapAsleep     = "asleep"     // the computer slept (or trackd was suspended)
	GapOff        = "off"        // the computer was off or restarted
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

// Gap is time in which nothing was recorded, and why.
type Gap struct {
	From, To int64
	Kind     string
}

// NotRecording says the gap is tock's to explain: trackd should have been
// recording and was not. Sleep and a computer that was off are not.
func (g Gap) NotRecording() bool { return g.Kind != GapAsleep && g.Kind != GapOff }

// Gaps returns, in order, the time in [from, min(to, now)) that no
// recording run covers and the time in paused runs, each with its kind.
// runs come from the store with one neighbour outside the range on each
// side; time before the first run is unknown and not a gap.
func Gaps(runs []Stretch, from, to, now int64) []Gap {
	runs = append([]Stretch(nil), runs...)
	sort.SliceStable(runs, func(i, j int) bool { return runs[i].Start < runs[j].Start })
	lim := min(to, now)
	var out []Gap
	add := func(a, b int64, kind string) {
		a, b = max(a, from), min(b, lim)
		if b > a {
			out = append(out, Gap{a, b, kind})
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
			add(covered, r.Start, gapAfter(prev, r))
		}
		if r.State == StatePaused {
			add(r.Start, end, GapConfig)
		}
		if prev == nil || end > covered {
			covered = end
		}
		prev = r
	}
	if prev != nil && covered < lim {
		add(covered, lim, gapTail(prev))
	}
	return out
}

// rebooted says the computer started again between run a and run b. Within
// one boot the boot time is the same; a later boot is after a's last tick.
func rebooted(a, b *Stretch) bool {
	return a.Boot > 0 && b.Boot > 0 && b.Boot-a.Boot > rebootSlackMS && b.Boot >= a.Last-rebootSlackMS
}

// gapAfter is the kind of the gap between run a and the next run b.
func gapAfter(a, b *Stretch) string {
	if rebooted(a, b) {
		return GapOff
	}
	switch a.Reason {
	case "", ReasonCrash:
		return GapCrash
	case ReasonAsleep:
		return GapAsleep
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

// Outages are the gaps that are not recording (see NotRecording), with
// outages less than MinOutageMS apart joined (a crash loop is one outage,
// of the longer part's kind) and those shorter than MinOutageMS dropped.
func Outages(gaps []Gap) []Gap {
	var out []Gap
	for _, g := range gaps {
		if !g.NotRecording() {
			continue
		}
		if n := len(out); n > 0 && g.From-out[n-1].To < MinOutageMS {
			last := &out[n-1]
			if g.To-g.From > last.To-last.From {
				last.Kind = g.Kind
			}
			last.To = max(last.To, g.To)
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
