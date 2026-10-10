package app

import (
	"hash/fnv"
	"sort"
	"strconv"
	"time"

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
			s, ok := a.past[dateKey(d)]
			complete = complete && ok
			prev = append(prev, s)
		}
		if complete {
			return stats.TargetFrom(prev), nil
		}
	}
	return stats.Target{}, nil
}

// fillPast computes and caches the stats of every day in [from, to) that is
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
		a.past = map[string]stats.Day{}
	}
	now := a.nowMS()
	for _, d := range missing {
		vs := a.decorate(views(rows, d.UnixMilli(), d.AddDate(0, 0, 1).UnixMilli(), now))
		a.past[dateKey(d)] = stats.Compute(vs, a.rs.LabelAll(vs, ovs))
	}
	return nil
}
