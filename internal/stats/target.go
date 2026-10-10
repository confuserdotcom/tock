package stats

import "sort"

// Target is the daily focus target a day is scored against.
type Target struct {
	MS       int64
	Adaptive bool // false: DefaultTargetMS because fewer than MinTargetDays workdays
	Workdays int  // workdays found, at most TargetDays
}

// TargetFrom takes the days before the scored day, most recent first, at
// most LookbackDays of them (days with no activity included as zero Day
// values). The target is the median focus time of the latest TargetDays
// workdays times 1.1, rounded to the minute and clamped to
// [MinTargetMS, MaxTargetMS]; with fewer than MinTargetDays workdays it is
// DefaultTargetMS.
func TargetFrom(prev []Day) Target {
	var focus []int64
	for _, d := range prev[:min(len(prev), LookbackDays)] {
		if d.ActiveMS >= WorkdayMinMS {
			focus = append(focus, d.FocusMS)
			if len(focus) == TargetDays {
				break
			}
		}
	}
	n := len(focus)
	if n < MinTargetDays {
		return Target{MS: DefaultTargetMS, Workdays: n}
	}
	sort.Slice(focus, func(a, b int) bool { return focus[a] < focus[b] })
	median := float64(focus[n/2])
	if n%2 == 0 {
		median = float64(focus[n/2-1]+focus[n/2]) / 2
	}
	ms := int64(median*1.1/60_000+0.5) * 60_000
	return Target{MS: min(max(ms, MinTargetMS), MaxTargetMS), Adaptive: true, Workdays: n}
}
