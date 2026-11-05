package app

import (
	"slices"
	"time"
)

// UsualWeeks is how many past weeks the usual day looks at.
const UsualWeeks = 4

// Usual is a typical day to compare a day with: the median of the same
// weekday over the last UsualWeeks weeks, counting only days with time
// tracked. Days is how many days it was taken from, 0 when fewer than two
// had any (then there is nothing to compare with).
type Usual struct {
	Days            int
	Score           int
	TrackedMS       int64
	FocusMS         int64
	LongestMS       int64
	DistractMS      int64
	SwitchesPerHour float64
}

// Usual returns the usual day for day's weekday.
func (a *App) Usual(day time.Time) (Usual, error) {
	var score, tracked, focus, longest, distract []int64
	var switches []float64
	from := a.midnight(day)
	for w := 1; w <= UsualWeeks; w++ {
		d, err := a.Day(from.AddDate(0, 0, -7*w))
		if err != nil {
			return Usual{}, err
		}
		if d.TrackedMS == 0 {
			continue
		}
		score = append(score, int64(d.Score.Value))
		tracked = append(tracked, d.TrackedMS)
		focus = append(focus, d.Stats.FocusMS)
		longest = append(longest, d.Stats.LongestMS)
		distract = append(distract, d.Stats.DistractMS)
		switches = append(switches, d.Score.SwitchesPerHour)
	}
	if len(score) < 2 {
		return Usual{}, nil
	}
	return Usual{Days: len(score), Score: int(median(score)), TrackedMS: median(tracked), FocusMS: median(focus),
		LongestMS: median(longest), DistractMS: median(distract), SwitchesPerHour: median(switches)}, nil
}

// median of xs, the mean of the middle two for an even count.
func median[T int64 | float64](xs []T) T {
	s := slices.Clone(xs)
	slices.Sort(s)
	n := len(s)
	if n%2 == 1 {
		return s[n/2]
	}
	return (s[n/2-1] + s[n/2]) / 2
}
