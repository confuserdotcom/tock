package app

import (
	"hash/fnv"
	"sort"
	"strconv"
	"time"

	"tock/internal/classify"
	"tock/internal/report"
	"tock/internal/stats"
	"tock/internal/store"
)

func dateKey(t time.Time) string { return t.Format("2006-01-02") }

// fingerprint identifies a set of overrides by (span, category, project).
func fingerprint(ovs map[int64]store.Override) uint64 {
	ids := make([]int64, 0, len(ovs))
	for id := range ovs {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	h := fnv.New64a()
	for _, id := range ids {
		o := ovs[id]
		h.Write([]byte(strconv.FormatInt(id, 10) + "\x00" + o.Category + "\x00" + o.Project + "\x00"))
	}
	return h.Sum64()
}

// targetFor returns the focus target of the day starting at local midnight
// day: stats.TargetFrom over the LookbackDays days before it.
func (a *App) targetFor(day time.Time) (stats.Target, error) {
	from := day.AddDate(0, 0, -stats.LookbackDays)
	// A second pass covers overrides changing between two reads, which
	// drops the cache half way.
	for range 2 {
		if err := a.fillPast(from, day); err != nil {
			return stats.Target{}, err
		}
		prev := make([]stats.Day, 0, stats.LookbackDays)
		complete := true
		for d := day.AddDate(0, 0, -1); !d.Before(from); d = d.AddDate(0, 0, -1) {
			p, ok := a.past[dateKey(d)]
			complete = complete && ok
			prev = append(prev, p.Stats)
		}
		if complete {
			return stats.TargetFrom(prev), nil
		}
	}
	return stats.Target{}, nil
}

// pastDay is a day before today as the cache keeps it.
type pastDay struct {
	Stats  stats.Day // the scored window only
	Active bool      // the whole day has active time
	GoalMS []int64   // whole-day time per goal of a.rs.Goals
}

// fillPast computes and caches every day in [from, to) that is
// before today and not cached yet, from one span query.
func (a *App) fillPast(from, to time.Time) error {
	if today := a.midnight(a.now()); to.After(today) {
		to = today
	}
	var missing []time.Time
	for d := from; d.Before(to); d = d.AddDate(0, 0, 1) {
		if _, ok := a.past[dateKey(d)]; !ok {
			missing = append(missing, d)
		}
	}
	if len(missing) == 0 {
		return nil
	}
	first, last := missing[0], missing[len(missing)-1].AddDate(0, 0, 1)
	rows, ovs, err := a.spans(first.UnixMilli(), last.UnixMilli())
	if err != nil {
		return err
	}
	if a.past == nil {
		a.past = map[string]pastDay{}
	}
	now := a.nowMS()
	for _, d := range missing {
		from, to := d.UnixMilli(), d.AddDate(0, 0, 1).UnixMilli()
		vs, err := a.decorate(views(rows, from, to, now), ovs, from, to)
		if err != nil {
			return err
		}
		ls := a.rs.LabelAll(vs, ovs)
		wFrom, wTo := a.cfg.Hours.Window(d)
		p := pastDay{Stats: stats.Compute(stats.Clip(vs, ls, wFrom, wTo)), Active: report.Active(vs)}
		if len(a.rs.Goals) > 0 {
			p.GoalMS = report.GoalTimes(a.rs.Goals, vs, ls)
		}
		a.past[dateKey(d)] = p
	}
	return nil
}

// goalStreaks is each goal's current and best streak as of the day starting
// at local midnight day, whose views (at least) are vs.
func (a *App) goalStreaks(day time.Time, vs []classify.SpanView, ls []classify.Label) ([][2]int, error) {
	goals := a.rs.Goals
	from := day.AddDate(0, 0, -report.StreakDays)
	var past []report.GoalDay
	// A second pass covers overrides changing between two reads, as in
	// targetFor.
	for range 2 {
		if err := a.fillPast(from, day); err != nil {
			return nil, err
		}
		past = past[:0]
		complete := true
		for d := day.AddDate(0, 0, -1); !d.Before(from); d = d.AddDate(0, 0, -1) {
			p, ok := a.past[dateKey(d)]
			complete = complete && ok
			past = append(past, report.GoalDay{Date: d, Active: p.Active, Off: a.cfg.Hours.Off(d), MS: p.GoalMS})
		}
		if complete {
			break
		}
	}
	dv, dl := clip(vs, ls, day, day.AddDate(0, 0, 1))
	today := report.GoalDay{Date: day, Active: report.Active(dv), Off: a.cfg.Hours.Off(day), MS: report.GoalTimes(goals, dv, dl)}
	_, end := a.cfg.Hours.Window(day)
	if a.cfg.Hours.Off(day) {
		end = day.AddDate(0, 0, 1).UnixMilli()
	}
	closed := a.nowMS() >= end
	for j := range past {
		if len(past[j].MS) != len(goals) { // not cached: a day after today
			past[j].MS = make([]int64, len(goals))
		}
	}
	out := make([][2]int, len(goals))
	for i, g := range goals {
		out[i][0], out[i][1] = report.Streak(i, g, past, today, closed)
	}
	return out, nil
}
