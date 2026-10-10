package report

import (
	"testing"
	"time"

	"tock/internal/focus"
	"tock/internal/stats"
)

func TestCountdown(t *testing.T) {
	for _, tt := range []struct {
		ms   int64
		want string
	}{{0, "0:00"}, {1, "0:01"}, {60_000, "1:00"}, {724_000, "12:04"}, {-5_000, "0:00"}, {25 * 60_000, "25:00"}} {
		if got := Countdown(tt.ms); got != tt.want {
			t.Errorf("Countdown(%d) = %q, want %q", tt.ms, got, tt.want)
		}
	}
}

func TestSessionText(t *testing.T) {
	zone := time.FixedZone("T", 2*3600)
	start := time.Date(2026, 10, 8, 9, 12, 0, 0, zone).UnixMilli() // a Thursday
	run := focus.Session{Kind: focus.KindManual, StartMS: start, PlannedMS: start + 25*60_000,
		Interruptions: 1, DeepMS: 10 * 60_000, LongestMS: 9 * 60_000}
	now := start + 12*60_000 + 56_000 // 12:04 left
	if got, want := DeepPct(run, now), 77; got != want {
		t.Errorf("DeepPct = %d, want %d", got, want)
	}
	if got, want := SessionStatus(run, now), "12:04 left of 25m · deep 77% · 1 interruption · longest 9m"; got != want {
		t.Errorf("SessionStatus = %q, want %q", got, want)
	}
	short := run
	short.LongestMS, short.Interruptions = 30_000, 2
	if got, want := SessionStats(short, now), "deep 77% · 2 interruptions"; got != want {
		t.Errorf("SessionStats = %q, want %q", got, want)
	}

	done := run
	done.EndMS, done.DeepMS, done.Interruptions = start+25*60_000, 21*60_000, 2
	if got, want := SessionLine(done, zone, false), "09:12  manual   25m  deep  84%  2 int"; got != want {
		t.Errorf("SessionLine = %q, want %q", got, want)
	}
	auto := focus.Session{Kind: focus.KindAuto, StartMS: start, EndMS: start + 100*60_000, DeepMS: 97 * 60_000}
	if got, want := SessionLine(auto, zone, true), "Thu 09:12  auto    1h40  deep  97%"; got != want {
		t.Errorf("SessionLine(auto) = %q, want %q", got, want)
	}
}

func TestRecap(t *testing.T) {
	d := Day{
		Stats: stats.Day{ActiveMS: 3 * 3_600_000, FocusMS: 134 * 60_000},
		Score: stats.Score{Value: 74, Target: stats.Target{MS: 4 * 3_600_000}},
		Categories: []CatRow{
			{Name: "Code", Projects: []NameMS{{"tock", 100 * 60_000}, {"ATLAS", 30 * 60_000}}},
			{Name: "Study", Projects: []NameMS{{"ATLAS", 80 * 60_000}, {"", 99 * 60_000}}},
		},
	}
	if got, want := Recap(d), "2h14 focus, score 74 (target 4h00). Top project: ATLAS."; got != want {
		t.Errorf("Recap = %q, want %q", got, want)
	}
	d.Stats.FocusMS, d.Categories = 30_000, nil
	if got, want := Recap(d), "No focus time, score 74 (target 4h00). Top project: none."; got != want {
		t.Errorf("Recap(no focus) = %q, want %q", got, want)
	}
	if got, want := Recap(Day{}), "No activity."; got != want {
		t.Errorf("Recap(empty) = %q, want %q", got, want)
	}
}
