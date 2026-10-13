package report

import (
	"strings"
	"testing"

	"tock/internal/stats"
)

// workHours is 10:00–12:00 on day0's weekday (a Saturday), and Sunday.
func workHours() stats.Hours {
	h := stats.Hours{Set: true, StartMin: 10 * 60, EndMin: 12 * 60}
	h.Days[day0.Weekday()] = true
	h.Days[day0.AddDate(0, 0, 1).Weekday()] = true
	return h
}

func TestBuildDayScoresOnlyWorkHours(t *testing.T) {
	vs, ls := build(t,
		sv{1, "08:00", "09:30", "term", "", "", "Code", "deep", "tock", false},
		sv{2, "10:00", "11:00", "term", "", "", "Code", "deep", "tock", false},
		sv{3, "11:30", "12:30", "arc", "", "", "Fun", "distract", "", false},
	)
	d := BuildDay(DayInput{Date: day0, NowMS: hm(t, "15:00"), Views: vs, Labels: ls, Hours: workHours(),
		Target: stats.Target{MS: 2 * 3_600_000}, Weights: stats.DefaultWeights})
	if d.TrackedMS != 210*60_000 {
		t.Fatalf("tracked %s, want the whole day", Dur(d.TrackedMS))
	}
	if d.Stats.ActiveMS != 90*60_000 || d.Stats.DeepMS != 60*60_000 || d.OutsideMS != 120*60_000 {
		t.Fatalf("active %s deep %s outside %s", Dur(d.Stats.ActiveMS), Dur(d.Stats.DeepMS), Dur(d.OutsideMS))
	}
	if d.Off || !d.Closed || d.From != hm(t, "10:00") || d.To != hm(t, "12:00") {
		t.Fatalf("off %v closed %v window %d..%d", d.Off, d.Closed, d.From, d.To)
	}
	line := ScoreLine(d)
	if !strings.HasSuffix(line, " · 10:00–12:00 · 2h00 outside hours") {
		t.Errorf("score line %q", line)
	}
	open := BuildDay(DayInput{Date: day0, NowMS: hm(t, "11:00"), Views: vs[:2], Labels: ls[:2], Hours: workHours()})
	if open.Closed {
		t.Error("closed before the end of hours")
	}
}

func TestBuildDayOff(t *testing.T) {
	vs, ls := build(t, sv{1, "10:00", "11:20", "term", "", "", "Code", "deep", "tock", false})
	h := workHours()
	h.Days[day0.Weekday()] = false
	d := BuildDay(DayInput{Date: day0, NowMS: hm(t, "15:00"), Views: vs, Labels: ls, Hours: h})
	if !d.Off || d.Stats.ActiveMS != 0 || d.Score.Value != 0 || d.OutsideMS != 80*60_000 {
		t.Fatalf("off %v active %d score %d outside %d", d.Off, d.Stats.ActiveMS, d.Score.Value, d.OutsideMS)
	}
	if got := ScoreLine(d); got != "day off · not scored" {
		t.Errorf("score line %q", got)
	}
	if got := Recap(d); got != "Day off: 1h20 tracked, not scored." {
		t.Errorf("recap %q", got)
	}
	if got := HoursText(d); got != "day off" {
		t.Errorf("hours text %q", got)
	}
}

func TestRecapNoActivityInHours(t *testing.T) {
	vs, ls := build(t, sv{1, "08:00", "09:00", "term", "", "", "Code", "deep", "tock", false})
	d := BuildDay(DayInput{Date: day0, NowMS: hm(t, "15:00"), Views: vs, Labels: ls, Hours: workHours()})
	if got := Recap(d); got != "No activity in work hours." {
		t.Errorf("recap %q", got)
	}
	if got := ScoreLine(d); got != "score — · no activity · 10:00–12:00 · 1h00 outside hours" {
		t.Errorf("score line %q", got)
	}
}

func TestBuildWeekScoresOnlyWorkHours(t *testing.T) {
	mon := MondayOf(day0)
	vs, ls := build(t,
		sv{1, "08:00", "09:00", "term", "", "", "Code", "deep", "tock", false},
		sv{2, "10:00", "11:00", "term", "", "", "Code", "deep", "tock", false},
	)
	w := BuildWeek(WeekInput{Monday: mon, NowMS: hm(t, "15:00"), Views: vs, Labels: ls, Hours: workHours()})
	i := dayIndex(mon, day0)
	if w.Days[i].TrackedMS != 120*60_000 || w.Days[i].Stats.ActiveMS != 60*60_000 {
		t.Fatalf("tracked %s active %s", Dur(w.Days[i].TrackedMS), Dur(w.Days[i].Stats.ActiveMS))
	}
}
