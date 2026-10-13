package stats

import (
	"time"

	"tock/internal/classify"
)

// Hours is the work schedule the score is limited to. The zero value is
// off: the whole day is scored.
type Hours struct {
	Set              bool
	Days             [7]bool // indexed by time.Weekday
	StartMin, EndMin int     // minutes after local midnight, StartMin < EndMin
}

// Off reports whether day is a day off: the schedule is set and day's
// weekday is not in it.
func (h Hours) Off(day time.Time) bool { return h.Set && !h.Days[day.Weekday()] }

// Window is the scored part of the calendar day of day: the whole day when
// the schedule is off, [start, end) on a work day, and empty (from == to)
// on a day off.
func (h Hours) Window(day time.Time) (from, to int64) {
	y, m, d := day.Date()
	loc := day.Location()
	at := func(min int) int64 { return time.Date(y, m, d, 0, min, 0, 0, loc).UnixMilli() }
	switch {
	case !h.Set:
		return at(0), time.Date(y, m, d+1, 0, 0, 0, 0, loc).UnixMilli()
	case h.Off(day):
		return at(0), at(0)
	}
	return at(h.StartMin), at(h.EndMin)
}

// Clip keeps the views overlapping [from, to), clipped to it, with their
// labels.
func Clip(vs []classify.SpanView, ls []classify.Label, from, to int64) ([]classify.SpanView, []classify.Label) {
	var ov []classify.SpanView
	var ol []classify.Label
	if from >= to {
		return ov, ol
	}
	for i, v := range vs {
		if v.End <= from || v.Start >= to {
			continue
		}
		v.Start, v.End = max(v.Start, from), min(v.End, to)
		ov = append(ov, v)
		ol = append(ol, ls[i])
	}
	return ov, ol
}
