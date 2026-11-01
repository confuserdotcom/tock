package report

import (
	"reflect"
	"strings"
	"testing"
	"time"

	"tock/internal/recommend"
)

const profMin = int64(60_000)

func TestProfileLines(t *testing.T) {
	var p recommend.Profile
	p.MinDays, p.ActiveDays = 7, 21
	p.Top[time.Monday] = []int{14, 9}
	p.Top[time.Saturday] = []int{9, 10, 14}
	p.SessionMS, p.GoodSessions, p.SustainedMS = 45*profMin, 12, 52*profMin
	want := []string{"", "Best focus hours",
		"Mon  09–10 · 14–15", "Tue  –", "Wed  –", "Thu  –", "Fri  –",
		"Sat  09–10 · 10–11 · 14–15", "Sun  –",
		"Typical session: 45 min (from 12 good sessions)",
		"Sustained focus: 52 min before a break"}
	if got := ProfileLines(p); !reflect.DeepEqual(got, want) {
		t.Errorf("ProfileLines =\n%q\nwant\n%q", got, want)
	}

	p.GoodSessions, p.SessionMS, p.SustainedMS = 0, 25*profMin, 0
	got := ProfileLines(p)
	if got[len(got)-2] != "Typical session: 25 min (default, not enough sessions yet)" ||
		got[len(got)-1] != "Sustained focus: (not enough history)" {
		t.Errorf("default lines = %q", got[len(got)-2:])
	}

	var thin recommend.Profile
	thin.MinDays, thin.ActiveDays, thin.Thin = 7, 4, true
	thin.SessionMS = 25 * profMin
	got = ProfileLines(thin)
	if got[1] != "Best focus hours (not enough history: 4 of 7 days)" {
		t.Errorf("thin title = %q", got[1])
	}
	for i := 2; i < 9; i++ {
		if !strings.HasSuffix(got[i], "  –") {
			t.Errorf("thin line %d = %q, want –", i, got[i])
		}
	}

	var late recommend.Profile
	late.Top[time.Sunday] = []int{23}
	if got := ProfileLines(late); got[8] != "Sun  23–00" {
		t.Errorf("Sunday = %q, want Sun  23–00", got[8])
	}
}

func hourStats(p *recommend.Profile, wd time.Weekday, h int, n int, ms int64) {
	p.Hours[wd][h] = recommend.HourStat{Samples: n, MeanMS: ms}
}

// gridRow builds the expected row: label, then each hour from..to as a 4-cell
// right-aligned cell, "·" where cells has no entry.
func gridRow(label string, from, to int, cells map[int]string) string {
	var b strings.Builder
	b.WriteString(label)
	for h := from; h <= to; h++ {
		c, ok := cells[h]
		if !ok {
			c = "·"
		}
		b.WriteString(strings.Repeat(" ", 4-len([]rune(c))) + c)
	}
	return strings.TrimRight(b.String(), " ")
}

func gridHeader(from, to int) string {
	cells := map[int]string{}
	for h := from; h <= to; h++ {
		cells[h] = twoDigits(h)
	}
	return gridRow("   ", from, to, cells)
}

func twoDigits(h int) string { return string([]byte{byte('0' + h/10), byte('0' + h%10)}) }

func TestHourGrid(t *testing.T) {
	var p recommend.Profile
	hourStats(&p, time.Saturday, 9, 3, 60*profMin)
	hourStats(&p, time.Saturday, 14, 3, 20*profMin)
	hourStats(&p, time.Monday, 9, 3, 50*profMin)
	want := []string{"Mean focus minutes per hour, last 28 days", gridHeader(7, 22),
		gridRow("Mon", 7, 22, map[int]string{9: "50"}),
		gridRow("Tue", 7, 22, nil), gridRow("Wed", 7, 22, nil), gridRow("Thu", 7, 22, nil), gridRow("Fri", 7, 22, nil),
		gridRow("Sat", 7, 22, map[int]string{9: "60", 14: "20"}),
		gridRow("Sun", 7, 22, nil)}
	if got := HourGrid(p); !reflect.DeepEqual(got, want) {
		t.Errorf("HourGrid =\n%s\nwant\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
	if !strings.HasPrefix(want[2], "Mon   ·   ·  50   ·") {
		t.Errorf("test helper drifted: %q", want[2])
	}

	hourStats(&p, time.Tuesday, 5, 3, 10*profMin)
	hourStats(&p, time.Tuesday, 23, 4, 29_000)
	got := HourGrid(p)
	if got[1] != gridHeader(5, 23) {
		t.Errorf("widened header = %q, want %q", got[1], gridHeader(5, 23))
	}
	if got[3] != gridRow("Tue", 5, 23, map[int]string{5: "10", 23: "0"}) {
		t.Errorf("Tue = %q", got[3])
	}
}

func TestReplyLines(t *testing.T) {
	now := day0.UnixMilli() + 12*3600_000
	rows := func(kind string, hour int, replies ...string) []recommend.Rec {
		var out []recommend.Rec
		for i, r := range replies {
			// One row a day: break replies of one day and hour are one answer.
			out = append(out, recommend.Rec{Kind: kind, Hour: hour, Reply: r, AtMS: now - int64(i+1)*recommend.DayMS})
		}
		return out
	}
	dis, acc := recommend.ReplyDismiss, recommend.ReplyAccept
	cat := func(parts ...[]recommend.Rec) []recommend.Rec {
		var out []recommend.Rec
		for _, p := range parts {
			out = append(out, p...)
		}
		return out
	}
	check := func(name string, recs []recommend.Rec, want ...string) {
		t.Helper()
		got := ReplyLines(recommend.Learn(recs, now))
		if strings.Join(got, "\n") != strings.Join(want, "\n") {
			t.Errorf("%s:\n%s\nwant\n%s", name, strings.Join(got, "\n"), strings.Join(want, "\n"))
		}
	}
	check("none", nil, "Suggestions: none answered yet", "Breaks: none answered yet")
	check("rates", cat(rows("focus", 16, dis, dis, dis), rows("focus", 9, acc, acc, acc, acc, acc), rows("break", 11, acc, acc, acc, dis, dis, dis)),
		"Suggestions: 5 of 8 accepted · muted at 16–17", "Breaks: 3 of 6 taken")
	check("two hours", cat(rows("focus", 16, dis, dis, dis), rows("focus", 9, dis, dis, dis), rows("focus", 12, acc)),
		"Suggestions: 1 of 7 accepted · muted at 09–10, 16–17", "Breaks: none answered yet")
	check("break muted", rows("break", 15, dis, dis, dis), "Suggestions: none answered yet", "Breaks: 0 of 3 taken · muted at 15–16")
	check("wrap", rows("focus", 23, dis, dis, dis), "Suggestions: 0 of 3 accepted · muted at 23–00", "Breaks: none answered yet")
}
