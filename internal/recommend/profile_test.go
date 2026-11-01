package recommend

import (
	"reflect"
	"testing"
	"time"
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
