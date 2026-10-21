package tui

import (
	"fmt"
	"strings"

	"charm.land/lipgloss/v2"

	"tock/internal/report"
)

// projectStyle is the colour of the i-th project in the strip; the time
// with no project is faint.
func projectStyle(i int, name string) lipgloss.Style {
	if name == "" {
		return sFaint
	}
	styles := []lipgloss.Style{sBrand, sDeep, sShallow, sUnl, sBrowse, sGood, sAccent}
	return styles[i%len(styles)]
}

// shareName is a share's name in the strip.
func shareName(s report.Share) string {
	if s.Name == "" {
		return "no project"
	}
	return report.Safe(s.Name)
}

// projectFilter is the timeline filter that shows a project's rows.
func projectFilter(name string) string { return "/ " + name }

// projectStrip is each project's share of the day as one bar w wide, a run
// of cells per project in its own colour, and under it the legend
// "tock 42% · thesis 31% · no project 27%". Every run and name is a button
// that filters the timeline to the project. Nil without project time.
func projectStrip(d report.Day, w int) []string {
	if len(d.Projects) == 0 || w < 20 {
		return nil
	}
	n := w - 2
	cells := shareCells(d.Projects, n)
	var bar strings.Builder
	var legend []string
	for i, s := range d.Projects {
		st := projectStyle(i, s.Name)
		text := fmt.Sprintf("%s %s %d%%", shareName(s), report.Dur(s.MS), s.Pct)
		run := st.Render(strings.Repeat("█", cells[i]))
		item := st.Render("■ ") + sText.Render(shareName(s)) + " " + sDim.Render(fmt.Sprintf("%d%%", s.Pct))
		if s.Name != "" {
			f := projectFilter(s.Name)
			run = tip("cat:"+f, text+" · show only its rows", run)
			item = tip("cat:"+f, text+" · show only its rows", item)
		}
		bar.WriteString(run)
		legend = append(legend, item)
	}
	line := " "
	for i, it := range legend {
		sep := ""
		if i > 0 {
			sep = sFaint.Render(" · ")
		}
		if lipgloss.Width(line+sep+it) > w {
			line += sFaint.Render(" …")
			break
		}
		line += sep + it
	}
	return []string{" " + bar.String(), line}
}

// shareCells splits n cells by the shares' time, largest remainder first,
// so the runs fill the bar exactly.
func shareCells(ss []report.Share, n int) []int {
	var total int64
	for _, s := range ss {
		total += s.MS
	}
	out := make([]int, len(ss))
	if total <= 0 {
		return out
	}
	left := n
	rem := make([]int64, len(ss))
	for i, s := range ss {
		out[i] = int(s.MS * int64(n) / total)
		rem[i] = s.MS * int64(n) % total
		left -= out[i]
	}
	for ; left > 0; left-- {
		best := 0
		for i := range rem {
			if rem[i] > rem[best] {
				best = i
			}
		}
		out[best]++
		rem[best] = -1
	}
	return out
}

// sharePct is a project's share of the day as "42%", "" when it has none.
func sharePct(ss []report.Share, name string) string {
	for _, s := range ss {
		if s.Name == name {
			return fmt.Sprintf("%d%%", s.Pct)
		}
	}
	return ""
}
