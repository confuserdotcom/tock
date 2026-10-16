// Package ask turns a question about tracked time into a query and runs it
// on local data. Claude only ever sees the question, today's date and the
// category and project names; the answer is computed here. It is pure.
package ask

import (
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"tock/internal/classify"
	"tock/internal/report"
	"tock/internal/span"
)

// MaxQuestion bounds the question, in runes.
const MaxQuestion = 500

// Ranges.
const (
	Today     = "today"
	Yesterday = "yesterday"
	ThisWeek  = "this_week"
	LastWeek  = "last_week"
	ThisMonth = "this_month"
	LastMonth = "last_month"
	LastDays  = "last_days"
	Dates     = "dates"
)

// Groupings.
const (
	ByTotal    = "total"
	ByCategory = "category"
	ByProject  = "project"
	ByApp      = "app"
	BySite     = "site"
	ByKind     = "kind"
	ByDay      = "day"
)

// Query is what Claude made of the question.
type Query struct {
	Range       string  `json:"range"`
	Days        int     `json:"days,omitempty"`
	From        string  `json:"from,omitempty"`
	To          string  `json:"to,omitempty"`
	GroupBy     string  `json:"group_by"`
	Category    *string `json:"category,omitempty"`
	Project     *string `json:"project,omitempty"`
	App         *string `json:"app,omitempty"`
	Site        *string `json:"site,omitempty"`
	Kind        *string `json:"kind,omitempty"`
	Limit       int     `json:"limit,omitempty"`
	Unsupported *string `json:"unsupported,omitempty"`
}

type category struct {
	Name string `json:"name"`
	Kind string `json:"kind"`
}

type payload struct {
	Today      string     `json:"today"`
	Categories []category `json:"categories"`
	Projects   []string   `json:"projects"`
	Question   string     `json:"question"`
}

// Payload is the exact message sent for question. It fails on an empty or
// too long question.
func Payload(question string, today time.Time, rs *classify.Ruleset) ([]byte, error) {
	q := strings.Join(strings.Fields(question), " ")
	switch n := len([]rune(q)); {
	case n == 0:
		return nil, errors.New("ask: the question is empty")
	case n > MaxQuestion:
		return nil, fmt.Errorf("ask: the question is longer than %d characters", MaxQuestion)
	}
	p := payload{Today: today.Format("2006-01-02 Monday"), Categories: []category{}, Projects: []string{}, Question: report.Safe(q)}
	for _, c := range rs.Categories {
		p.Categories = append(p.Categories, category{c.Name, c.Kind})
	}
	for _, pr := range rs.Projects {
		if !pr.Archived {
			p.Projects = append(p.Projects, pr.Name)
		}
	}
	b, err := json.MarshalIndent(p, "", "  ")
	if err != nil {
		panic(err)
	}
	return b, nil
}

// Parse reads the query tool input and checks it against the rules. An
// empty filter (null or "") is no filter.
func Parse(raw json.RawMessage, rs *classify.Ruleset) (Query, error) {
	var q Query
	if err := json.Unmarshal(raw, &q); err != nil {
		return q, fmt.Errorf("ask: bad answer: %w", err)
	}
	if q.Unsupported != nil && strings.TrimSpace(*q.Unsupported) != "" {
		return q, fmt.Errorf("tock cannot answer that: %s", report.Safe(*q.Unsupported))
	}
	for _, p := range []**string{&q.Category, &q.Project, &q.App, &q.Site, &q.Kind} {
		if *p != nil && strings.TrimSpace(**p) == "" {
			*p = nil
		}
	}
	switch q.Range {
	case Today, Yesterday, ThisWeek, LastWeek, ThisMonth, LastMonth:
	case LastDays:
		if q.Days < 1 || q.Days > 366 {
			return q, fmt.Errorf("ask: %d days, want 1 to 366", q.Days)
		}
	case Dates:
		if _, err := time.Parse("2006-01-02", q.From); err != nil {
			return q, errors.New("ask: bad from date")
		}
		if _, err := time.Parse("2006-01-02", q.To); err != nil || q.To < q.From {
			return q, errors.New("ask: bad to date")
		}
	default:
		return q, fmt.Errorf("ask: unknown range %q", q.Range)
	}
	switch q.GroupBy {
	case ByTotal, ByCategory, ByProject, ByApp, BySite, ByKind, ByDay:
	default:
		return q, fmt.Errorf("ask: unknown grouping %q", q.GroupBy)
	}
	if q.Category != nil && !rs.HasCategory(*q.Category) {
		return q, fmt.Errorf("ask: no category %q", *q.Category)
	}
	if q.Project != nil && !rs.HasProject(*q.Project) {
		return q, fmt.Errorf("ask: no project %q", *q.Project)
	}
	if q.Kind != nil {
		switch *q.Kind {
		case classify.KindDeep, classify.KindShallow, classify.KindDistract, classify.KindNeutral, classify.KindBrowse:
		default:
			return q, fmt.Errorf("ask: unknown kind %q", *q.Kind)
		}
	}
	if q.Limit <= 0 || q.Limit > 50 {
		q.Limit = 10
	}
	return q, nil
}

// Bounds is the query's range: from today's midnight (in today's zone),
// [from, to). Weeks start on Monday.
func (q Query) Bounds(today time.Time) (time.Time, time.Time) {
	day := time.Date(today.Year(), today.Month(), today.Day(), 0, 0, 0, 0, today.Location())
	mon := report.MondayOf(day)
	first := time.Date(day.Year(), day.Month(), 1, 0, 0, 0, 0, day.Location())
	switch q.Range {
	case Today:
		return day, day.AddDate(0, 0, 1)
	case Yesterday:
		return day.AddDate(0, 0, -1), day
	case ThisWeek:
		return mon, mon.AddDate(0, 0, 7)
	case LastWeek:
		return mon.AddDate(0, 0, -7), mon
	case ThisMonth:
		return first, first.AddDate(0, 1, 0)
	case LastMonth:
		return first.AddDate(0, -1, 0), first
	case LastDays:
		return day.AddDate(0, 0, 1-q.Days), day.AddDate(0, 0, 1)
	}
	f, _ := time.ParseInLocation("2006-01-02", q.From, day.Location())
	t, _ := time.ParseInLocation("2006-01-02", q.To, day.Location())
	return f, t.AddDate(0, 0, 1)
}

// Row is one group of the answer.
type Row struct {
	Name string
	MS   int64
}

// Result is the answer: the matching time, the range's tracked time and the
// groups, largest first (by day: in date order).
type Result struct {
	From, To  time.Time
	MS        int64
	TrackedMS int64
	Rows      []Row
}

// host is a view's URL host without "www.", lowercase.
func host(v classify.SpanView) string {
	if v.Private || v.URL == nil {
		return ""
	}
	h, _, _ := strings.Cut(strings.TrimPrefix(strings.ToLower(*v.URL), "www."), "/")
	h, _, _ = strings.Cut(h, ":")
	return h
}

// match reports whether a view passes q's filters. App names match without
// case and on a prefix; sites match the host or a parent domain.
func (q Query) match(v classify.SpanView, l classify.Label) bool {
	eq := func(a, b string) bool { return strings.EqualFold(a, b) }
	if q.Category != nil && !eq(l.Category, *q.Category) {
		return false
	}
	if q.Project != nil && !eq(l.Project, *q.Project) {
		return false
	}
	if q.Kind != nil && l.Kind != *q.Kind {
		return false
	}
	if q.App != nil && !strings.HasPrefix(strings.ToLower(v.App), strings.ToLower(strings.TrimSpace(*q.App))) {
		return false
	}
	if q.Site != nil {
		s := strings.TrimPrefix(strings.ToLower(strings.TrimSpace(*q.Site)), "www.")
		h := host(v)
		if h == "" || (h != s && !strings.HasSuffix(h, "."+s)) {
			return false
		}
	}
	return true
}

// Run answers q over views clipped to the range; labels[i] labels views[i].
func Run(q Query, from, to time.Time, views []classify.SpanView, labels []classify.Label) Result {
	r := Result{From: from, To: to}
	groups := map[string]int64{}
	for i, v := range views {
		d := v.DurMS()
		if v.BundleID == span.IdleBundleID || d <= 0 {
			continue
		}
		r.TrackedMS += d
		l := labels[i]
		if !q.match(v, l) {
			continue
		}
		r.MS += d
		var name string
		switch q.GroupBy {
		case ByCategory:
			name = l.Category
		case ByProject:
			name = l.Project
		case ByApp:
			name = v.App
		case BySite:
			name = host(v)
		case ByKind:
			name = l.Kind
		case ByDay:
			name = time.UnixMilli(v.Start).In(from.Location()).Format("2006-01-02 Mon")
		default:
			continue
		}
		if name == "" {
			name = report.Unlabelled
		}
		groups[name] += d
	}
	for n, d := range groups {
		r.Rows = append(r.Rows, Row{n, d})
	}
	sort.Slice(r.Rows, func(i, j int) bool {
		if q.GroupBy == ByDay {
			return r.Rows[i].Name < r.Rows[j].Name
		}
		if r.Rows[i].MS != r.Rows[j].MS {
			return r.Rows[i].MS > r.Rows[j].MS
		}
		return r.Rows[i].Name < r.Rows[j].Name
	})
	if q.GroupBy != ByDay && len(r.Rows) > q.Limit {
		r.Rows = r.Rows[:q.Limit]
	}
	return r
}

// Describe says what was measured: "Entertainment · youtube.com, last week".
func (q Query) Describe() string {
	var parts []string
	for _, p := range []*string{q.Category, q.Project, q.Kind, q.App, q.Site} {
		if p != nil {
			parts = append(parts, report.Safe(*p))
		}
	}
	what := "all tracked time"
	if len(parts) > 0 {
		what = strings.Join(parts, " · ")
	}
	rng := strings.ReplaceAll(q.Range, "_", " ")
	switch q.Range {
	case LastDays:
		rng = fmt.Sprintf("last %d days", q.Days)
	case Dates:
		rng = q.From + " to " + q.To
	}
	return what + ", " + rng
}

// Lines renders the answer.
func Lines(q Query, r Result) []string {
	last := r.To.AddDate(0, 0, -1)
	span := r.From.Format("Mon 2 Jan")
	if !last.Equal(r.From) {
		span += " – " + last.Format("Mon 2 Jan")
	}
	out := []string{fmt.Sprintf("%s: %s", q.Describe(), report.Dur(r.MS))}
	if r.TrackedMS > 0 && r.MS != r.TrackedMS {
		out[0] += fmt.Sprintf(" (%d%% of %s tracked)", (r.MS*100+r.TrackedMS/2)/r.TrackedMS, report.Dur(r.TrackedMS))
	}
	out = append(out, "  "+span)
	if q.GroupBy == ByTotal || len(r.Rows) == 0 {
		return out
	}
	out = append(out, "")
	var top int64
	w := 0
	for _, row := range r.Rows {
		top = max(top, row.MS)
		w = max(w, len([]rune(report.Safe(row.Name))))
	}
	w = min(w, 32)
	for _, row := range r.Rows {
		out = append(out, strings.TrimRight(fmt.Sprintf("  %s  %5s  %s", report.Pad(report.Safe(row.Name), w), report.Dur(row.MS), report.Bar(row.MS, top, 20)), " "))
	}
	return out
}
