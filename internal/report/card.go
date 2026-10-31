package report

import (
	"strconv"
	"strings"
	"time"

	"tock/internal/focus"
)

// SessionDetail is the card of one finished session: what the TUI focus tab
// and the menu bar draw (08-UI-SPEC Surfaces 1 and 3) without recomputing
// anything. Q is scored with the weights in force when it was built.
type SessionDetail struct {
	Sess focus.Session
	Q    focus.QualityScore
	Top  focus.Breakdown // categories, apps, project of the session window
}

// DeepPct is the deep share of the session, rounded half up; 0 for a session
// of no length (card line 2, "NN% deep").
func (d SessionDetail) DeepPct() int { return DeepPct(d.Sess, d.Sess.EndMS) }

// RunLine is card line 7: "1 interruption · longest run 32m", or
// "auto-detected · longest run 40m" for an auto session, which has no
// interruption count.
func (d SessionDetail) RunLine() string {
	lead := "auto-detected"
	if d.Sess.Kind != focus.KindAuto {
		lead = focus.InterruptionsText(d.Sess.Interruptions)
	}
	return lead + " · longest run " + Dur(d.Sess.LongestMS)
}

// Summary is the menu bar's first submenu row:
// "10:00–10:50 · 84% deep · 1 interruption" (auto: "... · auto-detected").
func (d SessionDetail) Summary(loc *time.Location) string {
	lead := "auto-detected"
	if d.Sess.Kind != focus.KindAuto {
		lead = focus.InterruptionsText(d.Sess.Interruptions)
	}
	clock := func(ms int64) string { return time.UnixMilli(ms).In(loc).Format("15:04") }
	return clock(d.Sess.StartMS) + "–" + clock(d.Sess.EndMS) + " · " + strconv.Itoa(d.DeepPct()) + "% deep · " + lead
}

// ShareText joins shares as "Analyse 1 24m · Anki 12m · Reading 6m" (card
// lines 5 and 6); it is "" for none, which the TUI shows as "none".
func ShareText(xs []focus.Share) string {
	parts := make([]string, len(xs))
	for i, x := range xs {
		parts[i] = x.Name + " " + Dur(x.MS)
	}
	return strings.Join(parts, " · ")
}

// Stars is the rating as "★★★★☆" (card line 8, menu parent), or "unrated"
// for 0.
func Stars(rating int) string {
	if rating < 1 {
		return "unrated"
	}
	rating = min(rating, 5)
	return strings.Repeat("★", rating) + strings.Repeat("☆", 5-rating)
}
