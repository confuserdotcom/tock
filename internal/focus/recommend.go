package focus

import (
	"fmt"
	"time"

	"tock/internal/recommend"
)

// Recommendation notice copy.
const (
	TitleRecommend = "Good time to focus"
	TopicRecommend = "recommend"
)

const (
	// RecGapMS is the least time between two focus suggestions.
	RecGapMS = 60 * 60_000
	// RecMinMS is the shortest session worth suggesting (CONTEXT: skip if
	// the length is under 15 min).
	RecMinMS = 15 * 60_000
)

// RecommendDue reports that a focus suggestion could be due now, from the
// cheap conditions only, so an idle tick scans no view and reads no row:
// recommendations and suggestions are on, [focus] notify is on, now is in
// the work window of a work day, the hour is a top hour of today's weekday
// and no manual session runs. The caller then reads what stepRecommend needs.
func RecommendDue(in Input) bool {
	if !in.Rec.Enabled || in.Rec.MaxPerDay <= 0 || !in.Cfg.Notify {
		return false
	}
	if !in.Hours.Set || in.Hours.Off(in.Day) {
		return false
	}
	if from, to := in.Hours.Window(in.Day); in.Now < from || in.Now >= to {
		return false
	}
	t := time.UnixMilli(in.Now).In(in.Day.Location())
	if in.Profile.Rank(t.Weekday(), t.Hour()) == 0 {
		return false
	}
	return openSession(in) == nil
}

// RecsDue reports that the caller must read the recommendation rows (and,
// with them, whatever else stepRecommend needs) before calling Step.
func RecsDue(in Input) bool {
	return RecommendDue(in)
}

// stepRecommend suggests a focus session at a top focus hour (REC-02): when
// RecommendDue, the rows were read, fewer than [recommend] max_per_day focus
// suggestions went out today and the last was at least RecGapMS ago. The
// length is the profile's typical session; it is skipped under RecMinMS.
func stepRecommend(in Input) (Event, bool) {
	if !RecommendDue(in) || !in.RecsOK {
		return Event{}, false
	}
	if recommend.CountSince(in.Recs, recommend.KindFocus, in.Day.UnixMilli()) >= in.Rec.MaxPerDay {
		return Event{}, false
	}
	if last := recommend.Last(in.Recs, recommend.KindFocus); last != 0 && in.Now-last < RecGapMS {
		return Event{}, false
	}
	n := in.Profile.SessionMS
	if n < RecMinMS {
		return Event{}, false
	}
	body := fmt.Sprintf("Your best focus hour · start a %d min session now?", n/60_000)
	ev := note(TopicRecommend, TitleRecommend, body, false)
	ev.Kind = RecDue
	ev.Minutes = int(n / 60_000)
	ev.RecKind, ev.RecLengthMS = recommend.KindFocus, n
	return ev, true
}
