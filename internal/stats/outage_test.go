package stats

import (
	"reflect"
	"testing"
)

const (
	mi = int64(60_000)
	hr = 60 * mi
)

// g is a gap; unknownEnd one that ends at a new boot, the computer
// perhaps off for part of it.
func g(from, to int64, kind string) Gap { return Gap{From: from, To: to, Kind: kind} }

func unknownEnd(from, to int64, kind string) Gap {
	return Gap{From: from, To: to, Kind: kind, EndUnknown: true}
}

// closed is a run from start to stop that ended for reason.
func closed(start, stop int64, reason string) Stretch {
	return Stretch{Start: start, Last: stop, Stop: stop, Reason: reason, State: StateRecording, Boot: 1}
}

func TestGaps(t *testing.T) {
	now := 24 * hr
	tests := []struct {
		name string
		runs []Stretch
		want []Gap
	}{
		{"nothing known", nil, nil},
		{"one live run covers the day",
			[]Stretch{{Start: 0, Last: now - 30_000, State: StateRecording, Boot: 1}}, nil},
		{"a crash and a restart 5 min later",
			[]Stretch{closed(0, 10*hr, ReasonCrash), {Start: 10*hr + 5*mi, Last: now, State: StateRecording, Boot: 1}},
			[]Gap{g(10*hr, 10*hr+5*mi, GapCrash)}},
		{"a run left open and a later one: the open one crashed at its last tick",
			[]Stretch{{Start: 0, Last: 10 * hr, State: StateRecording, Boot: 1}, {Start: 11 * hr, Last: now, State: StateRecording, Boot: 1}},
			[]Gap{g(10*hr, 11*hr, GapCrash)}},
		{"stopped on purpose",
			[]Stretch{closed(0, 9*hr, ReasonSignal), {Start: 10 * hr, Last: now, State: StateRecording, Boot: 1}},
			[]Gap{g(9*hr, 10*hr, GapStopped)}},
		{"asleep is a gap but not an outage",
			[]Stretch{closed(0, 9*hr, ReasonAsleep), {Start: 10 * hr, Last: now, State: StateRecording, Boot: 1}},
			[]Gap{g(9*hr, 10*hr, GapAsleep)}},
		// A reboot explains a gap only when the OS said the session was
		// ending as trackd stopped. Otherwise trackd stopped while the
		// computer may still have been on: the time up to the new boot is
		// an outage whose end is unknown, and only boot to start is off.
		{"session ended (shutdown or restart), then a new boot: off",
			[]Stretch{closed(0, 9*hr, ReasonSession), {Start: 10 * hr, Last: now, State: StateRecording, Boot: 10*hr - mi}},
			[]Gap{g(9*hr, 10*hr, GapOff)}},
		{"session ended (logout), same boot: logged out, not an outage",
			[]Stretch{closed(0, 9*hr, ReasonSession), {Start: 10 * hr, Last: now, State: StateRecording, Boot: 1}},
			[]Gap{g(9*hr, 10*hr, GapLoggedOut)}},
		{"stopped by hand, then a new boot: an outage up to the boot",
			[]Stretch{closed(0, 9*hr, ReasonSignal), {Start: 18 * hr, Last: now, State: StateRecording, Boot: 18*hr - mi}},
			[]Gap{unknownEnd(9*hr, 18*hr-mi, GapStopped), g(18*hr-mi, 18*hr, GapOff)}},
		{"a crash, then a new boot: an outage up to the boot",
			[]Stretch{closed(0, 9*hr, ReasonCrash), {Start: 18 * hr, Last: now, State: StateRecording, Boot: 18*hr - mi}},
			[]Gap{unknownEnd(9*hr, 18*hr-mi, GapCrash), g(18*hr-mi, 18*hr, GapOff)}},
		{"left open (killed), then a new boot: an outage up to the boot",
			[]Stretch{{Start: 0, Last: 9 * hr, State: StateRecording, Boot: 1}, {Start: 18 * hr, Last: now, State: StateRecording, Boot: 18*hr - mi}},
			[]Gap{unknownEnd(9*hr, 18*hr-mi, GapCrash), g(18*hr-mi, 18*hr, GapOff)}},
		{"exited to update, then a new boot: an outage up to the boot",
			[]Stretch{closed(0, 9*hr, ReasonUpdate), {Start: 18 * hr, Last: now, State: StateRecording, Boot: 18*hr - mi}},
			[]Gap{unknownEnd(9*hr, 18*hr-mi, GapUpdate), g(18*hr-mi, 18*hr, GapOff)}},
		{"exited for Accessibility, then a new boot: an outage up to the boot",
			[]Stretch{closed(0, 9*hr, ReasonAXStale), {Start: 18 * hr, Last: now, State: StateRecording, Boot: 18*hr - mi}},
			[]Gap{unknownEnd(9*hr, 18*hr-mi, GapPermission), g(18*hr-mi, 18*hr, GapOff)}},
		{"exited on an error, then a new boot: an outage up to the boot",
			[]Stretch{closed(0, 9*hr, ReasonError), {Start: 18 * hr, Last: now, State: StateRecording, Boot: 18*hr - mi}},
			[]Gap{unknownEnd(9*hr, 18*hr-mi, GapError), g(18*hr-mi, 18*hr, GapOff)}},
		{"asleep, then a new boot: off",
			[]Stretch{closed(0, 9*hr, ReasonAsleep), {Start: 18 * hr, Last: now, State: StateRecording, Boot: 18*hr - mi}},
			[]Gap{g(9*hr, 18*hr, GapOff)}},
		{"boot time unknown: the reason alone decides",
			[]Stretch{closed(0, 9*hr, ReasonSignal), {Start: 18 * hr, Last: now, State: StateRecording}},
			[]Gap{g(9*hr, 18*hr, GapStopped)}},
		{"session ended and trackd is not running now: down",
			[]Stretch{closed(0, 20*hr, ReasonSession)},
			[]Gap{g(20*hr, now, GapDown)}},
		{"paused by config.toml",
			[]Stretch{closed(0, 9*hr, ReasonState), {Start: 9 * hr, Last: 10 * hr, Stop: 10 * hr, Reason: ReasonState, State: StatePaused, Boot: 1},
				{Start: 10 * hr, Last: now, State: StateRecording, Boot: 1}},
			[]Gap{g(9*hr, 10*hr, GapConfig)}},
		{"Accessibility off: recording apps only, a gap of its own kind",
			[]Stretch{closed(0, 9*hr, ReasonState), {Start: 9 * hr, Last: 10 * hr, Stop: 10 * hr, Reason: ReasonState, State: StateNoAccess, Boot: 1},
				{Start: 10 * hr, Last: now, State: StateRecording, Boot: 1}},
			[]Gap{g(9*hr, 10*hr, GapNoAccess)}},
		{"accessibility went stale",
			[]Stretch{closed(0, 9*hr, ReasonAXStale), {Start: 9*hr + 2*mi, Last: now, State: StateRecording, Boot: 1}},
			[]Gap{g(9*hr, 9*hr+2*mi, GapPermission)}},
		{"a dead open run at the end: not running until now",
			[]Stretch{{Start: 0, Last: 20 * hr, State: StateRecording, Boot: 1}},
			[]Gap{g(20*hr, now, GapDown)}},
		{"stopped and never started again",
			[]Stretch{closed(0, 20*hr, ReasonSignal)},
			[]Gap{g(20*hr, now, GapStopped)}},
		{"a clock jump back overlaps: no gap",
			[]Stretch{closed(0, 10*hr, ReasonClock), {Start: 9 * hr, Last: now, State: StateRecording, Boot: 1}},
			nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Gaps(tt.runs, 0, now, now)
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("Gaps = %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestGapsClipped(t *testing.T) {
	runs := []Stretch{closed(0, 9*hr, ReasonCrash), {Start: 30 * hr, Last: 40 * hr, State: StateRecording, Boot: 1}}
	got := Gaps(runs, 24*hr, 48*hr, 40*hr)
	want := []Gap{g(24*hr, 30*hr, GapCrash)}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Gaps = %+v, want %+v", got, want)
	}
	// the range stops at now
	got = Gaps([]Stretch{closed(0, 9*hr, ReasonSignal)}, 0, 24*hr, 12*hr)
	if want := []Gap{g(9*hr, 12*hr, GapStopped)}; !reflect.DeepEqual(got, want) {
		t.Fatalf("Gaps = %+v, want %+v", got, want)
	}
}

// Outages keeps the gaps that are tock's to explain, joins a crash loop
// into one, and drops restarts too short to matter.
func TestOutages(t *testing.T) {
	gaps := []Gap{
		g(0, hr, GapAsleep),
		g(2*hr, 2*hr+1000, GapUpdate), // a restart for an update
		g(3*hr, 3*hr+20_000, GapCrash),
		g(3*hr+30_000, 3*hr+50_000, GapCrash),
		g(3*hr+60_000, 3*hr+10*mi, GapCrash),
		g(5*hr, 6*hr, GapOff),
		g(7*hr, 8*hr, GapConfig),
	}
	got := Outages(gaps)
	want := []Gap{g(3*hr, 3*hr+10*mi, GapCrash), g(7*hr, 8*hr, GapConfig)}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Outages = %+v, want %+v", got, want)
	}
}

// Apps-only time is listed but is never joined with time nothing was
// recorded in; an end that is not known stays not known after a join;
// logged-out time is not an outage.
func TestOutagesKeepsKindsApart(t *testing.T) {
	gaps := []Gap{
		g(0, 10*mi, GapNoAccess),
		g(10*mi+20_000, 20*mi, GapCrash), // 20 s after apps-only: not joined
		g(hr, 2*hr, GapLoggedOut),
		unknownEnd(3*hr, 4*hr, GapStopped),
		g(4*hr+30_000, 4*hr+40_000, GapCrash), // a short crash after it: joined
		g(5*hr, 6*hr, GapNoAccess),
		g(6*hr+10_000, 7*hr, GapNoAccess), // apps-only twice: joined
	}
	got := Outages(gaps)
	want := []Gap{
		g(0, 10*mi, GapNoAccess),
		g(10*mi+20_000, 20*mi, GapCrash),
		unknownEnd(3*hr, 4*hr+40_000, GapStopped),
		g(5*hr, 7*hr, GapNoAccess),
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Outages = %+v, want %+v", got, want)
	}
	for _, o := range got {
		if o.Partial() != (o.Kind == GapNoAccess) || o.NotRecording() == o.Partial() {
			t.Errorf("%+v: Partial %v NotRecording %v", o, o.Partial(), o.NotRecording())
		}
	}
}
