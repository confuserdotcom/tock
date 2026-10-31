package stats

import "testing"

func TestWindowSwitches(t *testing.T) {
	vs, ls := at(deep(10*mn), distract(10*mn), deep(10*mn))
	base := vs[0].Start
	for _, tt := range []struct {
		name     string
		from, to int64
		want     int
	}{
		{"whole", 0, 30 * mn, 2},
		{"from the second view", 10 * mn, 30 * mn, 1},
		{"one view only", 10 * mn, 20 * mn, 0},
		{"touching edges add nothing", 10 * mn, 20 * mn, 0},
		{"empty window", 5 * mn, 5 * mn, 0},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if got := WindowSwitches(vs, ls, base+tt.from, base+tt.to); got != tt.want {
				t.Fatalf("WindowSwitches = %d, want %d", got, tt.want)
			}
		})
	}
	if got := WindowSwitches(nil, nil, 0, 10); got != 0 {
		t.Fatalf("no views = %d, want 0", got)
	}
}
