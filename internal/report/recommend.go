package report

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"tock/internal/recommend"
)

// profileWeekdays is the week as the reports show it, Monday first.
var profileWeekdays = [7]time.Weekday{time.Monday, time.Tuesday, time.Wednesday, time.Thursday,
	time.Friday, time.Saturday, time.Sunday}

// profileMinutes is ms as whole minutes, half up.
func profileMinutes(ms int64) int64 { return (ms + 30_000) / 60_000 }

// ProfileLines is the "Best focus hours" section of `tock trends`: a blank
// line, the title, each weekday's top hours in clock order, then the typical
// session and the sustained focus. A thin history says so in the title and
// lists no hours.
func ProfileLines(p recommend.Profile) []string {
	title := "Best focus hours"
	if p.Thin {
		title = fmt.Sprintf("Best focus hours (not enough history: %d of %d days)", p.ActiveDays, p.MinDays)
	}
	out := []string{"", title}
	for _, wd := range profileWeekdays {
		hs := append([]int(nil), p.Top[wd]...)
		sort.Ints(hs)
		parts := make([]string, len(hs))
		for i, h := range hs {
			parts[i] = fmt.Sprintf("%02d–%02d", h, (h+1)%24)
		}
		hours := "–"
		if len(parts) > 0 {
			hours = strings.Join(parts, " · ")
		}
		out = append(out, wd.String()[:3]+"  "+hours)
	}
	if p.GoodSessions > 0 {
		out = append(out, fmt.Sprintf("Typical session: %d min (from %d good sessions)", profileMinutes(p.SessionMS), p.GoodSessions))
	} else {
		out = append(out, fmt.Sprintf("Typical session: %d min (default, not enough sessions yet)", profileMinutes(p.SessionMS)))
	}
	if p.SustainedMS > 0 {
		out = append(out, fmt.Sprintf("Sustained focus: %d min before a break", profileMinutes(p.SustainedMS)))
	} else {
		out = append(out, "Sustained focus: (not enough history)")
	}
	return out
}

// HourGrid is the weekday x hour table of mean focus minutes: hour columns
// 07 to 22, widened to every hour that has data, "·" where a weekday has no
// data at that hour.
func HourGrid(p recommend.Profile) []string {
	from, to := 7, 22
	for wd := range p.Hours {
		for h, st := range p.Hours[wd] {
			if st.Samples > 0 {
				from, to = min(from, h), max(to, h)
			}
		}
	}
	row := func(label string, cell func(h int) string) string {
		var b strings.Builder
		b.WriteString(label)
		for h := from; h <= to; h++ {
			fmt.Fprintf(&b, "%4s", cell(h))
		}
		return strings.TrimRight(b.String(), " ")
	}
	out := []string{"Mean focus minutes per hour, last 28 days",
		row("   ", func(h int) string { return fmt.Sprintf("%02d", h) })}
	for _, wd := range profileWeekdays {
		out = append(out, row(wd.String()[:3], func(h int) string {
			st := p.Hours[wd][h]
			if st.Samples == 0 {
				return "·"
			}
			return fmt.Sprint(profileMinutes(st.MeanMS))
		}))
	}
	return out
}

// ReplyLines are the two lines `tock trends` ends with: how many suggestions and
// break reminders were answered with the yes button, and the hours the replies
// muted. Both count the last recommend.ReplyDays only.
func ReplyLines(l recommend.Learning) []string {
	return []string{
		replyLine("Suggestions", "accepted", l, recommend.KindFocus),
		replyLine("Breaks", "taken", l, recommend.KindBreak),
	}
}

func replyLine(label, verb string, l recommend.Learning, kind string) string {
	accepted, answered := l.Totals(kind)
	if answered == 0 {
		return label + ": none answered yet"
	}
	line := fmt.Sprintf("%s: %d of %d %s", label, accepted, answered, verb)
	var muted []string
	for _, h := range l.MutedHours(kind) {
		muted = append(muted, fmt.Sprintf("%02d–%02d", h, (h+1)%24))
	}
	if len(muted) > 0 {
		line += " · muted at " + strings.Join(muted, ", ")
	}
	return line
}
