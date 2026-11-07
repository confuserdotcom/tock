package app

import (
	"fmt"
	"net/url"
	"sort"
	"strings"

	"tock/internal/classify"
	"tock/internal/span"
)

// RuleRef names a rule the caller has seen: its file, its 1-based index
// there and its Fingerprint then.
type RuleRef struct {
	File        string
	Index       int
	Fingerprint string
}

// RuleSpec is a rule as the dashboard edits it: its match patterns and
// what it files time as, "Category", "Category/Project" or "/Project".
type RuleSpec struct {
	App, Title, URL, Dir string
	Label                string
}

// AddedComment is the comment above a rule added from the dashboard.
const AddedComment = "added in the dashboard"

// ruleOf checks s and resolves its label against the current rules.
func (a *App) ruleOf(s RuleSpec) (classify.Rule, error) {
	r := classify.Rule{App: strings.TrimSpace(s.App), Title: strings.TrimSpace(s.Title), URL: strings.TrimSpace(s.URL), Dir: strings.TrimSpace(s.Dir)}
	if r.App == "" && r.Title == "" && r.URL == "" && r.Dir == "" {
		return r, fmt.Errorf("a rule needs something to match: an app, a site, a title or a folder")
	}
	if strings.TrimSpace(s.Label) == "" {
		return r, fmt.Errorf("a rule needs a category or a project to file time under")
	}
	w, err := a.rs.ParseWant(s.Label)
	if err != nil {
		return r, err
	}
	r.Category, r.Project = w.Category, w.Project
	return r, nil
}

// findRule is the rule at ref, if it is still the one ref saw.
func (a *App) findRule(ref RuleRef) (classify.Rule, error) {
	list := a.rs.Rules
	switch ref.File {
	case classify.LearnedFile:
		list = a.rs.Learned
	case classify.RulesFile:
	default:
		return classify.Rule{}, fmt.Errorf("unknown rule file %q", ref.File)
	}
	for _, r := range list {
		if r.Index == ref.Index {
			if r.Fingerprint() != ref.Fingerprint {
				return r, fmt.Errorf("app: %s: %w", ref.ref(), ErrRuleChanged)
			}
			return r, nil
		}
	}
	return classify.Rule{}, fmt.Errorf("app: %s: %w", ref.ref(), ErrRuleChanged)
}

func (r RuleRef) ref() string { return fmt.Sprintf("%s#%d", r.File, r.Index) }

// ruleEdit is the text change of an add (ref nil), an edit or, with remove,
// a delete, ready for rewriteRules or a preview.
func (a *App) ruleEdit(ref *RuleRef, s RuleSpec, remove bool) (func(rules, learned []byte) ([]byte, []byte, error), error) {
	if ref == nil {
		r, err := a.ruleOf(s)
		if err != nil {
			return nil, err
		}
		return func(rules, learned []byte) ([]byte, []byte, error) {
			out, err := classify.InsertRuleText(classify.RulesFile, rules, r, AddedComment)
			return out, learned, err
		}, nil
	}
	old, err := a.findRule(*ref)
	if err != nil {
		return nil, err
	}
	pick := func(rules, learned *[]byte) *[]byte {
		if ref.File == classify.LearnedFile {
			return learned
		}
		return rules
	}
	if remove {
		return func(rules, learned []byte) ([]byte, []byte, error) {
			p := pick(&rules, &learned)
			out, err := classify.RemoveRules(ref.File, *p, []int{ref.Index - 1})
			if err != nil {
				return nil, nil, err
			}
			*p = out
			return rules, learned, nil
		}, nil
	}
	r, err := a.ruleOf(s)
	if err != nil {
		return nil, err
	}
	return func(rules, learned []byte) ([]byte, []byte, error) {
		p := pick(&rules, &learned)
		text := *p
		// New values first, removals last, so the rule always keeps a match.
		var later [][2]string
		for _, f := range [][3]string{
			{"app", old.App, r.App}, {"title", old.Title, r.Title}, {"url", old.URL, r.URL}, {"dir", old.Dir, r.Dir},
			{"category", old.Category, r.Category}, {"project", old.Project, r.Project},
		} {
			if f[1] == f[2] {
				continue
			}
			if f[2] == "" {
				later = append(later, [2]string{f[0], ""})
				continue
			}
			var err error
			if text, err = classify.SetRuleField(ref.File, text, ref.Index-1, f[0], classify.TOMLString(f[2])); err != nil {
				return nil, nil, err
			}
		}
		for _, f := range later {
			var err error
			if text, err = classify.SetRuleField(ref.File, text, ref.Index-1, f[0], f[1]); err != nil {
				return nil, nil, err
			}
		}
		*p = text
		return rules, learned, nil
	}, nil
}

// AddRule writes s as the first rule of rules.toml: checked after the
// learned rules and before every other rule you wrote.
func (a *App) AddRule(s RuleSpec) error {
	edit, err := a.ruleEdit(nil, s, false)
	if err != nil {
		return err
	}
	return a.rewriteRules(edit)
}

// EditRule changes the rule at ref to s, field by field, in its own file.
func (a *App) EditRule(ref RuleRef, s RuleSpec) error {
	edit, err := a.ruleEdit(&ref, s, false)
	if err != nil {
		return err
	}
	return a.rewriteRules(edit)
}

// RemoveRule deletes the rule at ref from its file.
func (a *App) RemoveRule(ref RuleRef) error {
	edit, err := a.ruleEdit(&ref, RuleSpec{}, true)
	if err != nil {
		return err
	}
	return a.rewriteRules(edit)
}

// RulePreview is what a rule change would do to the last StatsDays days:
// the time the rule would decide, the time whose label would change, and
// the biggest of those things.
type RulePreview struct {
	MatchedMS, ChangedMS int64
	Examples             []RuleExample
}

// RuleExample is one thing (a site, or an app and title) a previewed rule
// takes: its time and its label before and after, "Category / Project".
type RuleExample struct {
	Text     string
	MS       int64
	From, To string
}

// PreviewExamples is how many things a preview lists.
const PreviewExamples = 6

// PreviewRule is what AddRule (ref nil) or EditRule would do, without
// writing anything.
func (a *App) PreviewRule(ref *RuleRef, s RuleSpec) (RulePreview, error) {
	edit, err := a.ruleEdit(ref, s, false)
	if err != nil {
		return RulePreview{}, err
	}
	cur, err := a.rules().Read()
	if err != nil {
		return RulePreview{}, err
	}
	rules, learned, err := edit(cur.Rules, cur.Learned)
	if err != nil {
		return RulePreview{}, err
	}
	cand, err := classify.Parse(rules, learned)
	if err != nil {
		return RulePreview{}, err
	}
	target := classify.RulesFile + "#1"
	if ref != nil {
		target = fmt.Sprintf("%s#%d", ref.File, ref.Index)
	}
	now := a.nowMS()
	from := now - int64(StatsDays)*libDayMS
	cut := a.cut
	defer func() { a.cut = cut }()
	rows, ovs, err := a.spans(from, now)
	if err != nil {
		return RulePreview{}, err
	}
	vs, err := a.decorate(a.base(rows, ovs, from, now, now), ovs, from, now)
	if err != nil {
		return RulePreview{}, err
	}
	before, after := a.rs.LabelAll(vs, ovs), cand.LabelAll(vs, ovs)
	var p RulePreview
	type key struct{ text, from, to string }
	changed, matched := map[key]int64{}, map[key]int64{}
	for i := range vs {
		v := &vs[i]
		if v.BundleID == span.IdleBundleID {
			continue
		}
		ms := v.DurMS()
		b, n := before[i], after[i]
		k := key{thingText(v), labelText(b), labelText(n)}
		if n.CategoryRule == target || n.ProjectRule == target {
			p.MatchedMS += ms
			matched[k] += ms
		}
		if b.Category != n.Category || b.Project != n.Project {
			p.ChangedMS += ms
			changed[k] += ms
		}
	}
	pickFrom := changed
	if len(changed) == 0 {
		pickFrom = matched
	}
	for k, ms := range pickFrom {
		p.Examples = append(p.Examples, RuleExample{k.text, ms, k.from, k.to})
	}
	sort.Slice(p.Examples, func(i, j int) bool {
		if p.Examples[i].MS != p.Examples[j].MS {
			return p.Examples[i].MS > p.Examples[j].MS
		}
		return p.Examples[i].Text < p.Examples[j].Text
	})
	if len(p.Examples) > PreviewExamples {
		p.Examples = p.Examples[:PreviewExamples]
	}
	return p, nil
}

// thingText names what a view was: its site, else its app and title.
func thingText(v *classify.SpanView) string {
	title := ""
	if v.Title != nil {
		title = *v.Title
	}
	if v.URL != nil && *v.URL != "" {
		u := *v.URL
		if !strings.Contains(u, "://") {
			u = "https://" + u
		}
		if p, err := url.Parse(u); err == nil && p.Host != "" {
			return strings.TrimPrefix(p.Host, "www.")
		}
	}
	if title != "" && title != v.App {
		return v.App + " · " + title
	}
	return v.App
}

func labelText(l classify.Label) string {
	switch {
	case l.Project == "":
		return l.Category
	case l.Category == "":
		return "/ " + l.Project
	}
	return l.Category + " / " + l.Project
}
