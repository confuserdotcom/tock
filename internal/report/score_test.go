package report

import (
	"testing"

	"tock/internal/stats"
)

func TestBuildDayScores(t *testing.T) {
	vs, ls := build(t,
		sv{1, "09:00", "09:40", "term", "tock │ x", "", "Code", "deep", "tock", false},
		sv{2, "09:40", "09:50", "arc", "Video", "www.youtube.com/watch", "Entertainment", "distract", "", false},
		sv{3, "09:50", "10:00", "term", "tock │ x", "", "Code", "deep", "tock", false},
	)
	target := stats.Target{MS: 4 * 3_600_000}
	d := BuildDay(DayInput{Date: day0, NowMS: hm(t, "15:00"), Views: vs, Labels: ls,
		Target: target, Weights: stats.DefaultWeights})
	if d.Stats.FocusMS != 50*60_000 || len(d.Stats.Blocks) != 2 || d.Stats.Switches != 2 {
		t.Fatalf("stats = %+v", d.Stats)
	}
	want := stats.ScoreDay(d.Stats, target, stats.DefaultWeights)
	if d.Score != want {
		t.Fatalf("score = %+v, want %+v", d.Score, want)
	}
}

func TestScoreLine(t *testing.T) {
	target := stats.Target{MS: 4 * 3_600_000}
	tests := []struct {
		name string
		st   stats.Day
		want string
	}{
		{"no activity", stats.Day{}, "score — · no activity"},
		{"several", stats.Day{ActiveMS: 3 * 3_600_000, FocusMS: 130 * 60_000, LongestMS: 48 * 60_000,
			Blocks: make([]stats.Block, 3), Switches: 14},
			"score 0 · focus 2h10 / 4h00 · 3 blocks, longest 48m · 14 switches"},
		{"one of each", stats.Day{ActiveMS: 3_600_000, FocusMS: 6 * 60_000, LongestMS: 6 * 60_000,
			Blocks: make([]stats.Block, 1), Switches: 1},
			"score 0 · focus 6m / 4h00 · 1 block, longest 6m · 1 switch"},
		{"none", stats.Day{ActiveMS: 3_600_000},
			"score 0 · focus 0s / 4h00 · no blocks · 0 switches"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Score.Value is left 0 so the line shows the fields it reads.
			d := Day{Stats: tt.st, Score: stats.Score{Target: target}}
			if got := ScoreLine(d); got != tt.want {
				t.Fatalf("ScoreLine = %q, want %q", got, tt.want)
			}
		})
	}
}
