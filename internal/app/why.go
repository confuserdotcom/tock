package app

import (
	"fmt"
	"math"
	"strings"

	"tock/internal/classify"
)

// WhyLine says what set one field of a label: Field is "category" or
// "project", Text is the reason in words ("rules.toml, rule 12: site ~
// github\.com/acme", "your tag", "local model, 87 %: words …"), and Ref
// is the rule's ref when a rule set it, so the page can open that rule.
type WhyLine struct {
	Field, Label, Source, Text, Ref string
}

// Why explains the label of span id, which lies in [from, to): one line
// for the category and one for the project, each when set. An id not in
// the range has no lines.
func (a *App) Why(id, from, to int64) ([]WhyLine, error) {
	vs, ls, err := a.labelled(from, to, a.nowMS())
	if err != nil {
		return nil, err
	}
	for i := range vs {
		if vs[i].ID != id {
			continue
		}
		v, l := vs[i], ls[i]
		var out []WhyLine
		if l.Category != "" {
			text := whyText(a.rs, l.CategorySource, l.CategoryRule)
			if l.CategorySource == classify.SourceModel || l.CategorySource == classify.SourceScreen {
				text = a.modelWhy(v, l)
			}
			out = append(out, WhyLine{"category", l.Category, string(l.CategorySource), text, l.CategoryRule})
		}
		if l.Project != "" {
			out = append(out, WhyLine{"project", l.Project, string(l.ProjectSource), whyText(a.rs, l.ProjectSource, l.ProjectRule), l.ProjectRule})
		}
		return out, nil
	}
	return nil, nil
}

// whyText is the reason a source gives, a rule's file, number and match
// fields for a rule.
func whyText(rs *classify.Ruleset, src classify.Source, ref string) string {
	switch src {
	case classify.SourceRule, classify.SourceLearned:
		file, n, _ := strings.Cut(ref, "#")
		head := fmt.Sprintf("%s, rule %s", file, n)
		r, ok := rs.RuleByRef(ref)
		if !ok {
			return head + " (changed since)"
		}
		text := head + ": " + ruleWords(r)
		if r.SinceMS != 0 {
			text += ", from " + classify.SinceShort(r.SinceMS)
		}
		return text
	case classify.SourceManual:
		return "your tag"
	case classify.SourceCall:
		return "a call was on"
	case classify.SourceCalendar:
		return "a calendar event"
	case classify.SourceKept:
		return "kept when its title was deleted"
	case classify.SourceModule:
		return "a module named in the title"
	case classify.SourceProject:
		return "the project's category"
	case classify.SourcePlan:
		return "your plan"
	case classify.SourceModel:
		return "local model"
	case classify.SourceScreen:
		return "local model, from words read on screen"
	}
	return string(src)
}

// ruleWords is a rule's match fields as the dashboard says them.
func ruleWords(r classify.Rule) string {
	var parts []string
	if len(r.Bundle) > 0 {
		parts = append(parts, "bundle "+strings.Join(r.Bundle, ", "))
	}
	for _, f := range []struct{ name, val string }{{"app", r.App}, {"site", r.URL}, {"title", r.Title}, {"folder", r.Dir}} {
		if f.val != "" {
			parts = append(parts, f.name+" ~ "+f.val)
		}
	}
	return strings.Join(parts, ", ")
}

// modelWhy is the local model's reason for l: its share for the category
// and the title words that pushed it there most.
func (a *App) modelWhy(v classify.SpanView, l classify.Label) string {
	text := whyText(a.rs, l.CategorySource, "")
	m, err := a.labelModel()
	if err != nil || m == nil {
		return text
	}
	rank, _ := m.Rank(v, false, "")
	for _, s := range rank {
		if s.Category == l.Category {
			text += fmt.Sprintf(", %d %%", int(math.Round(s.P*100)))
		}
	}
	var words []string
	for _, w := range m.Because(v, l.Category, "") {
		if w.P > 0 && len(words) < 3 {
			words = append(words, w.Category)
		}
	}
	if len(words) > 0 && !v.Private {
		text += ": " + strings.Join(words, ", ")
	}
	return text
}
