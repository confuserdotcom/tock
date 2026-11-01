package recommend

import "tock/internal/config"

// Bounds of the adaptive break (12-CONTEXT "Adaptive break").
const (
	// BreakAfterMinMS and BreakAfterMaxMS keep the learned sustained-focus
	// length from becoming a nag or a wall.
	BreakAfterMinMS = 25 * 60_000
	BreakAfterMaxMS = 120 * 60_000
	// BreakMinMin and BreakMaxMin bound the suggested break length (minutes).
	BreakMinMin = 5
	BreakMaxMin = 20
)

// Adaptive reports that the break reminder follows the profile: recommendations
// and adaptive_break are on and the profile knows a sustained-focus length.
func Adaptive(p Profile, rc config.Recommend) bool {
	return rc.Enabled && rc.AdaptiveBreak && p.SustainedMS > 0
}

// BreakAfterMS is how long an activity run lasts before the first break
// reminder: the typical sustained-focus length kept within BreakAfterMinMS..
// BreakAfterMaxMS when Adaptive, else fallbackMS (the [focus] break_after).
func BreakAfterMS(p Profile, rc config.Recommend, fallbackMS int64) int64 {
	if !Adaptive(p, rc) {
		return fallbackMS
	}
	return min(max(p.SustainedMS, BreakAfterMinMS), BreakAfterMaxMS)
}

// BreakMinutes is the break length to suggest after a run of runMS: a fifth of
// the run in whole minutes, rounded to 5 min (half up) and kept within
// BreakMinMin..BreakMaxMin; always BreakMinMin when not adaptive.
func BreakMinutes(runMS int64, adaptive bool) int64 {
	if !adaptive {
		return BreakMinMin
	}
	m := (max(runMS, 0) + 30_000) / 60_000
	return min(max((m+12)/25*5, BreakMinMin), BreakMaxMin)
}
