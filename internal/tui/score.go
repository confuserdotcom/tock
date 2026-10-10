package tui

import (
	"fmt"
	"math"
	"strings"
	"time"
	"unicode/utf8"

	"tock/internal/report"
	"tock/internal/stats"
)

// scoreView is the today tab's score breakdown: each term with its weight
// and contribution, the penalty, how the target was set and the blocks.
func (t todayTab) scoreView(w, h int, loc *time.Location) []string {
	if !t.loaded {
		return []string{"", " loading…"}
	}
	d := t.day
	st, sc := d.Stats, d.Score
	date := d.Date.Format("Mon 2 Jan")
	if st.ActiveMS <= 0 {
		return []string{" " + sHead.Render("Score · "+date), "", "  no activity on this day"}
	}
	term := func(name string, v, weight float64, why string) string {
		return fmt.Sprintf("  %-8s %4.2f × %.1f = %4.1f   %s", name, v, weight, 100*v*weight, why)
	}
	sum := int(math.Round(sc.Sum()))
	out := []string{
		" " + sHead.Render(fmt.Sprintf("Score %d · %s", sc.Value, date)),
		"",
		term("depth", sc.Depth, sc.Weights.Depth,
			fmt.Sprintf("focus %s of target %s", report.Dur(st.FocusMS), report.Dur(sc.Target.MS))),
		term("blocks", sc.Blocks, sc.Weights.Blocks,
			fmt.Sprintf("%s in blocks of %s or more", report.Dur(st.LongMS), report.Dur(stats.LongBlockMS))),
		term("calm", sc.Calm, sc.Weights.Calm,
			fmt.Sprintf("%s in %s active = %.1f an hour (%d = 0)", switches(st.Switches),
				report.Dur(st.ActiveMS), sc.SwitchesPerHour, stats.CalmZeroPerHour)),
		term("stretch", sc.Stretch, sc.Weights.Stretch,
			fmt.Sprintf("longest %s without a switch (%s = 1)", report.Dur(st.LongestRunMS), report.Dur(stats.StretchFullMS))),
		fmt.Sprintf("  %-8s            − %4d   %s distraction, 1 point per %dm, at most %d", "penalty",
			sc.Penalty, report.Dur(st.DistractMS), stats.PenaltyPerMin, stats.PenaltyCap),
		strings.Repeat(" ", 24) + "────",
		alignRight(fmt.Sprintf("%d − %d = %d", sum, sc.Penalty, sc.Value), 28) + "   (sum rounded, then penalty, 0–100)",
		"",
		"  target   " + targetText(sc.Target),
		"",
	}
	room := max(h-len(out), 1)
	lines := blockLines(st.Blocks, loc)
	if len(lines) > room {
		more := len(lines) - room + 1
		lines = append(lines[:room-1], fmt.Sprintf("+%d more", more))
	}
	for i, l := range lines {
		lead := "           "
		if i == 0 {
			lead = "  blocks   "
		}
		out = append(out, fit(lead+l, w))
	}
	return out
}

// alignRight left-pads s with spaces to n runes.
func alignRight(s string, n int) string {
	return strings.Repeat(" ", max(n-utf8.RuneCountInString(s), 0)) + s
}

func switches(n int) string {
	if n == 1 {
		return "1 switch"
	}
	return fmt.Sprintf("%d switches", n)
}

func targetText(t stats.Target) string {
	if !t.Adaptive {
		return fmt.Sprintf("%s default: %d of %d workdays so far", report.Dur(t.MS), t.Workdays, stats.MinTargetDays)
	}
	return fmt.Sprintf("%s = median focus of the last %d workdays × 1.1, 2h–6h", report.Dur(t.MS), t.Workdays)
}

func blockLines(bs []stats.Block, loc *time.Location) []string {
	if len(bs) == 0 {
		return []string{"none (a block is 5m of deep work; breaks under 1m are allowed)"}
	}
	out := make([]string, len(bs))
	for i, b := range bs {
		out[i] = time.UnixMilli(b.Start).In(loc).Format("15:04") + "–" + time.UnixMilli(b.End).In(loc).Format("15:04") +
			"  " + report.Dur(b.DeepMS)
	}
	return out
}
