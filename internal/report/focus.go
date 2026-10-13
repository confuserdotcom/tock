package report

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"tock/internal/focus"
)

// Focus is the focus tab: the running session and this week's history.
type Focus struct {
	NowMS int64
	Open  *focus.Session  // the running manual session with live counters, or nil
	Today []focus.Session // closed sessions that started today, newest first
	Week  []focus.Session // closed sessions of this week before today, newest first
}

// Countdown renders ms as m:ss, rounding up to the second; negative is 0:00.
func Countdown(ms int64) string {
	s := (max(ms, 0) + 999) / 1000
	return fmt.Sprintf("%d:%02d", s/60, s%60)
}

// sessionEnd is the session's end, or for a running one nowMS capped at its
// planned end.
func sessionEnd(s focus.Session, nowMS int64) int64 {
	if s.EndMS != 0 {
		return s.EndMS
	}
	return min(nowMS, s.PlannedMS)
}

// DeepPct is deep time as a rounded percentage of the session so far.
func DeepPct(s focus.Session, nowMS int64) int {
	n := sessionEnd(s, nowMS) - s.StartMS
	if n <= 0 {
		return 0
	}
	return int((s.DeepMS*100 + n/2) / n)
}

// SessionStats renders "deep 84% · 1 interruption · longest 9m"; the
// longest stretch shows from one minute.
func SessionStats(s focus.Session, nowMS int64) string {
	parts := []string{
		fmt.Sprintf("deep %d%%", DeepPct(s, nowMS)),
		fmt.Sprintf("%d %s", s.Interruptions, plural(s.Interruptions, "interruption", "interruptions")),
	}
	if s.LongestMS >= 60_000 {
		parts = append(parts, "longest "+Dur(s.LongestMS))
	}
	return strings.Join(parts, " · ")
}

// SessionStatus renders a running session:
// "12:04 left of 25m · deep 84% · 1 interruption".
func SessionStatus(s focus.Session, nowMS int64) string {
	return Countdown(s.PlannedMS-nowMS) + " left of " + Dur(s.PlannedMS-s.StartMS) + " · " + SessionStats(s, nowMS)
}

// SessionLine renders one closed session for the history:
// "09:12  manual   25m  deep  84%  2 int". withDay prefixes the weekday.
// Auto sessions have no interruption count.
func SessionLine(s focus.Session, loc *time.Location, withDay bool) string {
	layout := "15:04"
	if withDay {
		layout = "Mon 15:04"
	}
	line := fmt.Sprintf("%s  %-6s  %4s  deep %3d%%", time.UnixMilli(s.StartMS).In(loc).Format(layout),
		s.Kind, Dur(s.EndMS-s.StartMS), DeepPct(s, s.EndMS))
	if s.Kind == focus.KindManual {
		line += fmt.Sprintf("  %d int", s.Interruptions)
	}
	return line
}

// Recap is the daily recap shared by the notification and tock recap:
// "2h14 focus, score 74 (target 4h00). Top project: tock."
func Recap(d Day) string {
	switch {
	case d.Off && d.TrackedMS > 0:
		return "Day off: " + Dur(d.TrackedMS) + " tracked, not scored."
	case d.Stats.ActiveMS <= 0 && d.Work.Set && d.TrackedMS > 0:
		return "No activity in work hours."
	case d.Stats.ActiveMS <= 0:
		return "No activity."
	}
	focusText := "No focus time"
	if d.Stats.FocusMS >= 60_000 {
		focusText = Dur(d.Stats.FocusMS) + " focus"
	}
	return fmt.Sprintf("%s, score %d (target %s). Top project: %s.",
		focusText, d.Score.Value, Dur(d.Score.Target.MS), topProject(d))
}

// topProject is the project with the most time across categories, or
// "none"; ties go to the first name.
func topProject(d Day) string {
	totals := map[string]int64{}
	for _, c := range d.Categories {
		for _, p := range c.Projects {
			if p.Name != "" {
				totals[p.Name] += p.MS
			}
		}
	}
	names := make([]string, 0, len(totals))
	for n := range totals {
		names = append(names, n)
	}
	sort.Strings(names)
	top, best := "none", int64(0)
	for _, n := range names {
		if totals[n] > best {
			top, best = n, totals[n]
		}
	}
	return top
}
