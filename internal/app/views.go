package app

import (
	"math"
	"time"

	"tock/internal/classify"
	"tock/internal/report"
	"tock/internal/store"
)

// views converts rows to classify views clipped to [from, to); the open span
// ends at nowMS. Spans with nothing left after clipping are dropped.
func views(rows []store.SpanRow, from, to, nowMS int64) []classify.SpanView {
	out := make([]classify.SpanView, 0, len(rows))
	for _, r := range rows {
		end := nowMS
		if r.End != nil {
			end = *r.End
		}
		start := max(r.Start, from)
		end = min(end, to)
		if end <= start {
			continue
		}
		out = append(out, classify.SpanView{
			ID: r.ID, Start: start, End: end,
			BundleID: r.BundleID, App: r.App,
			Title: r.Title, URL: r.URL, Private: r.Private,
		})
	}
	return out
}

func overrides(m map[int64]store.Override) map[int64]classify.Override {
	out := make(map[int64]classify.Override, len(m))
	for id, o := range m {
		out[id] = classify.Override{Category: o.Category, Project: o.Project}
	}
	return out
}

// labelled loads the spans overlapping [from, to), clipped, with their
// labels. The open span ends at nowMS; callers pass the same nowMS to the
// report so it can tell which row is open.
func (a *App) labelled(from, to, nowMS int64) ([]classify.SpanView, []classify.Label, error) {
	rows, err := a.st.Spans(from, to)
	if err != nil {
		return nil, nil, err
	}
	ovs, err := a.st.Overrides()
	if err != nil {
		return nil, nil, err
	}
	vs := views(rows, from, to, nowMS)
	return vs, a.rs.LabelAll(vs, overrides(ovs)), nil
}

// history is every span, labelled, with the overrides.
func (a *App) history() ([]classify.SpanView, []classify.Label, map[int64]classify.Override, error) {
	rows, err := a.st.AllSpans()
	if err != nil {
		return nil, nil, nil, err
	}
	ovs, err := a.st.Overrides()
	if err != nil {
		return nil, nil, nil, err
	}
	o := overrides(ovs)
	vs := views(rows, math.MinInt64, math.MaxInt64, a.nowMS())
	return vs, a.rs.LabelAll(vs, o), o, nil
}

// midnight returns local midnight of t's calendar day in the clock's zone.
func (a *App) midnight(t time.Time) time.Time {
	t = t.In(a.now().Location())
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, t.Location())
}

// Day reports the calendar day of day.
func (a *App) Day(day time.Time) (report.Day, error) {
	from := a.midnight(day)
	to := from.AddDate(0, 0, 1)
	now := a.nowMS()
	vs, labels, err := a.labelled(from.UnixMilli(), to.UnixMilli(), now)
	if err != nil {
		return report.Day{}, err
	}
	ignored, err := a.st.Ignored()
	if err != nil {
		return report.Day{}, err
	}
	return report.BuildDay(report.DayInput{Date: from, NowMS: now, Views: vs, Labels: labels, Ignored: ignored}), nil
}

// Week reports the Monday-start week containing anyDay.
func (a *App) Week(anyDay time.Time) (report.Week, error) {
	mon := report.MondayOf(a.midnight(anyDay))
	now := a.nowMS()
	vs, labels, err := a.labelled(mon.UnixMilli(), mon.AddDate(0, 0, 7).UnixMilli(), now)
	if err != nil {
		return report.Week{}, err
	}
	return report.BuildWeek(report.WeekInput{Monday: mon, NowMS: now, Views: vs, Labels: labels}), nil
}

// Inbox returns the unlabelled keys of the last days days, largest first.
func (a *App) Inbox(days int) ([]classify.Item, error) {
	now := a.nowMS()
	vs, labels, err := a.labelled(now-int64(days)*24*60*60*1000, now, now)
	if err != nil {
		return nil, err
	}
	ignored, err := a.st.Ignored()
	if err != nil {
		return nil, err
	}
	return classify.Inbox(vs, labels, ignored), nil
}

// KeySpans returns the ids and total time of every span with key.
func (a *App) KeySpans(key string) ([]int64, int64, error) {
	hist, _, _, err := a.history()
	if err != nil {
		return nil, 0, err
	}
	var ids []int64
	var ms int64
	for _, v := range hist {
		if classify.Key(v) == key {
			ids = append(ids, v.ID)
			ms += v.DurMS()
		}
	}
	return ids, ms, nil
}
