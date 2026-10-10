package app

import (
	"errors"
	"time"

	"tock/internal/classify"
	"tock/internal/report"
	"tock/internal/stats"
)

// ErrFutureMonth is returned for a month after the current one.
var ErrFutureMonth = errors.New("that month has not started")

// Month reports the calendar month of any against the previous month: the
// whole previous month for a past month, its same days so far for the
// current one.
func (a *App) Month(any time.Time) (report.Month, error) {
	nowT := a.now()
	now := nowT.UnixMilli()
	d := a.midnight(any)
	first := time.Date(d.Year(), d.Month(), 1, 0, 0, 0, 0, d.Location())
	next, prevFirst := first.AddDate(0, 1, 0), first.AddDate(0, -1, 0)
	today := a.midnight(nowT)
	if first.After(today) {
		return report.Month{}, ErrFutureMonth
	}
	prevLen := first.AddDate(0, 0, -1).Day()
	in := report.MonthInput{First: first, Through: next.AddDate(0, 0, -1).Day(), PrevThrough: prevLen}
	if next.After(today) {
		in.Partial = true
		in.Through = today.Day()
		in.PrevThrough = min(in.Through, prevLen)
	}
	end := first.AddDate(0, 0, in.Through)
	prevEnd := prevFirst.AddDate(0, 0, in.PrevThrough)
	vs, ls, err := a.labelled(prevFirst.UnixMilli(), end.UnixMilli(), now)
	if err != nil {
		return report.Month{}, err
	}
	in.Views, in.Labels = clip(vs, ls, first, end)
	in.PrevViews, in.PrevLabels = clip(vs, ls, prevFirst, prevEnd)
	if in.Days, err = a.monthDays(vs, ls, first, in.Through); err != nil {
		return report.Month{}, err
	}
	if in.PrevDays, err = a.monthDays(vs, ls, prevFirst, in.PrevThrough); err != nil {
		return report.Month{}, err
	}
	return report.BuildMonth(in), nil
}

// monthDays scores the n days from first.
func (a *App) monthDays(vs []classify.SpanView, ls []classify.Label, first time.Time, n int) ([]report.MonthDay, error) {
	out := make([]report.MonthDay, n)
	for i := range n {
		day := first.AddDate(0, 0, i)
		dv, dl := clip(vs, ls, day, day.AddDate(0, 0, 1))
		target, err := a.targetFor(day)
		if err != nil {
			return nil, err
		}
		st := stats.Compute(dv, dl)
		out[i] = report.MonthDay{Stats: st, Score: stats.ScoreDay(st, target, a.cfg.Score)}
	}
	return out, nil
}

// clip keeps the views overlapping [from, to), clipped to it.
func clip(vs []classify.SpanView, ls []classify.Label, from, to time.Time) ([]classify.SpanView, []classify.Label) {
	f, t := from.UnixMilli(), to.UnixMilli()
	var ov []classify.SpanView
	var ol []classify.Label
	for i, v := range vs {
		if v.End <= f || v.Start >= t {
			continue
		}
		v.Start, v.End = max(v.Start, f), min(v.End, t)
		ov = append(ov, v)
		ol = append(ol, ls[i])
	}
	return ov, ol
}
