package stats

import (
	"reflect"
	"testing"
)

const (
	mi = int64(60_000)
	hr = 60 * mi
)

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
			[]Gap{{10 * hr, 10*hr + 5*mi, GapCrash}}},
		{"a run left open and a later one: the open one crashed at its last tick",
			[]Stretch{{Start: 0, Last: 10 * hr, State: StateRecording, Boot: 1}, {Start: 11 * hr, Last: now, State: StateRecording, Boot: 1}},
			[]Gap{{10 * hr, 11 * hr, GapCrash}}},
		{"stopped on purpose",
			[]Stretch{closed(0, 9*hr, ReasonSignal), {Start: 10 * hr, Last: now, State: StateRecording, Boot: 1}},
			[]Gap{{9 * hr, 10 * hr, GapStopped}}},
		{"asleep is a gap but not an outage",
			[]Stretch{closed(0, 9*hr, ReasonAsleep), {Start: 10 * hr, Last: now, State: StateRecording, Boot: 1}},
			[]Gap{{9 * hr, 10 * hr, GapAsleep}}},
		{"a new boot between runs: the computer was off",
			[]Stretch{closed(0, 9*hr, ReasonSignal), {Start: 10 * hr, Last: now, State: StateRecording, Boot: 10*hr - mi}},
			[]Gap{{9 * hr, 10 * hr, GapOff}}},
		{"a crash then a new boot: off, not a trackd crash",
			[]Stretch{closed(0, 9*hr, ReasonCrash), {Start: 10 * hr, Last: now, State: StateRecording, Boot: 10*hr - mi}},
			[]Gap{{9 * hr, 10 * hr, GapOff}}},
		{"paused by config.toml",
			[]Stretch{closed(0, 9*hr, ReasonState), {Start: 9 * hr, Last: 10 * hr, Stop: 10 * hr, Reason: ReasonState, State: StatePaused, Boot: 1},
				{Start: 10 * hr, Last: now, State: StateRecording, Boot: 1}},
			[]Gap{{9 * hr, 10 * hr, GapConfig}}},
		{"accessibility went stale",
			[]Stretch{closed(0, 9*hr, ReasonAXStale), {Start: 9*hr + 2*mi, Last: now, State: StateRecording, Boot: 1}},
			[]Gap{{9 * hr, 9*hr + 2*mi, GapPermission}}},
		{"a dead open run at the end: not running until now",
			[]Stretch{{Start: 0, Last: 20 * hr, State: StateRecording, Boot: 1}},
			[]Gap{{20 * hr, now, GapDown}}},
		{"stopped and never started again",
			[]Stretch{closed(0, 20*hr, ReasonSignal)},
			[]Gap{{20 * hr, now, GapStopped}}},
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
	want := []Gap{{24 * hr, 30 * hr, GapCrash}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Gaps = %+v, want %+v", got, want)
	}
	// the range stops at now
	got = Gaps([]Stretch{closed(0, 9*hr, ReasonSignal)}, 0, 24*hr, 12*hr)
	if want := []Gap{{9 * hr, 12 * hr, GapStopped}}; !reflect.DeepEqual(got, want) {
		t.Fatalf("Gaps = %+v, want %+v", got, want)
	}
}

// Outages keeps the gaps that are tock's to explain, joins a crash loop
// into one, and drops restarts too short to matter.
func TestOutages(t *testing.T) {
	gaps := []Gap{
		{0, hr, GapAsleep},
		{2 * hr, 2*hr + 1000, GapUpdate}, // a restart for an update
		{3 * hr, 3*hr + 20_000, GapCrash},
		{3*hr + 30_000, 3*hr + 50_000, GapCrash},
		{3*hr + 60_000, 3*hr + 10*mi, GapCrash},
		{5 * hr, 6 * hr, GapOff},
		{7 * hr, 8 * hr, GapConfig},
	}
	got := Outages(gaps)
	want := []Gap{{3 * hr, 3*hr + 10*mi, GapCrash}, {7 * hr, 8 * hr, GapConfig}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Outages = %+v, want %+v", got, want)
	}
}
