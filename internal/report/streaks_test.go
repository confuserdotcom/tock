package report

import (
	"testing"

	"tock/internal/classify"
)

const hr = int64(3_600_000)

// days builds past days before day0, most recent first, from ms values:
// -1 is an inactive day, -2 a day off with 3h of time.
func days(ms ...int64) []GoalDay {
	out := make([]GoalDay, len(ms))
	for i, m := range ms {
		d := GoalDay{Date: day0.AddDate(0, 0, -(i + 1)), Active: true, MS: []int64{m}}
		switch m {
		case -1:
			d.Active, d.MS = false, []int64{0}
		case -2:
			d.Off, d.MS = true, []int64{3 * hr}
		}
		out[i] = d
	}
	return out
}

func today(ms int64) GoalDay { return GoalDay{Date: day0, Active: true, MS: []int64{ms}} }

func TestStreakDayGoals(t *testing.T) {
	study := classify.Goal{Category: "Study", MS: 2 * hr}
	fun := classify.Goal{Category: "Fun", MS: hr, AtMost: true}
	for _, c := range []struct {
		name      string
		g         classify.Goal
		past      []GoalDay
		today     GoalDay
		closed    bool
		cur, best int
	}{
		{"met run", study, days(2*hr, 3*hr, hr, 2*hr), today(0), false, 2, 2},
		{"today met counts", study, days(2*hr, 3*hr), today(2 * hr), false, 3, 3},
		{"today short is neutral", study, days(2*hr, 3*hr), today(hr), false, 2, 2},
		{"today short and closed breaks", study, days(2*hr, 3*hr), today(hr), true, 0, 2},
		{"inactive and off days skip", study, days(2*hr, -1, -2, 3*hr), today(0), false, 2, 2},
		{"best older run", study, days(2*hr, 0, 2*hr, 2*hr, 2*hr), today(0), false, 1, 3},
		{"limit held past", fun, days(30*60_000, 0, 2*hr), today(0), false, 2, 2},
		{"limit not closed is neutral", fun, days(0, 0), today(0), false, 2, 2},
		{"limit closed counts", fun, days(0, 0), today(0), true, 3, 3},
		{"limit over today breaks", fun, days(0, 0), today(2 * hr), false, 0, 2},
		{"nothing", study, nil, GoalDay{Date: day0, MS: []int64{0}}, false, 0, 0},
	} {
		cur, best := Streak(0, c.g, c.past, c.today, c.closed)
		if cur != c.cur || best != c.best {
			t.Errorf("%s: streak %d best %d, want %d %d", c.name, cur, best, c.cur, c.best)
		}
	}
}

func TestStreakWeekGoal(t *testing.T) {
	g := classify.Goal{Category: "Study", MS: 5 * hr, Week: true}
	// day0 is a Saturday: this week holds the 6 days before it.
	mon := MondayOf(day0)
	var past []GoalDay
	for d := day0.AddDate(0, 0, -1); !d.Before(mon.AddDate(0, 0, -21)); d = d.AddDate(0, 0, -1) {
		ms := 2 * hr // weeks of 14h: held
		if d.Before(mon.AddDate(0, 0, -14)) {
			ms = 0 // three weeks back: short
		}
		past = append(past, GoalDay{Date: d, Active: true, MS: []int64{ms}})
	}
	cur, best := Streak(0, g, past, today(0), false)
	if cur != 3 || best != 3 { // two past weeks plus this one, met already
		t.Errorf("streak %d best %d, want 3 3", cur, best)
	}
	// This week not met yet is left out.
	short := make([]GoalDay, len(past))
	copy(short, past)
	for i := range short[:5] {
		short[i].MS = []int64{0}
	}
	if cur, _ := Streak(0, g, short, today(0), false); cur != 2 {
		t.Errorf("streak %d with this week open, want 2", cur)
	}
}

func TestStreakTexts(t *testing.T) {
	r := GoalRow{Goal: classify.Goal{Category: "Study", MS: 2 * hr}, DoneMS: 2 * hr, State: GoalMet, Streak: 5}
	if got := GoalText(r); got != "Study 2h00/2h00 · 5d" {
		t.Errorf("goal text %q", got)
	}
	r.Streak = 1
	if got := GoalText(r); got != "Study 2h00/2h00" {
		t.Errorf("goal text %q", got)
	}
	w := WeekGoalRow{Goal: classify.Goal{Category: "Study", MS: 10 * hr, Week: true}, DoneMS: 12 * hr, State: GoalMet, Streak: 3, Best: 4}
	if got := WeekGoalText(w); got != "12h00/10h00 met, streak 3w (best 4w)" {
		t.Errorf("week goal text %q", got)
	}
}
