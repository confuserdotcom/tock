package stats

import "testing"

// work is a workday with focus minutes of focus time.
func work(focus int64) Day { return Day{ActiveMS: 2 * 60 * mn, FocusMS: focus * mn} }

func TestTargetFrom(t *testing.T) {
	off := Day{ActiveMS: 59 * mn, FocusMS: 50 * mn} // under an hour active: not a workday
	repeat := func(n int, d Day) []Day {
		out := make([]Day, n)
		for i := range out {
			out[i] = d
		}
		return out
	}
	tests := []struct {
		name string
		prev []Day
		want Target
	}{
		{"no history", nil, Target{MS: DefaultTargetMS}},
		{"6 workdays: default", repeat(6, work(180)), Target{MS: DefaultTargetMS, Workdays: 6}},
		{"7 workdays: adaptive", repeat(7, work(180)), Target{MS: 198 * mn, Adaptive: true, Workdays: 7}},
		{"non-workdays are skipped",
			append(append(repeat(3, work(180)), off, Day{}, off), repeat(3, work(180))...),
			Target{MS: DefaultTargetMS, Workdays: 6}},
		{"odd count takes the middle",
			[]Day{work(100), work(300), work(150), work(200), work(120), work(250), work(130)},
			Target{MS: 165 * mn, Adaptive: true, Workdays: 7}}, // median 150
		{"even count takes the mean of the middle two",
			[]Day{work(100), work(300), work(150), work(200), work(120), work(250), work(130), work(170)},
			Target{MS: 176 * mn, Adaptive: true, Workdays: 8}}, // median (150+170)/2 = 160
		{"rounds to the minute", repeat(7, Day{ActiveMS: 2 * 60 * mn, FocusMS: 125*mn + 20*sec}),
			Target{MS: 138 * mn, Adaptive: true, Workdays: 7}}, // 137.87 min
		{"clamped at 2 h", repeat(7, work(30)), Target{MS: MinTargetMS, Adaptive: true, Workdays: 7}},
		{"clamped at 6 h", repeat(7, work(400)), Target{MS: MaxTargetMS, Adaptive: true, Workdays: 7}},
		{"latest 14 workdays only",
			append(repeat(14, work(200)), repeat(10, work(30))...),
			Target{MS: 220 * mn, Adaptive: true, Workdays: 14}},
		{"days past LookbackDays are ignored",
			append(repeat(LookbackDays, Day{}), repeat(7, work(200))...),
			Target{MS: DefaultTargetMS}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := TargetFrom(tt.prev); got != tt.want {
				t.Fatalf("TargetFrom = %+v (%.2f min), want %+v (%.2f min)",
					got, float64(got.MS)/float64(mn), tt.want, float64(tt.want.MS)/float64(mn))
			}
		})
	}
}
