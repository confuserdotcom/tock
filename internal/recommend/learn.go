package recommend

import (
	"sort"
	"time"
)

// Thresholds of the reply learning (12-CONTEXT "Replies").
const (
	// MuteMinReplies answers with none accepted mute an hour.
	MuteMinReplies = 3
	// StrictMinReplies answers with an accept rate under StrictRatePct percent
	// make an hour need to rank first.
	StrictMinReplies = 4
	StrictRatePct    = 25
)

// HourReplies counts the answered recommendations of one hour of the day.
type HourReplies struct{ Answered, Accepted int }

// Learning is what the replies of the last ReplyDays say, per kind and hour.
type Learning struct{ Focus, Break [24]HourReplies }

// breakGapMS separates two days of break reminders sent in the same hour of the
// day: rows of one hour on one day are less than an hour apart.
const breakGapMS = 12 * 3600_000

// Learn counts the accept and dismiss replies to recs sent in the ReplyDays before nowMS, by kind
// and the hour they were sent. Unanswered rows do not count and older rows have
// aged out, so a muted hour recovers. A snoozed row is no answer. A break reminder repeats during one run,
// so all the break replies of one hour of one day make a single answer: taken
// when any of them was accepted, else dismissed.
func Learn(recs []Rec, nowMS int64) Learning {
	var l Learning
	from := nowMS - ReplyDays*DayMS
	sorted := make([]Rec, 0, len(recs))
	for _, r := range recs {
		if (r.Reply != ReplyAccept && r.Reply != ReplyDismiss) || r.AtMS < from || r.Hour < 0 || r.Hour > 23 {
			continue
		}
		sorted = append(sorted, r)
	}
	sort.SliceStable(sorted, func(a, b int) bool { return sorted[a].AtMS < sorted[b].AtMS })
	var group [24]*HourReplies // the break answer of each hour's current day
	var groupStart [24]int64
	for _, r := range sorted {
		var h *HourReplies
		switch r.Kind {
		case KindFocus:
			h = &l.Focus[r.Hour]
		case KindBreak:
			h = &l.Break[r.Hour]
			if g := group[r.Hour]; g != nil && r.AtMS-groupStart[r.Hour] < breakGapMS {
				// Another reminder of the same run: it can only turn the
				// answer into a taken break.
				if r.Reply == ReplyAccept && g.Accepted == 0 {
					g.Accepted = 1
					h.Accepted++
				}
				continue
			}
			group[r.Hour], groupStart[r.Hour] = &HourReplies{Answered: 1}, r.AtMS
			if r.Reply == ReplyAccept {
				group[r.Hour].Accepted = 1
			}
		default:
			continue
		}
		h.Answered++
		if r.Reply == ReplyAccept {
			h.Accepted++
		}
	}
	return l
}

func (l Learning) hours(kind string) *[24]HourReplies {
	switch kind {
	case KindFocus:
		return &l.Focus
	case KindBreak:
		return &l.Break
	}
	return nil
}

func (l Learning) at(kind string, hour int) HourReplies {
	if hs := l.hours(kind); hs != nil && hour >= 0 && hour < 24 {
		return hs[hour]
	}
	return HourReplies{}
}

// Muted reports an hour whose suggestions were always answered Not now: at
// least MuteMinReplies answers and none accepted.
func (l Learning) Muted(kind string, hour int) bool {
	h := l.at(kind, hour)
	return h.Answered >= MuteMinReplies && h.Accepted == 0
}

// Strict reports an hour that is not muted but is rarely taken: at least
// StrictMinReplies answers and an accept rate under StrictRatePct percent.
func (l Learning) Strict(kind string, hour int) bool {
	h := l.at(kind, hour)
	return !l.Muted(kind, hour) && h.Answered >= StrictMinReplies && h.Accepted*100 < StrictRatePct*h.Answered
}

// Totals is how many of the answered recommendations of kind were accepted.
func (l Learning) Totals(kind string) (accepted, answered int) {
	hs := l.hours(kind)
	if hs == nil {
		return 0, 0
	}
	for _, h := range hs {
		accepted += h.Accepted
		answered += h.Answered
	}
	return accepted, answered
}

// MutedHours lists the muted hours of kind in clock order.
func (l Learning) MutedHours(kind string) []int {
	var out []int
	for h := 0; h < 24; h++ {
		if l.Muted(kind, h) {
			out = append(out, h)
		}
	}
	return out
}

// FocusHour reports that the profile and the replies allow a focus suggestion
// at hour on weekday wd: the hour is ranked, not muted, and ranks first when
// its accept rate is low.
func FocusHour(p Profile, l Learning, wd time.Weekday, hour int) bool {
	rank := p.Rank(wd, hour)
	if rank == 0 || l.Muted(KindFocus, hour) {
		return false
	}
	return rank == 1 || !l.Strict(KindFocus, hour)
}
