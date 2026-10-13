package report

import (
	"embed"
	"html/template"
	"io"
	"strconv"
	"time"
)

//go:embed templates/week.html
var templates embed.FS

var weekTmpl = template.Must(template.New("week.html").Funcs(template.FuncMap{"dur": Dur}).
	ParseFS(templates, "templates/week.html"))

type htmlPart struct {
	Kind string
	Pct  float64
}

type htmlDay struct {
	Name      string
	TrackedMS int64
	FocusMS   int64
	Score     int
	Scored    bool
	Width     float64 // bar width against the longest day, percent
	Parts     []htmlPart
}

type htmlRow struct {
	Name, Kind string
	MS         int64
	Pct        float64
}

type htmlList struct {
	Title string
	Rows  []NameMS
}

type htmlGoal struct{ Goal, Text, State string }

type htmlWeek struct {
	Week                    Week
	Title, Range, Generated string
	FocusMS                 int64
	AvgScore                string
	Scored                  int
	Days                    []htmlDay
	Categories              []htmlRow
	Lists                   []htmlList
	Goals                   []htmlGoal
	Commits                 string
}

// kindClass is the CSS class suffix of a kind ("" is unlabelled).
func kindClass(kind string) string {
	if kind == "" {
		return "none"
	}
	return kind
}

// WriteWeekHTML writes the week as one self-contained HTML page: no
// scripts and nothing loaded from elsewhere, so it can be mailed or kept.
// generated is when the page was made.
func WriteWeekHTML(w io.Writer, wk Week, generated time.Time) error {
	sun := wk.Monday.AddDate(0, 0, 6)
	v := htmlWeek{Week: wk, Title: wk.Monday.Format("2 Jan 2006"),
		Range:     wk.Monday.Format("Mon 2 Jan") + " – " + sun.Format("Sun 2 Jan 2006"),
		Generated: generated.Format("2006-01-02 15:04"), AvgScore: "—", Commits: CommitsLine(wk.Commits)}
	var maxMS int64
	for _, d := range wk.Days {
		maxMS = max(maxMS, d.TrackedMS)
	}
	sum := 0
	for _, d := range wk.Days {
		hd := htmlDay{Name: d.Date.Format("Mon 2"), TrackedMS: d.TrackedMS, FocusMS: d.Stats.FocusMS,
			Score: d.Score.Value, Scored: !d.Future && d.Stats.ActiveMS > 0}
		if hd.Scored {
			v.Scored++
			sum += d.Score.Value
			v.FocusMS += d.Stats.FocusMS
		}
		if d.TrackedMS > 0 && maxMS > 0 {
			hd.Width = pct1(d.TrackedMS, maxMS)
			for _, k := range Kinds {
				if ms := d.ByKind[k]; ms > 0 {
					hd.Parts = append(hd.Parts, htmlPart{kindClass(k), pct1(ms, d.TrackedMS)})
				}
			}
		}
		v.Days = append(v.Days, hd)
	}
	if v.Scored > 0 {
		v.AvgScore = strconv.Itoa((sum + v.Scored/2) / v.Scored)
	}
	var top int64
	for _, c := range wk.Categories {
		top = max(top, c.MS)
	}
	for _, c := range wk.Categories {
		v.Categories = append(v.Categories, htmlRow{Name: Safe(c.Name), Kind: kindClass(c.Kind), MS: c.MS, Pct: pct1(c.MS, top)})
	}
	for _, l := range []htmlList{{"Projects", wk.Projects}, {"Apps and sites", wk.Apps}, {"Distractions", wk.Distractions}} {
		if len(l.Rows) == 0 {
			continue
		}
		rows := make([]NameMS, len(l.Rows))
		for i, r := range l.Rows {
			rows[i] = NameMS{Safe(r.Name), r.MS}
		}
		v.Lists = append(v.Lists, htmlList{l.Title, rows})
	}
	for _, g := range wk.Goals {
		state := g.State
		if !g.Goal.Week {
			state = dayGoalState(g)
		}
		v.Goals = append(v.Goals, htmlGoal{Goal: g.Goal.Text(), Text: WeekGoalText(g), State: state})
	}
	return weekTmpl.Execute(w, v)
}

// dayGoalState is a day goal's state over the week: met when it held every
// active day, over when a limit was passed on any.
func dayGoalState(g WeekGoalRow) string {
	switch {
	case g.Days == 0:
		return ""
	case g.Goal.AtMost && g.Held < g.Days:
		return GoalOver
	case g.Held == g.Days:
		return GoalMet
	}
	return GoalShort
}

func pct1(part, whole int64) float64 {
	if whole <= 0 {
		return 0
	}
	return float64(part*1000/whole) / 10
}
