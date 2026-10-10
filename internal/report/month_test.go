package report

import (
	"reflect"
	"testing"

	"tock/internal/stats"
)

func TestChange(t *testing.T) {
	for _, c := range []struct {
		cur, prev int64
		want      string
	}{
		{110 * 60_000, 100 * 60_000, "  +10m  +10%"},
		{90 * 60_000, 100 * 60_000, "  −10m  −10%"},
		{100 * 60_000, 100 * 60_000, "    ±0  ±0%"},
		{60_000, 59_999, "new"},
		{0, 2 * 3_600_000, " −2h00  gone"},
		{0, 0, ""},
	} {
		if got := change(c.cur, c.prev); got != c.want {
			t.Errorf("change(%d, %d) = %q, want %q", c.cur, c.prev, got, c.want)
		}
	}
}

func TestBuildMonth(t *testing.T) {
	cur, curL := build(t,
		sv{id: 1, start: "09:00", end: "11:00", app: "term", cat: "Code", kind: "deep"},
		sv{id: 2, start: "11:00", end: "11:30", kind: "idle"},
		sv{id: 3, start: "11:30", end: "12:00", app: "x", kind: ""},
	)
	prev, prevL := build(t,
		sv{id: 4, start: "09:00", end: "10:00", app: "term", cat: "Code", kind: "deep"},
		sv{id: 5, start: "10:00", end: "11:00", app: "pdf", cat: "Study", kind: "deep"},
		sv{id: 6, start: "11:00", end: "12:00", app: "yt", cat: "Entertainment", kind: "distract"},
	)
	day := func(active int64, score, switches int) MonthDay {
		return MonthDay{Stats: stats.Day{ActiveMS: active, FocusMS: active / 2, Switches: switches}, Score: stats.Score{Value: score}}
	}
	m := BuildMonth(MonthInput{
		First: day0, Through: 3, PrevThrough: 2, Partial: true,
		Views: cur, Labels: curL, PrevViews: prev, PrevLabels: prevL,
		Days:     []MonthDay{day(2*3_600_000, 80, 4), day(30*60_000, 99, 9), day(2*3_600_000, 60, 2)},
		PrevDays: []MonthDay{day(3*3_600_000, 50, 6), {}},
	})
	if !reflect.DeepEqual(m.Scores, []int{80, -1, 60}) {
		t.Fatalf("scores = %v", m.Scores)
	}
	if m.Cur.Workdays != 2 || m.Cur.AvgScore != 70 || m.Cur.SwitchesPerHour != 1.5 || m.Cur.TrackedMS != 150*60_000 ||
		m.Cur.FocusMS != (2*3_600_000+30*60_000+2*3_600_000)/2 {
		t.Fatalf("cur = %+v", m.Cur)
	}
	if m.Prev.Workdays != 1 || m.Prev.AvgScore != 50 || m.Prev.SwitchesPerHour != 2 {
		t.Fatalf("prev = %+v", m.Prev)
	}
	var names []string
	for _, c := range m.Categories {
		names = append(names, c.Name)
	}
	// Code has time; Entertainment and Study only last month (ties by name); Unclassified last.
	if !reflect.DeepEqual(names, []string{"Code", "Entertainment", "Study", "Unclassified"}) {
		t.Fatalf("order = %v", names)
	}
	if got := Spark([]int{0, -1, 100, 50}); got != "▁·█▅" {
		t.Fatalf("spark = %q", got)
	}
	m.Through, m.PrevThrough = 1, 1
	if got := MonthTitle(m); got != "October 2026 · 1 vs September 1" {
		t.Fatalf("title = %q", got)
	}
}
