package report

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"tock/internal/classify"
	"tock/internal/span"
)

// Top section names, also the values of tock top --by.
const (
	TopCategory = "category"
	TopProject  = "project"
	TopApp      = "app"
	TopSite     = "site"
)

// TopSections is the order tock top prints sections in.
var TopSections = []string{TopCategory, TopApp, TopSite, TopProject}

// Unlabelled names time with no category (or project) in a top list.
const Unlabelled = "(unlabelled)"

// TopRow is one ranked name. Kind is the category kind for category rows;
// Mostly is the category with most of an app's or site's time.
type TopRow struct {
	Name   string
	MS     int64
	Kind   string
	Mostly string
}

// Top ranks tracked time over a range by category, project, app and site.
type Top struct {
	From, To time.Time // To is exclusive
	TotalMS  int64
	Sections map[string][]TopRow
}

// BuildTop ranks the non-idle views; labels[i] labels views[i]. Projects
// leave out time without one; sites are URL hosts without "www.".
func BuildTop(from, to time.Time, views []classify.SpanView, labels []classify.Label) Top {
	t := Top{From: from, To: to, Sections: map[string][]TopRow{}}
	ms := map[string]map[string]int64{}
	cats := map[string]map[string]map[string]int64{TopApp: {}, TopSite: {}} // section → name → category → ms
	kinds := map[string]string{}
	for _, s := range TopSections {
		ms[s] = map[string]int64{}
	}
	for i, v := range views {
		d := v.DurMS()
		if v.BundleID == span.IdleBundleID || d <= 0 {
			continue
		}
		l := labels[i]
		t.TotalMS += d
		cat := l.Category
		if cat == "" {
			cat = Unlabelled
		}
		kinds[cat] = l.Kind
		ms[TopCategory][cat] += d
		if l.Project != "" {
			ms[TopProject][l.Project] += d
		}
		names := map[string]string{TopApp: v.App}
		if !v.Private && v.URL != nil && *v.URL != "" {
			h, _, _ := strings.Cut(strings.TrimPrefix(*v.URL, "www."), "/")
			names[TopSite] = strings.ToLower(h)
		}
		for s, n := range names {
			ms[s][n] += d
			if cats[s][n] == nil {
				cats[s][n] = map[string]int64{}
			}
			cats[s][n][cat] += d
		}
	}
	for _, s := range TopSections {
		rows := make([]TopRow, 0, len(ms[s]))
		for n, d := range ms[s] {
			r := TopRow{Name: n, MS: d}
			if s == TopCategory {
				r.Kind = kinds[n]
			}
			if c := cats[s][n]; c != nil {
				r.Mostly = mostly(c)
			}
			rows = append(rows, r)
		}
		sort.Slice(rows, func(i, j int) bool {
			if rows[i].MS != rows[j].MS {
				return rows[i].MS > rows[j].MS
			}
			return rows[i].Name < rows[j].Name
		})
		t.Sections[s] = rows
	}
	return t
}

// mostly is the category with the most time; ties go alphabetically.
func mostly(c map[string]int64) string {
	best, name := int64(-1), ""
	for n, d := range c {
		if d > best || (d == best && n < name) {
			best, name = d, n
		}
	}
	return name
}

// topTitles are the section headings.
var topTitles = map[string]string{
	TopCategory: "Categories", TopProject: "Projects", TopApp: "Apps", TopSite: "Sites",
}

// TopLines renders the given sections, at most limit rows each, with a bar,
// the time and the share of the range's tracked time.
func TopLines(t Top, sections []string, limit int) []string {
	last := t.To.AddDate(0, 0, -1)
	out := []string{fmt.Sprintf("Top  %s – %s · %s tracked", t.From.Format("Mon 2 Jan"), last.Format("Mon 2 Jan"), Dur(t.TotalMS))}
	if t.TotalMS == 0 {
		return append(out, "nothing tracked in this range")
	}
	for _, s := range sections {
		rows := t.Sections[s]
		out = append(out, "", topTitles[s])
		if len(rows) == 0 {
			out = append(out, "  none")
			continue
		}
		var top int64
		nameW := 0
		shown := rows
		if limit > 0 && len(shown) > limit {
			shown = shown[:limit]
		}
		for _, r := range shown {
			top = max(top, r.MS)
			nameW = max(nameW, len([]rune(Safe(r.Name))))
		}
		nameW = min(nameW, 32)
		for _, r := range shown {
			name := []rune(Safe(r.Name))
			if len(name) > nameW {
				name = append(name[:nameW-1], '…')
			}
			line := fmt.Sprintf("  %s  %5s %3d%%  %s", Pad(string(name), nameW), Dur(r.MS), pct(r.MS, t.TotalMS), Pad(Bar(r.MS, top, 20), 20))
			switch {
			case r.Kind != "":
				line += "  " + r.Kind
			case r.Mostly != "" && s != TopCategory:
				line += "  mostly " + Safe(r.Mostly)
			}
			out = append(out, strings.TrimRight(line, " "))
		}
		if n := len(rows) - len(shown); n > 0 {
			var rest int64
			for _, r := range rows[len(shown):] {
				rest += r.MS
			}
			out = append(out, fmt.Sprintf("  … %d more, %s", n, Dur(rest)))
		}
	}
	return out
}

func pct(ms, total int64) int {
	if total <= 0 {
		return 0
	}
	return int((ms*100 + total/2) / total)
}

