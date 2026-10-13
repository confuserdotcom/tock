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
		wFrom, wTo := a.cfg.Hours.Window(day)
		dv, dl := stats.Clip(vs, ls, wFrom, wTo)
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
	return stats.Clip(vs, ls, from.UnixMilli(), to.UnixMilli())
}

// Top ranks the tracked time of the days [from, to) by category, project,
// app and site.
func (a *App) Top(from, to time.Time) (report.Top, error) {
	from, to = a.midnight(from), a.midnight(to)
	now := a.nowMS()
	vs, labels, err := a.labelled(from.UnixMilli(), to.UnixMilli(), now)
	if err != nil {
		return report.Top{}, err
	}
	return report.BuildTop(from, to, vs, labels), nil
}
