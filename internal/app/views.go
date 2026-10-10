package app

import (
	"math"
	"time"

	"tock/internal/classify"
	"tock/internal/report"
	"tock/internal/shell"
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

// spans loads the spans overlapping [from, to) and the overrides. A change
// in the overrides or the away rows drops the cached stats of past days.
func (a *App) spans(from, to int64) ([]store.SpanRow, map[int64]classify.Override, error) {
	rows, err := a.st.Spans(from, to)
	if err != nil {
		return nil, nil, err
	}
	ovs, err := a.st.Overrides()
	if err != nil {
		return nil, nil, err
	}
	n, lastAt, err := a.st.AwayStamp()
	if err != nil {
		return nil, nil, err
	}
	if fp := fingerprint(ovs) ^ uint64(n)<<48 ^ uint64(lastAt); fp != a.pastKey {
		a.past, a.pastKey = nil, fp
	}
	return rows, overrides(ovs), nil
}

// labelled loads the spans overlapping [from, to), clipped, with their
// labels. The open span ends at nowMS; callers pass the same nowMS to the
// report so it can tell which row is open.
func (a *App) labelled(from, to, nowMS int64) ([]classify.SpanView, []classify.Label, error) {
	rows, ovs, err := a.spans(from, to)
	if err != nil {
		return nil, nil, err
	}
	vs, err := a.decorate(views(rows, from, to, nowMS), ovs, from, to)
	if err != nil {
		return nil, nil, err
	}
	return vs, a.rs.LabelAll(vs, ovs), nil
}

// CallMinMS is the shortest call that labels time, which keeps dictation and
// Siri out.
const CallMinMS = 2 * 60_000

// decorate adds what is not in the span table to views clipped to
// [from, to): the shell's working directory on terminal views, the calls
// (MarkCalls) when the rules name a call category, and the labelled time
// away (MarkAway), whose labels it adds to ovs under the views' negative ids.
func (a *App) decorate(vs []classify.SpanView, ovs map[int64]classify.Override, from, to int64) ([]classify.SpanView, error) {
	vs, err := a.markCalls(vs, from, to)
	if err != nil {
		return nil, err
	}
	aw, err := a.st.Aways(from, to)
	if err != nil {
		return nil, err
	}
	var marks []classify.AwayMark
	for _, r := range aw {
		if r.Category != "" || r.Project != "" {
			marks = append(marks, classify.AwayMark{ID: r.ID, Interval: classify.Interval{From: r.StartMS, To: r.EndMS}})
			ovs[-r.ID] = classify.Override{Category: r.Category, Project: r.Project}
		}
	}
	return classify.MarkAway(vs, marks, from, to), nil
}

// markCalls attaches shell dirs and marks the calls of at least CallMinMS.
func (a *App) markCalls(vs []classify.SpanView, from, to int64) ([]classify.SpanView, error) {
	if a.paths.ShellLog != "" {
		ev, err := a.shell.Events()
		a.shellErr = err
		if err == nil {
			shell.Attach(vs, ev, a.cfg.Shell.Terminals, a.home)
		}
	}
	if a.rs.CallCategory == "" || len(vs) == 0 {
		return vs, nil
	}
	cs, err := a.st.Calls(from, to)
	if err != nil {
		return nil, err
	}
	var iv []classify.Interval
	for _, c := range cs {
		if c.EndMS-c.StartMS >= CallMinMS {
			iv = append(iv, classify.Interval{From: c.StartMS, To: c.EndMS})
		}
	}
	return classify.MarkCalls(vs, iv), nil
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
	vs, err := a.decorate(views(rows, math.MinInt64, math.MaxInt64, a.nowMS()), o, math.MinInt64, math.MaxInt64)
	if err != nil {
		return nil, nil, nil, err
	}
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
	target, err := a.targetFor(from)
	if err != nil {
		return report.Day{}, err
	}
	d := report.BuildDay(report.DayInput{Date: from, NowMS: now, Views: vs, Labels: labels, Ignored: ignored,
		Target: target, Weights: a.cfg.Score})
	if d.Away, err = a.pending(vs, from.UnixMilli(), to.UnixMilli()); err != nil {
		return report.Day{}, err
	}
	d.Commits = a.commits(from.UnixMilli(), to.UnixMilli())
	if goals := a.rs.Goals; len(goals) > 0 {
		mon := report.MondayOf(from)
		wvs, wls, err := a.labelled(mon.UnixMilli(), to.UnixMilli(), now)
		if err != nil {
			return report.Day{}, err
		}
		d.Goals = report.Goals(goals, wvs, wls, from, mon)
	}
	return d, nil
}

// Week reports the Monday-start week containing anyDay.
func (a *App) Week(anyDay time.Time) (report.Week, error) {
	mon := report.MondayOf(a.midnight(anyDay))
	now := a.nowMS()
	vs, labels, err := a.labelled(mon.UnixMilli(), mon.AddDate(0, 0, 7).UnixMilli(), now)
	if err != nil {
		return report.Week{}, err
	}
	in := report.WeekInput{Monday: mon, NowMS: now, Views: vs, Labels: labels, Weights: a.cfg.Score, Goals: a.rs.Goals}
	today := a.midnight(a.now())
	for i := range in.Targets {
		day := mon.AddDate(0, 0, i)
		if day.After(today) {
			break
		}
		if in.Targets[i], err = a.targetFor(day); err != nil {
			return report.Week{}, err
		}
	}
	w := report.BuildWeek(in)
	w.Commits = a.commits(mon.UnixMilli(), mon.AddDate(0, 0, 7).UnixMilli())
	if w.Plan, err = a.st.Plan(dateKey(mon)); err != nil {
		return report.Week{}, err
	}
	return w, nil
}

// SetPlan saves the plan of the Monday-start week containing anyDay.
func (a *App) SetPlan(anyDay time.Time, text string) error {
	return a.st.SetPlan(dateKey(report.MondayOf(a.midnight(anyDay))), report.CleanPlan(text), a.nowMS())
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
			if n := len(ids); n == 0 || ids[n-1] != v.ID { // a call splits a span into adjacent pieces
				ids = append(ids, v.ID)
			}
			ms += v.DurMS()
		}
	}
	return ids, ms, nil
}
