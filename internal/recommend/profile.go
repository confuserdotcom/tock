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
	// RunMS is the day's longest activity run (focus.LongestRunMS), 0 when
	// the day had no activity.
	RunMS int64
}

// Learning thresholds of the typical session and the sustained focus.
const (
	GoodQuality      = 70 // a session of this quality or more was a good one
	GoodRating       = 4  // so was one rated this or more
	MinGoodSessions  = 5  // good sessions the typical length needs
	SessionMinMS     = 15 * 60_000
	SessionMaxMS     = 120 * 60_000
	RoundMS          = 5 * 60_000
	MinSustainedDays = 7 // days with activity the sustained length needs
)

// Session is a finished manual session as Build reads it.
type Session struct {
	StartMS, EndMS, PlannedMS int64
	Quality                   int // 0..100, with the current weights
	Rating                    int // 0 = unrated, else 1..5
}

// Input is everything Build learns from.
type Input struct {
	// Days are the days before today inside the HourDays window, any order.
	Days []Day
	// MinDays is [recommend] min_days: active days the profile needs.
	MinDays int
	// DefaultSessionMS is the [focus] default session length.
	DefaultSessionMS int64
	// NowMS bounds the sessions: those that started within SessionDays
	// before it count.
	NowMS int64
	// Sessions are finished manual sessions, any order; a running one
	// (EndMS 0) is ignored.
	Sessions []Session
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
	// SustainedMS is the typical longest activity run before a break, 0 =
	// not enough history; SustainedDays is the days with activity it comes
	// from.
	SustainedMS   int64
	SustainedDays int
}

// Build computes the profile. A day is active when any hour holds active
// time; a (weekday, hour) sample is an active day of that weekday with active
// time in that hour.
func Build(in Input) Profile {
	p := Profile{MinDays: in.MinDays, SessionMS: in.DefaultSessionMS}
	var sum [7][24]int64
	var runs []int64
	for _, d := range in.Days {
		if d.RunMS > 0 {
			runs = append(runs, d.RunMS)
		}
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
	p.SessionMS, p.GoodSessions = sessionLength(in)
	p.SustainedDays = len(runs)
	if len(runs) >= MinSustainedDays {
		p.SustainedMS = lowerMedian(runs)
	}
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

// sessionLength is the typical length of the good sessions of the last
// SessionDays days (the lower median of their planned lengths, rounded to
// RoundMS and kept within SessionMinMS..SessionMaxMS) and their count; with
// fewer than MinGoodSessions it is the default and 0.
func sessionLength(in Input) (int64, int) {
	var lens []int64
	for _, s := range in.Sessions {
		if s.EndMS <= 0 || s.StartMS < in.NowMS-SessionDays*DayMS {
			continue
		}
		if s.Quality >= GoodQuality || s.Rating >= GoodRating {
			lens = append(lens, s.PlannedMS-s.StartMS)
		}
	}
	if len(lens) < MinGoodSessions {
		return in.DefaultSessionMS, 0
	}
	m := (lowerMedian(lens) + RoundMS/2) / RoundMS * RoundMS
	return min(max(m, SessionMinMS), SessionMaxMS), len(lens)
}

// lowerMedian is the middle element of xs sorted, the lower of the two for an
// even count: a value that was really measured. xs is not empty and is
// reordered.
func lowerMedian(xs []int64) int64 {
	sort.Slice(xs, func(a, b int) bool { return xs[a] < xs[b] })
	return xs[(len(xs)-1)/2]
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
