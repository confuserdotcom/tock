package report

import (
	"fmt"
	"math"
	"sort"
	"strings"
	"time"

	"tock/internal/classify"
	"tock/internal/span"
	"tock/internal/stats"
)

// MonthDay is one day's focus numbers.
type MonthDay struct {
	Stats stats.Day
	Score stats.Score
}

// MonthInput is a month (up to Through) and the previous month's comparison
// window (its days 1 to PrevThrough).
type MonthInput struct {
	First                time.Time // local midnight of the month's first day
	Through, PrevThrough int       // last day of each window, 1-based
	Partial              bool      // the month is still going on
	Views                []classify.SpanView
	Labels               []classify.Label
	PrevViews            []classify.SpanView
	PrevLabels           []classify.Label
	Days, PrevDays       []MonthDay // Days[i] is day i+1
}

// TrendRow is one category's time in both windows.
type TrendRow struct {
	Name, Kind string
	MS, PrevMS int64
}

// MonthSide is one window's totals.
type MonthSide struct {
	TrackedMS, FocusMS int64
	Workdays           int
	AvgScore           float64 // over workdays; 0 with none
	SwitchesPerHour    float64 // workday switches per workday active hour
}

// Month is the trends view.
type Month struct {
	First                time.Time
	Through, PrevThrough int
	Partial              bool
	Cur, Prev            MonthSide
	Categories           []TrendRow // by MS desc, then PrevMS desc, then name; Unclassified last
	Scores               []int      // one per day of the window: the score, or -1 for a day that is not a workday
}

// BuildMonth computes the trends.
func BuildMonth(in MonthInput) Month {
	m := Month{First: in.First, Through: in.Through, PrevThrough: in.PrevThrough, Partial: in.Partial}
	var scores []int
	m.Cur, scores = side(in.Views, in.Days)
	m.Prev, _ = side(in.PrevViews, in.PrevDays)
	m.Scores = scores
	rows := map[string]*TrendRow{}
	add := func(vs []classify.SpanView, ls []classify.Label, prev bool) {
		for i, v := range vs {
			if v.BundleID == span.IdleBundleID {
				continue
			}
			name := ls[i].Category
			if name == "" {
				name = Unclassified
			}
			r := rows[name]
			if r == nil {
				r = &TrendRow{Name: name, Kind: ls[i].Kind}
				rows[name] = r
			}
			if prev {
				r.PrevMS += v.DurMS()
			} else {
				r.MS += v.DurMS()
			}
		}
	}
	add(in.Views, in.Labels, false)
	add(in.PrevViews, in.PrevLabels, true)
	for _, r := range rows {
		m.Categories = append(m.Categories, *r)
	}
	c := m.Categories
	sort.Slice(c, func(a, b int) bool {
		switch {
		case (c[a].Name == Unclassified) != (c[b].Name == Unclassified):
			return c[b].Name == Unclassified
		case c[a].MS != c[b].MS:
			return c[a].MS > c[b].MS
		case c[a].PrevMS != c[b].PrevMS:
			return c[a].PrevMS > c[b].PrevMS
		}
		return c[a].Name < c[b].Name
	})
	return m
}

func side(vs []classify.SpanView, days []MonthDay) (MonthSide, []int) {
	var s MonthSide
	for _, v := range vs {
		if v.BundleID != span.IdleBundleID {
			s.TrackedMS += v.DurMS()
		}
	}
	scores := make([]int, len(days))
	var sum float64
	var switches int
	var active int64
	for i, d := range days {
		s.FocusMS += d.Stats.FocusMS
		scores[i] = -1
		if d.Stats.ActiveMS < stats.WorkdayMinMS {
			continue
		}
		scores[i] = d.Score.Value
		s.Workdays++
		sum += float64(d.Score.Value)
		switches += d.Stats.Switches
		active += d.Stats.ActiveMS
	}
	if s.Workdays > 0 {
		s.AvgScore = sum / float64(s.Workdays)
		s.SwitchesPerHour = float64(switches) / (float64(active) / 3_600_000)
	}
	return s, scores
}

// Spark renders one cell per score: ▁ (0) to █ (100), · for -1.
func Spark(scores []int) string {
	var b strings.Builder
	for _, s := range scores {
		if s < 0 {
			b.WriteRune('·')
		} else {
			b.WriteRune(SparkCell(s))
		}
	}
	return b.String()
}

// SparkCell is the sparkline cell of a score: ▁ for 0-12 up to █ for 88-100.
func SparkCell(score int) rune {
	return []rune("▁▂▃▄▅▆▇█")[min(max(score, 0)*8/100, 7)]
}

// Delta renders a signed difference: "+4h20", "−1h10" (U+2212) or "±0".
func Delta(ms int64) string {
	switch {
	case ms > 0:
		return "+" + Dur(ms)
	case ms < 0:
		return "−" + Dur(-ms)
	}
	return "±0"
}

// change is a row's delta and its note: a percentage when the previous
// value is at least a minute, else "new"; "gone" when the time went to 0.
func change(cur, prev int64) string {
	switch {
	case prev < 60_000 && cur > 0:
		return "new"
	case cur == 0 && prev > 0:
		return fmt.Sprintf("%6s  gone", Delta(-prev))
	case prev < 60_000:
		return ""
	}
	pct := int(math.Round(float64(cur-prev) * 100 / float64(prev)))
	sign := "+"
	if pct < 0 {
		sign, pct = "−", -pct
	} else if pct == 0 {
		sign = "±"
	}
	return fmt.Sprintf("%6s  %s%d%%", Delta(cur-prev), sign, pct)
}

// MonthTitle is "October 2026 · 1–10 vs September 1–10" for a month in
// progress, "September 2026 vs August" for a past one.
func MonthTitle(m Month) string {
	prev := m.First.AddDate(0, -1, 0).Format("January")
	if !m.Partial {
		return m.First.Format("January 2006") + " vs " + prev
	}
	days := func(n int) string {
		if n == 1 {
			return "1"
		}
		return fmt.Sprintf("1–%d", n)
	}
	return fmt.Sprintf("%s · %s vs %s %s", m.First.Format("January 2006"), days(m.Through), prev, days(m.PrevThrough))
}

// MonthLines renders the trends, shared by tock trends and the TUI.
func MonthLines(m Month) []string {
	avg := func(s MonthSide) string {
		if s.Workdays == 0 {
			return "—"
		}
		return fmt.Sprint(int(math.Round(s.AvgScore)))
	}
	perHour := func(s MonthSide) string {
		if s.Workdays == 0 {
			return "—"
		}
		return fmt.Sprintf("%.1f/h", s.SwitchesPerHour)
	}
	score := "—"
	if m.Cur.Workdays > 0 {
		score = fmt.Sprintf("%s avg · %d %s", avg(m.Cur), m.Cur.Workdays, plural(m.Cur.Workdays, "workday", "workdays"))
	}
	out := []string{
		MonthTitle(m),
		strings.TrimRight(fmt.Sprintf("%-9s %6s  %s", "tracked", Dur(m.Cur.TrackedMS), change(m.Cur.TrackedMS, m.Prev.TrackedMS)), " "),
		strings.TrimRight(fmt.Sprintf("%-9s %6s  %s", "focus", Dur(m.Cur.FocusMS), change(m.Cur.FocusMS, m.Prev.FocusMS)), " "),
		fmt.Sprintf("%-9s %s   vs %s   %s", "score", score, avg(m.Prev), Spark(m.Scores)),
		fmt.Sprintf("%-9s %s   vs %s", "switches", perHour(m.Cur), perHour(m.Prev)),
		"",
	}
	for _, c := range m.Categories {
		out = append(out, strings.TrimRight(fmt.Sprintf("%s %6s  %s", Pad(c.Name, 16), Dur(c.MS), change(c.MS, c.PrevMS)), " "))
	}
	if len(m.Categories) == 0 {
		out = append(out, "no activity")
	}
	return out
}
