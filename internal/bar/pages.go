package bar

import (
	"fmt"
	"math"
	"slices"
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
	Dial  *Dial       `json:"dial,omitempty"`
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
	// Big draws the ring large with the value inside: the session's clock.
	Big bool `json:"big,omitempty"`
}

// Dial is a day as a 24-hour ring, midnight at the top: what was done in
// kind colours, the work hours outside it, a hand at now.
type Dial struct {
	Arcs   []Arc       `json:"arcs"`
	Now    float64     `json:"now"`            // share of the day; -1 = not today
	Work   *[2]float64 `json:"work,omitempty"` // the work hours as shares of the day
	Center string      `json:"center"`         // the time now, or the weekday
	Sub    string      `json:"sub"`
}

// Arc is a run of one kind as shares of the day, 0..1 from midnight; kind
// "none" is unlabelled, "away" is nobody there.
type Arc struct {
	From float64 `json:"from"`
	To   float64 `json:"to"`
	Kind string  `json:"kind"`
}

// dialCell is how finely the dial reads the day: each 5-minute cell takes
// the kind it holds most of, so a choppy day reads as its main runs.
const dialCell = 5 * 60_000

// dayDial is day d as a dial at nowMS.
func dayDial(d report.Day, loc *time.Location, nowMS int64) *Dial {
	day0 := d.Date.UnixMilli()
	day1 := d.Date.AddDate(0, 0, 1).UnixMilli()
	span := float64(day1 - day0) // 23 or 25 hours on a clock change
	n := int((day1 - day0 + dialCell - 1) / dialCell)
	cells := make([]map[string]int64, n)
	for _, r := range d.Timeline {
		from, to := max(r.Start, day0), min(r.End, day1)
		if to <= from {
			continue
		}
		kind := r.Label.Kind
		switch {
		case r.Idle:
			kind = "away"
		case kind == "":
			kind = "none"
		}
		for i := int((from - day0) / dialCell); i < n && day0+int64(i)*dialCell < to; i++ {
			lo := day0 + int64(i)*dialCell
			if cells[i] == nil {
				cells[i] = map[string]int64{}
			}
			cells[i][kind] += min(lo+dialCell, to) - max(lo, from)
		}
	}
	out := &Dial{Arcs: []Arc{}, Now: -1, Sub: report.Dur(d.TrackedMS) + " tracked"}
	end := -1 // the cell the last arc ends at
	for i, m := range cells {
		var best string
		var bestMS, total int64
		for _, k := range append(kindOrder[:len(kindOrder)-1:len(kindOrder)-1], "none", "away") { // a fixed order: ties go the same way every time
			total += m[k]
			if m[k] > bestMS {
				best, bestMS = k, m[k]
			}
		}
		if total < dialCell/3 {
			continue
		}
		at := func(c int) float64 { return math.Min(float64(int64(c)*dialCell)/span, 1) }
		if k := len(out.Arcs) - 1; k >= 0 && out.Arcs[k].Kind == best && end == i {
			out.Arcs[k].To = at(i + 1)
		} else {
			out.Arcs = append(out.Arcs, Arc{From: at(i), To: at(i + 1), Kind: best})
		}
		end = i + 1
	}
	if nowMS >= day0 && nowMS < day1 {
		out.Now = float64(nowMS-day0) / span
		out.Center = clock(nowMS, loc)
	} else {
		out.Center = d.Date.Format("Mon")
	}
	if w := d.Work; w.Set && !w.Off(d.Date) {
		out.Work = &[2]float64{float64(w.StartMin) * 60_000 / span, float64(w.EndMin) * 60_000 / span}
	}
	return out
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
	out = append(out, todayPage(d, u, s.loc(), nowMS), weekPage(w, nowMS), goalsPage(d))
	if s.NoFocus {
		for i := range out {
			out[i] = unscored(out[i])
		}
	}
	return out
}

// unscored is p without the focus score, for when Focus is off.
func unscored(p Page) Page {
	var cells []Cell
	for _, c := range p.Cells {
		if c.K != "Score" && c.K != "Avg score" {
			cells = append(cells, c)
		}
	}
	p.Cells = cells
	if p.ID == "week" {
		p.Bars = slices.Clone(p.Bars)
		for i := range p.Bars {
			p.Bars[i].Sub = ""
		}
	}
	if p.Mid != nil {
		m := unscored(*p.Mid)
		p.Mid = &m
	}
	return p
}

// sessionPage is the running session at nowMS, to the minute: the page
// changes once a minute, so the notch is not drawn again every second.
func (s Snapshot) sessionPage(nowMS int64) Page {
	nowMS -= nowMS % 60_000
	ss := s.Session
	p := Page{ID: "session", Title: "Session", Lead: ss.Goal}
	if p.Lead == "" {
		p.Lead = "Focus session"
	}
	kind := LevelKind(s.Live.Level)
	if ss.PlannedMS > ss.StartMS {
		frac := math.Min(float64(nowMS-ss.StartMS)/float64(ss.PlannedMS-ss.StartMS), 1)
		p.Rings = []Ring{{Label: "Time", Value: report.Dur(max(ss.PlannedMS-nowMS, 0)) + " left", Frac: math.Max(frac, 0), Kind: kind, Big: true}}
	} else {
		p.Rings = []Ring{{Label: "Time", Value: report.Dur(nowMS-ss.StartMS) + " so far", Frac: 1, Kind: kind, Big: true}}
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

func todayPage(d report.Day, u app.Usual, loc *time.Location, nowMS int64) Page {
	p := Page{ID: "today", Title: "Today"}
	if d.TrackedMS == 0 {
		p.Note = "Nothing tracked yet today."
		return p
	}
	has := u.Days > 0
	score := Cell{K: "Score", V: strconv.Itoa(d.Score.Value)}
	if d.Off { // nothing is scored on a day off
		score.V = "day off"
	} else if has && d.Score.Value != u.Score {
		score.C, score.Tone = fmt.Sprintf("%+d", d.Score.Value-u.Score), "good"
		if d.Score.Value < u.Score {
			score.C, score.Tone = "−"+strconv.Itoa(u.Score-d.Score.Value), "bad"
		}
	}
	deep := Cell{K: "Deep focus", V: report.Dur(d.Stats.FocusMS)}
	deep.C, deep.Tone = change(d.Stats.FocusMS, u.FocusMS, has, false)
	p.Cells = []Cell{score, deep, {K: "Tracked", V: report.Dur(d.TrackedMS)}}
	p.Dial = dayDial(d, loc, nowMS)
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
	mid := &Page{Cells: []Cell{{K: "Tracked", V: report.Dur(w.TrackedMS)}, {K: "Deep focus", V: report.Dur(w.ByKind["deep"])}}}
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
