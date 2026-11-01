package recommend

import (
	"testing"

	"tock/internal/config"
)

func TestBreakRules(t *testing.T) {
	rc := config.DefaultRecommend()
	prof := func(m int64) Profile { return Profile{SustainedMS: m * min1} }
	fallback := 50 * min1

	if !Adaptive(prof(52), rc) {
		t.Error("Adaptive false for a sustained profile")
	}
	off := rc
	off.AdaptiveBreak = false
	disabled := rc
	disabled.Enabled = false
	for name, c := range map[string]struct {
		p  Profile
		rc config.Recommend
	}{"thin": {Profile{}, rc}, "adaptive_break off": {prof(52), off}, "disabled": {prof(52), disabled}} {
		if Adaptive(c.p, c.rc) {
			t.Errorf("Adaptive true for %s", name)
		}
		if got := BreakAfterMS(c.p, c.rc, fallback); got != fallback {
			t.Errorf("%s: BreakAfterMS = %d, want the fallback", name, got/min1)
		}
	}
	for _, c := range []struct{ sustained, want int64 }{{52, 52}, {20, 25}, {25, 25}, {120, 120}, {150, 120}} {
		if got := BreakAfterMS(prof(c.sustained), rc, fallback); got != c.want*min1 {
			t.Errorf("BreakAfterMS(sustained %d) = %d min, want %d", c.sustained, got/min1, c.want)
		}
	}

	for _, c := range []struct{ run, want int64 }{{52, 10}, {62, 10}, {63, 15}, {30, 5}, {10, 5}, {100, 20}, {125, 20}, {0, 5}} {
		if got := BreakMinutes(c.run*min1, true); got != c.want {
			t.Errorf("BreakMinutes(%d min, adaptive) = %d, want %d", c.run, got, c.want)
		}
		if got := BreakMinutes(c.run*min1, false); got != 5 {
			t.Errorf("BreakMinutes(%d min, not adaptive) = %d, want 5", c.run, got)
		}
	}
}
