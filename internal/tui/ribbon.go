package tui

import (
	"fmt"
	"strings"
	"time"

	"tock/internal/report"
)

// ribbon is the day as one strip of cells, from the first row to the last,
// each cell coloured by the kind that filled most of it: idle is faint, no
// record blank, unlabelled violet. Every cell is a button that selects the
// row at that time, with the row as its tooltip. Under it, the hours.
func ribbon(d report.Day, w int, loc *time.Location) []string {
	rows := d.Timeline
	if len(rows) == 0 || w < 20 {
		return nil
	}
	from, to := rows[0].Start, rows[0].End
	for _, r := range rows {
		from, to = min(from, r.Start), max(to, r.End)
	}
	n := w - 2
	if to-from < int64(n)*1000 {
		to = from + int64(n)*1000
	}
	step := (to - from) / int64(n)
	var b, axis strings.Builder
	lastLabel := -10
	for i := range n {
		c0, c1 := from+int64(i)*step, from+int64(i+1)*step
		kinds := map[string]int64{}
		var idle, have int64
		var tipRow *report.Row
		for j, r := range rows {
			ov := min(r.End, c1) - max(r.Start, c0)
			if ov <= 0 {
				continue
			}
			have += ov
			if r.Idle {
				idle += ov
				continue
			}
			kinds[r.Label.Kind] += ov
			if tipRow == nil || ov > min(tipRow.End, c1)-max(tipRow.Start, c0) {
				tipRow = &rows[j]
			}
		}
		cell := " "
		if have > 0 {
			best, bestMS := "", int64(-1)
			for _, k := range report.Kinds {
				if kinds[k] > bestMS {
					best, bestMS = k, kinds[k]
				}
			}
			switch {
			case idle > bestMS:
				cell = sFaint.Render("·")
			case best == "":
				cell = sUnl.Render("▆")
			default:
				cell = kindStyle(best).Render("█")
			}
		}
		mid := (c0 + c1) / 2
		text := time.UnixMilli(c0).In(loc).Format("15:04")
		if tipRow != nil {
			text = rowTip(*tipRow, loc)
		} else if idle > 0 {
			text += " idle"
		}
		b.WriteString(tip(fmt.Sprintf("at:%d", mid), text, cell))
		t0 := time.UnixMilli(c0).In(loc)
		if h := t0.Hour(); (t0.Minute() == 0 || time.UnixMilli(c1).In(loc).Hour() != h) && i-lastLabel >= 4 && i < n-2 {
			hh := (h + 1) % 24
			if t0.Minute() == 0 {
				hh = h
			}
			label := fmt.Sprintf("%02d", hh)
			for axis.Len() < i {
				axis.WriteByte(' ')
			}
			axis.WriteString(label)
			lastLabel = i
		}
	}
	return []string{" " + b.String(), " " + sFaint.Render(axis.String())}
}

// rowTip is a row in one line: "10:40–11:14  34m  Code / tock · WezTerm".
func rowTip(r report.Row, loc *time.Location) string {
	clock := func(ms int64) string { return time.UnixMilli(ms).In(loc).Format("15:04") }
	label := rowLabel(r)
	switch {
	case r.Idle:
		label = "idle"
	case label == "—":
		label = "unlabelled"
	}
	s := clock(r.Start) + "–" + clock(r.End) + "  " + report.Dur(r.End-r.Start) + "  " + label + " · " + r.App
	if r.Text != "" {
		s += " · " + r.Text
	}
	return s
}
