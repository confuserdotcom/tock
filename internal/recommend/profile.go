package recommend

import (
	"sort"
	"time"
)

// Day is one past local day: the active (non-idle) and the focus (deep-kind)
// milliseconds per wall-clock hour.
type Day struct {
	Date              time.Time // local midnight
	ActiveMS, FocusMS [24]int64
}

// Input is everything Build learns from.
type Input struct {
	// Days are the days before today inside the HourDays window, any order.
	Days []Day
	// MinDays is [recommend] min_days: active days the profile needs.
	MinDays int
	// DefaultSessionMS is the [focus] default session length.
	DefaultSessionMS int64
}

// HourStat is the score of one (weekday, hour): the days of that weekday with
// active time in that hour and their mean focus time.
type HourStat struct {
	Samples int
	MeanMS  int64
}

// Profile is what tock knows about when and for how long the user focuses.
type Profile struct {
	MinDays    int
	ActiveDays int
	// Thin: fewer than MinDays active days; no Top hours then.
	Thin bool
	// Hours is indexed by time.Weekday and the hour of day.
	Hours [7][24]HourStat
	// Top holds each weekday's best hours, best first.
	Top [7][]int
	// SessionMS is the typical successful session length; GoodSessions is
	// how many sessions it comes from, 0 = it is the default.
	SessionMS    int64
	GoodSessions int
}

// Build computes the profile. A day is active when any hour holds active
// time; a (weekday, hour) sample is an active day of that weekday with active
// time in that hour.
func Build(in Input) Profile {
	p := Profile{MinDays: in.MinDays, SessionMS: in.DefaultSessionMS}
	var sum [7][24]int64
	for _, d := range in.Days {
		wd := d.Date.Weekday()
		active := false
		for h := 0; h < 24; h++ {
			if d.ActiveMS[h] <= 0 {
				continue
			}
			active = true
			p.Hours[wd][h].Samples++
			sum[wd][h] += d.FocusMS[h]
		}
		if active {
			p.ActiveDays++
		}
	}
	for wd := range p.Hours {
		for h := range p.Hours[wd] {
			if n := p.Hours[wd][h].Samples; n > 0 {
				p.Hours[wd][h].MeanMS = sum[wd][h] / int64(n)
			}
		}
	}
	p.Thin = p.ActiveDays < in.MinDays
	if p.Thin {
		return p
	}
	for wd := range p.Hours {
		var hs []int
		for h, st := range p.Hours[wd] {
			if st.Samples >= MinSamples && st.MeanMS >= MinHourMS {
				hs = append(hs, h)
			}
		}
		sort.SliceStable(hs, func(a, b int) bool {
			ma, mb := p.Hours[wd][hs[a]].MeanMS, p.Hours[wd][hs[b]].MeanMS
			if ma != mb {
				return ma > mb
			}
			return hs[a] < hs[b]
		})
		if len(hs) > TopHours {
			hs = hs[:TopHours]
		}
		p.Top[wd] = hs
	}
	return p
}

// Rank is the 1-based position of hour in the weekday's top hours, 0 when it
// is not one.
func (p Profile) Rank(wd time.Weekday, hour int) int {
	for i, h := range p.Top[wd] {
		if h == hour {
			return i + 1
		}
	}
	return 0
}
