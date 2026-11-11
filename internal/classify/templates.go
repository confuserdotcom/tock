package classify

import (
	"embed"
	"fmt"
	"slices"
	"strings"
	"time"
)

//go:embed templates/*.toml
var templateFS embed.FS

// Template is a set of categories and rules for one use of the computer,
// plus the capability group it wants on. Applying one only ever appends
// what is missing (see PlanTemplates); no key records it was applied.
type Template struct {
	Name  string // what --template takes
	Title string // what the plan and the file comment say
	Group string // the config feature it wants on ("billing", "focus")
	Rules []byte // rules.toml fragment: [[category]] and [[rule]] tables
}

var templates = []struct{ name, title, group string }{
	{"client", "client work", "billing"},
	{"own", "my own projects", "focus"},
}

// AllTemplates returns every template, in the order setup offers them.
func AllTemplates() []Template {
	out := make([]Template, len(templates))
	for i, t := range templates {
		b, err := templateFS.ReadFile("templates/" + t.name + ".toml")
		if err != nil {
			panic(err) // embedded: a missing file is a build mistake
		}
		out[i] = Template{Name: t.name, Title: t.title, Group: t.group, Rules: b}
	}
	return out
}

// TemplateNames lists the names --template takes.
func TemplateNames() []string {
	out := make([]string, len(templates))
	for i, t := range templates {
		out[i] = t.name
	}
	return out
}

// TemplatesByName returns the named templates in the order given, each once.
func TemplatesByName(names []string) ([]Template, error) {
	all := AllTemplates()
	var out []Template
	for _, n := range names {
		n = strings.TrimSpace(n)
		i := slices.IndexFunc(all, func(t Template) bool { return t.Name == n })
		if i < 0 {
			return nil, fmt.Errorf("template %q: want %s", n, strings.Join(TemplateNames(), " or "))
		}
		if !slices.ContainsFunc(out, func(t Template) bool { return t.Name == n }) {
			out = append(out, all[i])
		}
	}
	return out, nil
}

// TemplatePlan is what applying templates would change in rules.toml.
type TemplatePlan struct {
	AddCategories  []string // declared by a template, missing from yours
	HaveCategories []string // yours already, same kind
	KeptCategories []string // yours already with another kind: yours stays
	AddRules       []Rule   // missing rules, in template order
	HaveRules      []Rule   // your identical rules (SinceMS set: dated)
	Text           []byte   // rules.toml after the change; the input when Empty
}

// Empty is whether the plan changes nothing.
func (p TemplatePlan) Empty() bool { return len(p.AddCategories) == 0 && len(p.AddRules) == 0 }

// ruleIdentity is what makes two rules the same for a template: their match
// fields and what they set. since is left out, so applying again never
// duplicates a dated rule.
func ruleIdentity(r Rule) string {
	b := slices.Clone(r.Bundle)
	slices.Sort(b)
	return strings.Join([]string{strings.Join(b, "\x00"), r.App, r.Title, r.URL, r.Dir,
		nameKey(r.Category), nameKey(r.Project)}, "\x01")
}

// PlanTemplates plans applying tpls to rules.toml, whose text is rulesText
// and whose parse is rs. Nothing of yours changes: missing categories and
// rules are appended after your text behind a comment naming the template
// and the day of nowMS. Unless past, each new rule gets since = nowMS so it
// labels only what comes after (history keeps its labels). A text that
// does not parse is an error and nothing is planned.
func PlanTemplates(rs *Ruleset, rulesText []byte, tpls []Template, nowMS int64, past bool) (TemplatePlan, error) {
	if _, err := Parse(rulesText, nil); err != nil {
		return TemplatePlan{}, err
	}
	p := TemplatePlan{Text: rulesText}
	haveRule := map[string]Rule{}
	for _, r := range rs.Rules {
		if _, ok := haveRule[ruleIdentity(r)]; !ok {
			haveRule[ruleIdentity(r)] = r
		}
	}
	seenCat, seenRule := map[string]bool{}, map[string]bool{}
	var blocks []string
	for _, tp := range tpls {
		trs, err := Parse(tp.Rules, nil)
		if err != nil {
			return TemplatePlan{}, fmt.Errorf("template %s: %w", tp.Name, err)
		}
		var b strings.Builder
		for _, c := range trs.Categories {
			k := nameKey(c.Name)
			if seenCat[k] {
				continue
			}
			seenCat[k] = true
			if mine, ok := rs.catByLower[k]; ok {
				if mine.Kind == c.Kind {
					p.HaveCategories = append(p.HaveCategories, mine.Name)
				} else {
					p.KeptCategories = append(p.KeptCategories, mine.Name)
				}
				continue
			}
			p.AddCategories = append(p.AddCategories, c.Name)
			fmt.Fprintf(&b, "[[category]]\nname = %s\nkind = %s\n\n", TOMLString(c.Name), TOMLString(c.Kind))
		}
		for _, r := range trs.Rules {
			id := ruleIdentity(r)
			if seenRule[id] {
				continue
			}
			seenRule[id] = true
			if mine, ok := haveRule[id]; ok {
				p.HaveRules = append(p.HaveRules, mine)
				continue
			}
			if !past {
				r.SinceMS = nowMS
			}
			r.File, r.Index = RulesFile, 0
			p.AddRules = append(p.AddRules, r)
			b.WriteString(ruleTable(r))
		}
		if b.Len() > 0 {
			day := time.UnixMilli(nowMS).In(time.Local).Format(time.DateOnly)
			blocks = append(blocks, fmt.Sprintf("# from template %q, %s\n", tp.Title, day)+b.String())
		}
	}
	if len(blocks) == 0 {
		return p, nil
	}
	text := string(rulesText) // kept byte for byte
	switch {
	case text == "" || strings.HasSuffix(text, "\n\n"):
	case strings.HasSuffix(text, "\n"):
		text += "\n"
	default:
		text += "\n\n"
	}
	p.Text = []byte(strings.TrimRight(text+strings.Join(blocks, ""), "\n") + "\n")
	if _, err := Parse(p.Text, nil); err != nil {
		return TemplatePlan{}, fmt.Errorf("template: result does not parse: %w", err)
	}
	return p, nil
}
