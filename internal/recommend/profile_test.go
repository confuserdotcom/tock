package recommend

import (
	"reflect"
	"testing"
	"time"

	"tock/internal/stats"
)

const min1 = int64(60_000)

var zone = time.FixedZone("test", 2*3600)

// day0 is Saturday 2026-10-10 00:00.
var day0 = time.Date(2026, 10, 10, 0, 0, 0, 0, zone)

// history returns n days ending the day before day0, shaped by fill.
func history(n int, fill func(d *Day)) []Day {
	var out []Day
	for i := n; i >= 1; i-- {
		d := Day{Date: day0.AddDate(0, 0, -i)}
		fill(&d)
		out = append(out, d)
	}
	return out
}

func TestBuildTopHours(t *testing.T) {
	days := history(21, func(d *Day) {
		d.ActiveMS[9], d.FocusMS[9] = 60*min1, 50*min1
		d.ActiveMS[14], d.FocusMS[14] = 60*min1, 20*min1
		if d.Date.Weekday() == time.Saturday {
			d.ActiveMS[10], d.FocusMS[10] = 60*min1, 40*min1
			d.ActiveMS[16], d.FocusMS[16] = 60*min1, 14*min1
		}
	})
	p := Build(Input{Days: days, MinDays: 7, DefaultSessionMS: 25 * min1})
	if p.ActiveDays != 21 || p.Thin || p.MinDays != 7 {
		t.Fatalf("ActiveDays %d Thin %v MinDays %d", p.ActiveDays, p.Thin, p.MinDays)
	}
	if got := p.Hours[time.Saturday][9]; got != (HourStat{3, 50 * min1}) {
		t.Errorf("Hours[Saturday][9] = %+v", got)
	}
	if !reflect.DeepEqual(p.Top[time.Saturday], []int{9, 10, 14}) {
		t.Errorf("Top[Saturday] = %v", p.Top[time.Saturday])
	}
	if !reflect.DeepEqual(p.Top[time.Monday], []int{9, 14}) {
		t.Errorf("Top[Monday] = %v", p.Top[time.Monday])
	}
	if p.Rank(time.Saturday, 10) != 2 || p.Rank(time.Saturday, 16) != 0 || p.Rank(time.Saturday, 3) != 0 {
		t.Errorf("Rank = %d, %d", p.Rank(time.Saturday, 10), p.Rank(time.Saturday, 16))
	}
	if p.SessionMS != 25*min1 || p.GoodSessions != 0 {
		t.Errorf("SessionMS %d GoodSessions %d", p.SessionMS, p.GoodSessions)
	}
}

func TestBuildRules(t *testing.T) {
	// An hour with mean 60m but only 2 samples is not ranked.
	p := Build(Input{Days: history(14, func(d *Day) {
		d.ActiveMS[8], d.FocusMS[8] = 60*min1, 30*min1
		if d.Date.Weekday() == time.Monday {
			d.ActiveMS[11], d.FocusMS[11] = 60*min1, 60*min1 // 2 Mondays in 14 days
		}
	}), MinDays: 7, DefaultSessionMS: 25 * min1})
	if p.Rank(time.Monday, 11) != 0 {
		t.Errorf("hour with one sample ranked: Top[Monday] = %v", p.Top[time.Monday])
	}

	// Four hours over 15 min -> only the best 3; equal means rank the earlier hour first.
	p = Build(Input{Days: history(21, func(d *Day) {
		for _, h := range []int{9, 10, 11, 12} {
			d.ActiveMS[h], d.FocusMS[h] = 60*min1, 30*min1
		}
		d.ActiveMS[13], d.FocusMS[13] = 60*min1, 45*min1
	}), MinDays: 7, DefaultSessionMS: 25 * min1})
	if !reflect.DeepEqual(p.Top[time.Tuesday], []int{13, 9, 10}) {
		t.Errorf("Top[Tuesday] = %v, want [13 9 10]", p.Top[time.Tuesday])
	}

	// 6 active days with MinDays 7 -> thin; Hours still filled.
	p = Build(Input{Days: history(6, func(d *Day) {
		d.ActiveMS[9], d.FocusMS[9] = 60*min1, 50*min1
	}), MinDays: 7, DefaultSessionMS: 25 * min1})
	if !p.Thin || p.ActiveDays != 6 {
		t.Errorf("Thin %v ActiveDays %d, want thin with 6", p.Thin, p.ActiveDays)
	}
	for wd := range p.Top {
		if p.Top[wd] != nil {
			t.Errorf("Top[%d] = %v on thin history", wd, p.Top[wd])
		}
	}
	filled := false
	for wd := range p.Hours {
		if p.Hours[wd][9].Samples > 0 {
			filled = true
		}
	}
	if !filled {
		t.Error("Hours not filled on thin history")
	}

	// A day with no active ms does not count as an active day.
	ds := history(10, func(d *Day) {
		if d.Date.Day()%2 == 0 {
			d.ActiveMS[9], d.FocusMS[9] = 60*min1, 50*min1
		}
	})
	want := 0
	for _, d := range ds {
		if d.ActiveMS[9] > 0 {
			want++
		}
	}
	if p := Build(Input{Days: ds, MinDays: 3, DefaultSessionMS: 25 * min1}); p.ActiveDays != want {
		t.Errorf("ActiveDays = %d, want %d", p.ActiveDays, want)
	}
}

// sess is a finished manual session on day0 that started daysAgo days before
// NowMS, with the planned length in minutes.
func sess(nowMS int64, daysAgo int, plannedMin int64, quality, rating int) Session {
	start := nowMS - int64(daysAgo)*DayMS
	return Session{StartMS: start, EndMS: start + plannedMin*min1, PlannedMS: start + plannedMin*min1, Quality: quality, Rating: rating}
}

func TestBuildSessions(t *testing.T) {
	now := day0.Add(12 * time.Hour).UnixMilli()
	good := func(lens ...int64) []Session {
		var out []Session
		for i, l := range lens {
			out = append(out, sess(now, i+1, l, 80, 0))
		}
		return out
	}
	build := func(ss []Session) Profile {
		return Build(Input{NowMS: now, MinDays: 7, DefaultSessionMS: 25 * min1, Sessions: ss})
	}
	poor := []Session{sess(now, 2, 25, 50, 0), sess(now, 3, 25, 50, 0), sess(now, 4, 25, 50, 0)}
	cases := []struct {
		name      string
		ss        []Session
		want      int64
		goodCount int
	}{
		{"lower median of 6 is the 3rd", append(good(40, 45, 45, 50, 60, 90), poor...), 45, 6},
		{"44 rounds up to 45", good(43, 43, 44, 47, 47), 45, 5},
		{"clamped up to 15", good(10, 10, 10, 10, 10), 15, 5},
		{"clamped down to 120", good(150, 150, 150, 150, 150), 120, 5},
		{"4 good sessions are not enough", good(60, 60, 60, 60), 25, 0},
		{"unsorted input", good(90, 40, 60, 45, 50, 45), 45, 6},
		{"rating 4 with quality 30 counts", []Session{
			sess(now, 1, 30, 30, 4), sess(now, 2, 30, 30, 4), sess(now, 3, 30, 30, 5),
			sess(now, 4, 30, 30, 4), sess(now, 5, 30, 30, 4)}, 30, 5},
		{"quality 70 counts", []Session{
			sess(now, 1, 50, 70, 0), sess(now, 2, 50, 70, 0), sess(now, 3, 50, 70, 0),
			sess(now, 4, 50, 70, 0), sess(now, 5, 50, 70, 0)}, 50, 5},
		{"quality 69 unrated does not", []Session{
			sess(now, 1, 50, 69, 0), sess(now, 2, 50, 69, 0), sess(now, 3, 50, 69, 0),
			sess(now, 4, 50, 69, 0), sess(now, 5, 50, 69, 3)}, 25, 0},
	}
	for _, c := range cases {
		p := build(c.ss)
		if p.SessionMS != c.want*min1 || p.GoodSessions != c.goodCount {
			t.Errorf("%s: SessionMS %d min, GoodSessions %d; want %d, %d", c.name, p.SessionMS/min1, p.GoodSessions, c.want, c.goodCount)
		}
	}

	// A session that started 61 days before NowMS and a running one are left out.
	ss := good(40, 40, 40, 40)
	old := sess(now, 61, 40, 90, 0)
	run := sess(now, 1, 40, 90, 0)
	run.EndMS = 0
	if p := build(append(append([]Session{}, ss...), old)); p.GoodSessions != 0 {
		t.Errorf("61-day-old session counted: GoodSessions %d", p.GoodSessions)
	}
	if p := build(append(append([]Session{}, ss...), run)); p.GoodSessions != 0 {
		t.Errorf("running session counted: GoodSessions %d", p.GoodSessions)
	}
	edge := sess(now, 60, 40, 90, 0)
	if p := build(append(append([]Session{}, ss...), edge)); p.GoodSessions != 5 {
		t.Errorf("60-day-old session left out: GoodSessions %d", p.GoodSessions)
	}
}

func TestBuildSustained(t *testing.T) {
	runs := []int64{30, 40, 52, 52, 55, 60, 61, 70, 80, 0}
	i := 0
	days := history(10, func(d *Day) {
		d.RunMS = runs[i] * min1
		i++
	})
	p := Build(Input{Days: days, MinDays: 7, DefaultSessionMS: 25 * min1})
	if p.SustainedMS != 55*min1 || p.SustainedDays != 9 {
		t.Errorf("SustainedMS %d min, SustainedDays %d; want 55, 9", p.SustainedMS/min1, p.SustainedDays)
	}
	i = 0
	days = history(10, func(d *Day) {
		if i < 6 {
			d.RunMS = 50 * min1
		}
		i++
	})
	p = Build(Input{Days: days, MinDays: 7, DefaultSessionMS: 25 * min1})
	if p.SustainedMS != 0 || p.SustainedDays != 6 {
		t.Errorf("6 days: SustainedMS %d, SustainedDays %d; want 0, 6", p.SustainedMS, p.SustainedDays)
	}
}

// TestBuildRanksWorkHours: the best hours are ranked among the hours of the
// [hours] window, so evening hours that outscore it cannot take their places.
func TestBuildRanksWorkHours(t *testing.T) {
	days := history(21, func(d *Day) {
		for _, h := range []int{21, 22, 23} { // after work, the best of the day
			d.ActiveMS[h], d.FocusMS[h] = 60*min1, 55*min1
		}
		for _, h := range []int{9, 10, 11, 12} {
			d.ActiveMS[h], d.FocusMS[h] = 60*min1, 30*min1
		}
		d.ActiveMS[8], d.FocusMS[8] = 60*min1, 40*min1 // 08:30 start: partly inside
	})
	hours := stats.Hours{Set: true, StartMin: 8*60 + 30, EndMin: 18 * 60}
	for i := range hours.Days {
		hours.Days[i] = i != int(time.Sunday) && i != int(time.Saturday)
	}
	in := Input{Days: days, MinDays: 7, DefaultSessionMS: 25 * min1}

	if p := Build(in); !reflect.DeepEqual(p.Top[time.Tuesday], []int{21, 22, 23}) {
		t.Fatalf("without [hours] Top[Tuesday] = %v, want [21 22 23]", p.Top[time.Tuesday])
	}
	in.Hours = hours
	p := Build(in)
	if !reflect.DeepEqual(p.Top[time.Tuesday], []int{8, 9, 10}) {
		t.Errorf("Top[Tuesday] = %v, want [8 9 10] (08 is inside a window from 08:30)", p.Top[time.Tuesday])
	}
	if p.Top[time.Saturday] != nil || p.Top[time.Sunday] != nil {
		t.Errorf("days off have top hours: %v %v", p.Top[time.Saturday], p.Top[time.Sunday])
	}
	// The grid keeps every hour.
	if p.Hours[time.Tuesday][22].Samples == 0 {
		t.Error("Hours lost the evening")
	}
}
