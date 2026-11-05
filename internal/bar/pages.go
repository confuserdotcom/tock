package bar

import (
	"fmt"
	"math"
	"strconv"
	"time"

	"tock/internal/app"
	"tock/internal/report"
)

// Page is one page of the open quiet notch: you swipe between them with two
// fingers. The panel has three sizes. The small one shows the page itself,
// the medium one adds Mid under it and the large one adds Big beside it.
type Page struct {
	ID    string      `json:"id"` // "session", "today", "week" or "goals"
	Title string      `json:"title"`
	Lead  string      `json:"lead,omitempty"`
	Note  string      `json:"note,omitempty"` // an empty state, in place of the figures
	Cells []Cell      `json:"cells,omitempty"`
	Strip []string    `json:"strip,omitempty"`
	Axis  []string    `json:"axis,omitempty"`
	Bars  []BarCol    `json:"bars,omitempty"`
	Rings []Ring      `json:"rings,omitempty"`
	Rows  [][2]string `json:"rows,omitempty"`
	Mid   *Page       `json:"mid,omitempty"`
	Big   *Page       `json:"big,omitempty"`
}

// BarCol is one column of a stacked bar chart, bottom part first.
type BarCol struct {
	Label string `json:"label"`
	Sub   string `json:"sub,omitempty"` // under the label, e.g. the day's score
	Hi    bool   `json:"hi,omitempty"`  // today or now
	Parts []Part `json:"parts,omitempty"`
}

// Part is one kind's time in a bar.
type Part struct {
	Kind string `json:"kind"` // "none" for unlabelled time
	MS   int64  `json:"ms"`
}

// Ring is a progress ring: a goal, or the time a session has run.
type Ring struct {
	Label string  `json:"label"`
	Value string  `json:"value"`
	Sub   string  `json:"sub,omitempty"`
	Frac  float64 `json:"frac"` // 0..1
	Kind  string  `json:"kind"`
}

// maxRings is how many goals the Goals page shows.
const maxRings = 4

// kindOrder is the order of the kinds in bars and totals; "" is unlabelled.
var kindOrder = []string{"deep", "shallow", "browse", "neutral", "distract", ""}

var kindNames = map[string]string{
	"deep": "Deep", "shallow": "Shallow", "browse": "Browsing",
	"neutral": "Neutral", "distract": "Distracting", "": "No label",
}

func parts(by map[string]int64) []Part {
	var out []Part
	for _, k := range kindOrder {
		if ms := by[k]; ms > 0 {
			if k == "" {
				k = "none"
			}
			out = append(out, Part{Kind: k, MS: ms})
		}
	}
	return out
}

// TodayPages are the pages of the open quiet notch at nowMS: the running
// session (when there is one), today d against the usual day u, the week w
// and the goals. Pure: the caller reads d, w and u.
func TodayPages(s Snapshot, d report.Day, w report.Week, u app.Usual, nowMS int64) []Page {
	var out []Page
	if ss := s.Session; ss != nil {
		out = append(out, s.sessionPage(nowMS))
	}
	return append(out, todayPage(d, u, s.loc()), weekPage(w, nowMS), goalsPage(d))
}

func (s Snapshot) sessionPage(nowMS int64) Page {
	ss := s.Session
	p := Page{ID: "session", Title: "Session", Lead: ss.Goal}
	if p.Lead == "" {
		p.Lead = "Focus session"
	}
	kind := LevelKind(s.Live.Level)
	if ss.PlannedMS > ss.StartMS {
		frac := math.Min(float64(nowMS-ss.StartMS)/float64(ss.PlannedMS-ss.StartMS), 1)
		p.Rings = []Ring{{Label: "Time", Value: report.Dur(max(ss.PlannedMS-nowMS, 0)) + " left", Frac: math.Max(frac, 0), Kind: kind}}
	} else {
		p.Rings = []Ring{{Label: "Time", Value: report.Dur(nowMS-ss.StartMS) + " so far", Frac: 1, Kind: kind}}
	}
	if s.Live.Level >= 0 {
		p.Cells = append(p.Cells, Cell{K: "Focus", V: strconv.Itoa(s.Live.Level) + "%"})
	}
	p.Cells = append(p.Cells, Cell{K: "Started", V: clock(ss.StartMS, s.loc())})
	if ss.PlannedMS > 0 {
		p.Cells = append(p.Cells, Cell{K: "Ends", V: clock(ss.PlannedMS, s.loc())})
	}
	return p
}

func todayPage(d report.Day, u app.Usual, loc *time.Location) Page {
	p := Page{ID: "today", Title: "Today"}
	if d.TrackedMS == 0 {
		p.Note = "Nothing tracked yet today."
		return p
	}
	has := u.Days > 0
	score := Cell{K: "Score", V: strconv.Itoa(d.Score.Value)}
	if has && d.Score.Value != u.Score {
		score.C, score.Tone = fmt.Sprintf("%+d", d.Score.Value-u.Score), "good"
		if d.Score.Value < u.Score {
			score.C, score.Tone = "−"+strconv.Itoa(u.Score-d.Score.Value), "bad"
		}
	}
	deep := Cell{K: "Deep focus", V: report.Dur(d.Stats.FocusMS)}
	deep.C, deep.Tone = change(d.Stats.FocusMS, u.FocusMS, has, false)
	p.Cells = []Cell{score, deep, {K: "Tracked", V: report.Dur(d.TrackedMS)}}
	p.Strip, p.Axis = dayStrip(d, loc)
	if d.Hours.Last >= d.Hours.First && d.Hours.MS != nil {
		mid := &Page{}
		for h := d.Hours.First; h <= d.Hours.Last; h++ {
			by := map[string]int64{}
			for k, ms := range d.Hours.MS {
				by[k] = ms[h]
			}
			mid.Bars = append(mid.Bars, BarCol{Label: fmt.Sprintf("%02d", h), Parts: parts(by)})
		}
		p.Mid = mid
	}
	if len(d.Projects) > 0 {
		big := &Page{Title: "Projects"}
		for i, sh := range d.Projects {
			if i == recapProjects {
				break
			}
			name := sh.Name
			if name == "" {
				name = "No project"
			}
			big.Rows = append(big.Rows, [2]string{report.Cut(name, goalCells), report.Dur(sh.MS)})
		}
		p.Big = big
	}
	return p
}

func weekPage(w report.Week, nowMS int64) Page {
	p := Page{ID: "week", Title: "This week"}
	var scored, sum int
	for _, day := range w.Days {
		b := BarCol{Label: day.Date.Format("Mon")}
		if !day.Future {
			b.Parts = parts(day.ByKind)
			if day.TrackedMS > 0 {
				b.Sub = strconv.Itoa(day.Score.Value)
				scored++
				sum += day.Score.Value
			}
		}
		next := day.Date.AddDate(0, 0, 1).UnixMilli()
		b.Hi = day.Date.UnixMilli() <= nowMS && nowMS < next
		p.Bars = append(p.Bars, b)
	}
	mid := &Page{Cells: []Cell{{K: "Tracked", V: report.Dur(w.TrackedMS)}, {K: "Deep", V: report.Dur(w.ByKind["deep"])}}}
	if scored > 0 {
		mid.Cells = append(mid.Cells, Cell{K: "Avg score", V: strconv.Itoa(int(math.Round(float64(sum) / float64(scored))))})
	}
	p.Mid = mid
	big := &Page{Title: "By kind"}
	for _, k := range kindOrder {
		if ms := w.ByKind[k]; ms > 0 {
			big.Rows = append(big.Rows, [2]string{kindNames[k], report.Dur(ms)})
		}
	}
	if len(big.Rows) > 0 {
		p.Big = big
	}
	return p
}

func goalsPage(d report.Day) Page {
	p := Page{ID: "goals", Title: "Goals"}
	for _, g := range d.Goals {
		if len(p.Rings) == maxRings {
			break
		}
		r := Ring{Label: report.Cut(g.Goal.Name(), goalCells),
			Value: report.Dur(g.DoneMS) + " / " + report.Dur(g.Goal.MS), Kind: "deep"}
		if g.Goal.MS > 0 {
			r.Frac = math.Min(float64(g.DoneMS)/float64(g.Goal.MS), 1)
		}
		switch g.State {
		case "short":
			r.Kind = "shallow"
		case "over":
			r.Kind = "distract"
		}
		if g.Streak >= 2 {
			per := "day"
			if g.Goal.Week {
				per = "week"
			}
			r.Sub = fmt.Sprintf("%d %s streak", g.Streak, per)
		}
		p.Rings = append(p.Rings, r)
	}
	if len(p.Rings) == 0 {
		p.Note = "No goals yet · tock goal add"
	}
	return p
}
