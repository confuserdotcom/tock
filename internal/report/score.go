package report

import (
	"fmt"
	"strings"
	"time"
)

// ScoreLine renders the plain score summary shared by the TUI and tock
// today, for example
// "score 72 · focus 2h10 / 4h00 · 3 blocks, longest 48m · 14 switches".
// With no active time it is "score — · no activity".
func ScoreLine(d Day) string {
	if d.Off {
		return "day off · not scored"
	}
	st, sc := d.Stats, d.Score
	var parts []string
	if st.ActiveMS <= 0 {
		parts = []string{"score —", "no activity"}
	} else {
		blocks := "no blocks"
		if n := len(st.Blocks); n > 0 {
			blocks = fmt.Sprintf("%d %s, longest %s", n, plural(n, "block", "blocks"), Dur(st.LongestMS))
		}
		parts = []string{
			fmt.Sprintf("score %d", sc.Value),
			fmt.Sprintf("focus %s / %s", Dur(st.FocusMS), Dur(sc.Target.MS)),
			blocks,
			fmt.Sprintf("%d %s", st.Switches, plural(st.Switches, "switch", "switches")),
		}
	}
	if d.Work.Set {
		parts = append(parts, HoursText(d))
		if d.OutsideMS >= 60_000 {
			parts = append(parts, Dur(d.OutsideMS)+" outside hours")
		}
	}
	return strings.Join(parts, " · ")
}

// HoursText is the day's work hours: "09:00–18:00", "day off", or "" when
// no schedule is set.
func HoursText(d Day) string {
	switch {
	case !d.Work.Set:
		return ""
	case d.Off:
		return "day off"
	}
	loc := d.Date.Location()
	return time.UnixMilli(d.From).In(loc).Format("15:04") + "–" + time.UnixMilli(d.To).In(loc).Format("15:04")
}

func plural(n int, one, many string) string {
	if n == 1 {
		return one
	}
	return many
}
