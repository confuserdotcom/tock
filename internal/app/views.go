package app

import (
	"math"
	"time"

	"tock/internal/classify"
	"tock/internal/report"
	"tock/internal/shell"
	"tock/internal/span"
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
		v := classify.SpanView{
			ID: r.ID, Start: start, End: end,
			BundleID: r.BundleID, App: r.App,
			Title: r.Title, URL: r.URL, Private: r.Private,
		}
		if r.Kept != nil {
			v.Kept = &classify.Kept{Category: r.Kept.Category, Project: r.Kept.Project}
		}
		out = append(out, v)
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
//
// The rows start up to classify.MaxAFKMS earlier, so a pause that began
// before from is judged with the span in front of it (see base).
func (a *App) spans(from, to int64) ([]store.SpanRow, map[int64]classify.Override, error) {
	if from > math.MinInt64+classify.MaxAFKMS {
		from -= classify.MaxAFKMS
	}
	rows, err := a.st.Spans(from, to)
	if err != nil {
		return nil, nil, err
	}
	ovs, err := a.refreshPast()
	if err != nil {
		return nil, nil, err
	}
	return rows, overrides(ovs), nil
}

// refreshPast drops the cached past days when what labels them changed:
// the overrides, the away rows, the events, the other machines' views, the
// edits, the plan or when input began to be known. It returns the
// overrides.
func (a *App) refreshPast() (map[int64]store.Override, error) {
	ovs, err := a.st.Overrides()
	if err != nil {
		return nil, err
	}
	n, lastAt, err := a.st.AwayStamp()
	if err != nil {
		return nil, err
	}
	en, eid, err := a.st.EventStamp()
	if err != nil {
		return nil, err
	}
	rn, rid, err := a.st.RemoteStamp()
	if err != nil {
		return nil, err
	}
	dn, dsum, err := a.st.EditStamp()
	if err != nil {
		return nil, err
	}
	if fp := fingerprint(ovs) ^ uint64(n)<<48 ^ uint64(lastAt) ^ uint64(en)<<32 ^ uint64(eid)<<8 ^
		uint64(rn)<<40 ^ uint64(rid)<<16 ^ uint64(dn)<<52 ^ uint64(dsum)*31 ^ a.planStamp()*17 ^ uint64(a.inputSince)*13; fp != a.pastKey {
		a.past, a.pastKey = nil, fp
	}
	return ovs, nil
}

// labelled loads the spans overlapping [from, to), clipped, with their
// labels, the local model's included (see labelModel). The open span ends at nowMS; callers pass the same nowMS to the
// report so it can tell which row is open.
func (a *App) labelled(from, to, nowMS int64) ([]classify.SpanView, []classify.Label, error) {
	a.cut = report.Uncounted{}
	rows, ovs, err := a.spans(from, to)
	if err != nil {
		return nil, nil, err
	}
	vs, err := a.decorate(a.base(rows, ovs, from, to, nowMS), ovs, from, to)
	if err != nil {
		return nil, nil, err
	}
	labels := a.rs.LabelAll(vs, ovs)
	cut := a.cut
	m, err := a.labelModel() // its history() counts its own cuts
	a.cut = cut
	if err != nil {
		return nil, nil, err
	}
	a.rs.ApplyModel(m, vs, labels)
	return vs, labels, nil
}

// CallMinMS is the shortest call that labels time, which keeps dictation and
// Siri out.
const CallMinMS = 2 * 60_000

// decorate adds what is not in the span table to views clipped to
// [from, to) by base: the keywords read on screen, then every source that
// claims time (see classify.Resolve): the calls when the rules name a call
// category, the other machines' views, the labelled time away, the plan,
// the calendar events when the rules name a calendar category, and the
// timeline edits. The labels of away rows, remote views and edits go into
// ovs under their views' negative ids.
func (a *App) decorate(vs []classify.SpanView, ovs map[int64]classify.Override, from, to int64) ([]classify.SpanView, error) {
	if err := a.markScreen(vs); err != nil {
		return nil, err
	}
	src := classify.Sources{NowMS: a.nowMS(), Plan: a.planSlots(from, to)}
	var err error
	if src.Calls, err = a.calls(vs, from, to); err != nil {
		return nil, err
	}
	if src.Remote, err = a.remote(vs, src.Calls, ovs, from, to); err != nil {
		return nil, err
	}
	aw, err := a.st.Aways(from, to)
	if err != nil {
		return nil, err
	}
	for _, r := range aw {
		if r.Category != "" || r.Project != "" {
			src.Away = append(src.Away, classify.AwayMark{ID: r.ID, Interval: classify.Interval{From: r.StartMS, To: r.EndMS}})
			ovs[-r.ID] = classify.Override{Category: r.Category, Project: r.Project}
		}
	}
	if a.rs.CalendarCategory != "" {
		evs, err := a.st.Events(from, to)
		if err != nil {
			return nil, err
		}
		src.Events = make([]classify.EventMark, len(evs))
		for i, e := range evs {
			src.Events[i] = classify.EventMark{ID: e.ID, Title: e.Title, Interval: classify.Interval{From: e.StartMS, To: e.EndMS}}
		}
	}
	if src.Edits, err = a.edits(ovs, from, to); err != nil {
		return nil, err
	}
	return classify.Resolve(vs, src, from, to), nil
}

// remote is the other machines' views over [from, to), with the stale rule
// applied. Their labels go into ovs under the views' ids when the category
// and project exist here, unless a module here names the view; otherwise
// this machine's rules label them. It counts in a.cut the time the stale
// rule took and the local time they took.
func (a *App) remote(vs []classify.SpanView, calls []classify.Interval, ovs map[int64]classify.Override, from, to int64) ([]classify.RemoteMark, error) {
	rv, err := a.st.RemoteViews(from, to)
	if err != nil || len(rv) == 0 {
		return nil, err
	}
	whole := remoteMarks(rv)
	rv = a.staleRemote(rv)
	marks := remoteMarks(rv)
	for i, r := range rv {
		var o classify.Override
		if p, _ := a.rs.ModuleFor(marks[i].View); p != "" {
			continue // this machine's modules know better
		}
		// Names the other machine used before a rename follow the log.
		if r.Category != "" {
			o.Category = a.rs.CurrentName(classify.NameCategory, r.Category)
		}
		if r.Project != "" {
			o.Project = a.rs.CurrentName(classify.NameProject, r.Project)
		}
		if o != (classify.Override{}) {
			ovs[-(classify.RemoteIDBase + r.ID)] = o
		}
	}
	// What the stale rule took is what the uncut views would have added.
	now := a.nowMS()
	out := classify.Resolve(vs, classify.Sources{Calls: calls, Remote: marks, NowMS: now}, from, to)
	all := classify.Resolve(vs, classify.Sources{Calls: calls, Remote: whole, NowMS: now}, from, to)
	a.cut.StaleMS += remoteMS(all) - remoteMS(out)
	a.cut.OtherMS += localMS(clipViews(vs, from, to)) - localMS(out)
	return marks, nil
}

func remoteMarks(rv []store.RemoteView) []classify.RemoteMark {
	marks := make([]classify.RemoteMark, len(rv))
	for i, r := range rv {
		marks[i] = classify.RemoteMark{ID: r.ID, Machine: r.Machine, View: classify.SpanView{
			Start: r.StartMS, End: r.EndMS, BundleID: r.Bundle, App: r.App, Title: r.Title, URL: r.URL}}
	}
	return marks
}

// localMS is the time of this machine's recorded activity: not idle, not
// a call over idle time.
func localMS(vs []classify.SpanView) int64 {
	var ms int64
	for _, v := range vs {
		if v.ID > 0 && v.BundleID != span.IdleBundleID && v.BundleID != span.CallBundleID {
			ms += v.DurMS()
		}
	}
	return ms
}

// remoteMS is the time of the other machines' views.
func remoteMS(vs []classify.SpanView) int64 {
	var ms int64
	for _, v := range vs {
		if classify.IsRemote(v) {
			ms += v.DurMS()
		}
	}
	return ms
}

// attachShell sets the shell's working directory on terminal views.
func (a *App) attachShell(vs []classify.SpanView) {
	if a.paths.ShellLog == "" {
		return
	}
	ev, err := a.shell.Events()
	a.shellErr = err
	if err == nil {
		shell.Attach(vs, ev, a.cfg.Shell.Terminals, a.home)
	}
}

// calls is the calls of at least CallMinMS, when the rules name a call
// category.
func (a *App) calls(vs []classify.SpanView, from, to int64) ([]classify.Interval, error) {
	if a.rs.CallCategory == "" || len(vs) == 0 {
		return nil, nil
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
	return iv, nil
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
	vs, err := a.decorate(a.base(rows, o, math.MinInt64, math.MaxInt64, a.nowMS()), o, math.MinInt64, math.MaxInt64)
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
	cut := a.cut
	ignored, err := a.st.Ignored()
	if err != nil {
		return report.Day{}, err
	}
	target, err := a.targetFor(from)
	if err != nil {
		return report.Day{}, err
	}
	d := report.BuildDay(report.DayInput{Date: from, NowMS: now, Views: vs, Labels: labels, Ignored: ignored,
		Target: target, Weights: a.cfg.Score, Hours: a.cfg.Hours})
	d.Cut = cut
	if d.Away, err = a.pending(vs, from.UnixMilli(), to.UnixMilli()); err != nil {
		return report.Day{}, err
	}
	for _, iv := range d.Away {
		d.AwayBefore = append(d.AwayBefore, classify.Before(vs, labels, iv.From))
	}
	d.Commits = a.commits(from.UnixMilli(), to.UnixMilli())
	if d.Outages, err = a.Outages(from.UnixMilli(), to.UnixMilli()); err != nil {
		return report.Day{}, err
	}
	if err := a.fillSummaries(from, &d.Summary, func(start int64, text string) {
		for i := range d.Blocks {
			if d.Blocks[i].Start == start {
				d.Blocks[i].Summary = text
			}
		}
	}); err != nil {
		return report.Day{}, err
	}
	if goals := a.rs.Goals; len(goals) > 0 {
		mon := report.MondayOf(from)
		wvs, wls, err := a.labelled(mon.UnixMilli(), to.UnixMilli(), now)
		if err != nil {
			return report.Day{}, err
		}
		d.Goals = report.Goals(goals, wvs, wls, from, mon)
		st, err := a.goalStreaks(from, wvs, wls)
		if err != nil {
			return report.Day{}, err
		}
		for i := range d.Goals {
			d.Goals[i].Streak, d.Goals[i].Best = st[i][0], st[i][1]
		}
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
	in := report.WeekInput{Monday: mon, NowMS: now, Views: vs, Labels: labels, Weights: a.cfg.Score, Goals: a.rs.Goals,
		Hours: a.cfg.Hours}
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
	if w.Outages, err = a.Outages(mon.UnixMilli(), mon.AddDate(0, 0, 7).UnixMilli()); err != nil {
		return report.Week{}, err
	}
	if len(w.Goals) > 0 {
		// Streaks as of today, or the week's Sunday for a past week.
		anchor := mon.AddDate(0, 0, 6)
		if today.Before(anchor) {
			anchor = today
		}
		st, err := a.goalStreaks(anchor, vs, labels)
		if err != nil {
			return report.Week{}, err
		}
		for i := range w.Goals {
			w.Goals[i].Streak, w.Goals[i].Best = st[i][0], st[i][1]
		}
	}
	w.Modules = a.modules(vs, labels)
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

// markScreen attaches the keywords read on screen to this machine's views.
// They stay local: no payload, sync or export reads them.
func (a *App) markScreen(vs []classify.SpanView) error {
	ids := make([]int64, 0, len(vs))
	for _, v := range vs {
		if v.ID > 0 && !v.Private {
			ids = append(ids, v.ID)
		}
	}
	if len(ids) == 0 {
		return nil
	}
	words, err := a.st.SpanWords(ids)
	if err != nil || len(words) == 0 {
		return err
	}
	for i := range vs {
		if w := words[vs[i].ID]; w != nil && vs[i].ID > 0 && !vs[i].Private {
			vs[i].Screen = w
		}
	}
	return nil
}
