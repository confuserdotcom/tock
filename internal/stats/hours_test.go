package stats

import (
	"testing"
	"time"

	"tock/internal/classify"
)

func weekdays() Hours {
	h := Hours{Set: true, StartMin: 9 * 60, EndMin: 18 * 60}
	for d := time.Monday; d <= time.Friday; d++ {
		h.Days[d] = true
	}
	return h
}

func TestWindow(t *testing.T) {
	loc, err := time.LoadLocation("Europe/Paris")
	if err != nil {
		t.Skip(err)
	}
	mon := time.Date(2026, 10, 12, 0, 0, 0, 0, loc)
	sun := time.Date(2026, 10, 18, 0, 0, 0, 0, loc)
	dst := time.Date(2026, 3, 30, 0, 0, 0, 0, loc) // Monday after the spring change
	day := func(t time.Time, h, m int) int64 {
		return time.Date(t.Year(), t.Month(), t.Day(), h, m, 0, 0, loc).UnixMilli()
	}
	for _, c := range []struct {
		name     string
		h        Hours
		day      time.Time
		from, to int64
		off      bool
	}{
		{"off", Hours{}, mon, day(mon, 0, 0), mon.AddDate(0, 0, 1).UnixMilli(), false},
		{"work day", weekdays(), mon, day(mon, 9, 0), day(mon, 18, 0), false},
		{"day off", weekdays(), sun, day(sun, 0, 0), day(sun, 0, 0), true},
		{"after dst", weekdays(), dst, day(dst, 9, 0), day(dst, 18, 0), false},
	} {
		from, to := c.h.Window(c.day)
		if from != c.from || to != c.to || c.h.Off(c.day) != c.off {
			t.Errorf("%s: window %d..%d off %v, want %d..%d off %v", c.name, from, to, c.h.Off(c.day), c.from, c.to, c.off)
		}
	}
	// The spring change day is 23 hours long; an unset window still spans it.
	spring := time.Date(2026, 3, 29, 0, 0, 0, 0, loc)
	if from, to := (Hours{}).Window(spring); to-from != 23*3_600_000 {
		t.Errorf("spring day window %dh, want 23h", (to-from)/3_600_000)
	}
}

func TestClip(t *testing.T) {
	vs, ls := at(deep(10*mn), shallow(10*mn), distract(10*mn))
	from, to := vs[0].Start+5*mn, vs[2].Start+5*mn
	cv, cl := Clip(vs, ls, from, to)
	if len(cv) != 3 || cv[0].Start != from || cv[2].End != to || cl[1].Kind != classify.KindShallow {
		t.Fatalf("clip = %+v %+v", cv, cl)
	}
	if cv, _ := Clip(vs, ls, from, from); len(cv) != 0 {
		t.Errorf("empty window kept %d views", len(cv))
	}
}
