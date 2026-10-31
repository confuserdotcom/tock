package report

import (
	"testing"

	"tock/internal/stats"
)

func yesterday() Day {
	d := Day{TrackedMS: 7 * 3_600_000}
	d.Stats.ActiveMS = 6*3_600_000 + 12*60_000
	d.Score.Value = 74
	return d
}

func TestStartBodyYesterday(t *testing.T) {
	withGoals := yesterday()
	withGoals.Goals = []GoalRow{{State: GoalMet}, {State: GoalOK}, {State: GoalShort}}
	off := yesterday()
	off.Off = true
	none := Day{}
	cut := Day{TrackedMS: 3_600_000, Work: stats.Hours{Set: true}}
	for _, tt := range []struct {
		name string
		d    Day
		want string
	}{
		{"goals", withGoals, "Yesterday: 6h12 tracked, score 74, goals 2/3 met."},
		{"no goals", yesterday(), "Yesterday: 6h12 tracked, score 74."},
		{"off", off, "Yesterday: day off."},
		{"none", none, "Yesterday: no activity."},
		{"outside hours", cut, "Yesterday: no activity in work hours."},
	} {
		if got := StartBody(tt.d, nil); got != tt.want {
			t.Errorf("%s: StartBody = %q, want %q", tt.name, got, tt.want)
		}
	}
}
