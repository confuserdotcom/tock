package report

import (
	"strings"
	"testing"

	"tock/internal/plan"
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

func TestStartBodyToday(t *testing.T) {
	withGoals := yesterday()
	withGoals.Goals = []GoalRow{{State: GoalMet}, {State: GoalOK}, {State: GoalShort}}
	rows := plan.Rows([]string{
		"Standup due:2026-10-10 @10h00",
		"Review due:2026-10-10 @14:00",
		"Untimed due:2026-10-10",
		"Gym due:2026-10-11 @09h00",
		"x 2026-10-10 Done one due:2026-10-10 @08h00",
		"Late due:2026-10-09 @11h00",
	}, "2026-10-10", "2026-10-11")
	want := "Yesterday: 6h12 tracked, score 74, goals 2/3 met. Today: 10:00 Standup · 14:00 Review."
	if got := StartBody(withGoals, rows); got != want {
		t.Errorf("StartBody = %q, want %q", got, want)
	}

	five := plan.Rows([]string{
		"A due:2026-10-10 @09h00", "B due:2026-10-10 @10h00", "C due:2026-10-10 @11h00",
		"D due:2026-10-10 @12h00", "E due:2026-10-10 @13h00",
	}, "2026-10-10", "2026-10-11")
	want = "Yesterday: 6h12 tracked, score 74. Today: 09:00 A · 10:00 B · 11:00 C · +2 more."
	if got := StartBody(yesterday(), five); got != want {
		t.Errorf("five items: %q, want %q", got, want)
	}

	long := plan.Rows([]string{"Write the quarterly report for the board due:2026-10-10 @09h00"}, "2026-10-10", "2026-10-11")
	got := StartBody(yesterday(), long)
	item := strings.TrimSuffix(strings.TrimPrefix(got, "Yesterday: 6h12 tracked, score 74. Today: 09:00 "), ".")
	if item != "Write the quarterly rep…" {
		t.Errorf("long item = %q", item)
	}

	untimed := plan.Rows([]string{"Untimed due:2026-10-10", "Gym due:2026-10-11 @09h00"}, "2026-10-10", "2026-10-11")
	for _, rs := range [][]plan.Row{nil, untimed} {
		if got := StartBody(yesterday(), rs); got != "Yesterday: 6h12 tracked, score 74." {
			t.Errorf("no timed item today: %q", got)
		}
	}
}

func TestMiddayBody(t *testing.T) {
	d := Day{Categories: []CatRow{{Name: "Coding", MS: 2 * 3_600_000}, {Name: "Mail", MS: 3_600_000}}}
	d.Stats.ActiveMS = 3*3_600_000 + 5*60_000
	d.Stats.FocusMS = 3_600_000 + 40*60_000
	d.Score.Target.MS = 4*3_600_000 + 50*60_000
	noTarget := d
	noTarget.Score.Target.MS = 0
	noCat := d
	noCat.Categories = nil
	onlyUnclassified := d
	onlyUnclassified.Categories = []CatRow{{Name: Unclassified, MS: 3_600_000}}
	skipUnclassified := d
	skipUnclassified.Categories = []CatRow{{Name: Unclassified, MS: 3_600_000}, {Name: "Mail", MS: 600_000}}
	long := d
	long.Categories = []CatRow{{Name: strings.Repeat("x", 30), MS: 3_600_000}}
	for _, tt := range []struct {
		name string
		d    Day
		want string
	}{
		{"full", d, "3h05 tracked, 64% of 4h50 target · top: Coding · focus 1h40."},
		{"no target", noTarget, "3h05 tracked · top: Coding · focus 1h40."},
		{"no categories", noCat, "3h05 tracked, 64% of 4h50 target · focus 1h40."},
		{"only unclassified", onlyUnclassified, "3h05 tracked, 64% of 4h50 target · focus 1h40."},
		{"skips unclassified", skipUnclassified, "3h05 tracked, 64% of 4h50 target · top: Mail · focus 1h40."},
		{"cut", long, "3h05 tracked, 64% of 4h50 target · top: " + Cut(strings.Repeat("x", 30), RoutineItemCells) + " · focus 1h40."},
	} {
		if got := MiddayBody(tt.d); got != tt.want {
			t.Errorf("%s: got %q, want %q", tt.name, got, tt.want)
		}
	}
	if got := Cut(strings.Repeat("x", 30), RoutineItemCells); len([]rune(got)) > RoutineItemCells {
		t.Errorf("cut name %q too long", got)
	}
}
