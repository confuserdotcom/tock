package app

import (
	"fmt"
	"time"

	"tock/internal/recommend"
)

// profileCache is the recommendation profile of one local day.
type profileCache struct {
	p   *recommend.Profile
	day string // dateKey of the day p was built for
}

// Profile returns what tock learned about when the user focuses best, built
// from the recommend.HourDays days before the day of now. It is built once
// per local day (and again after a config or rules change) and kept; the days
// come from the same cache the daily target uses. A day whose history changed
// while it was read is an error, so the next call tries again.
func (a *App) Profile(now time.Time) (recommend.Profile, error) {
	day := a.midnight(now)
	key := dateKey(day)
	if a.profile.p != nil && a.profile.day == key {
		return *a.profile.p, nil
	}
	from := day.AddDate(0, 0, -recommend.HourDays)
	var days []recommend.Day
	// A second pass covers overrides changing between two reads, which drops
	// the cache half way, as in targetFor.
	complete := false
	for range 2 {
		if err := a.fillPast(from, day); err != nil {
			return recommend.Profile{}, fmt.Errorf("app: profile: %w", err)
		}
		days = days[:0]
		complete = true
		for d := from; d.Before(day); d = d.AddDate(0, 0, 1) {
			p, ok := a.past[dateKey(d)]
			if !ok {
				complete = false
				break
			}
			days = append(days, recommend.Day{Date: d, ActiveMS: p.ActiveHours, FocusMS: p.DeepHours})
		}
		if complete {
			break
		}
	}
	if !complete {
		return recommend.Profile{}, fmt.Errorf("app: profile: history changed while it was read")
	}
	p := recommend.Build(recommend.Input{Days: days, MinDays: a.cfg.Recommend.MinDays,
		DefaultSessionMS: int64(a.cfg.Focus.Presets[0]) * 60_000})
	a.profile = profileCache{p: &p, day: key}
	return p, nil
}
