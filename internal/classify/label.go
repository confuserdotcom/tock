package classify

import (
	"strings"

	"tock/internal/span"
)

// Source says where a label field came from.
type Source string

const (
	SourceNone     Source = "none"
	SourceRule     Source = "rule"
	SourceLearned  Source = "learned"
	SourceManual   Source = "manual"
	SourceCall     Source = "call"
	SourceCalendar Source = "calendar"
	SourceKept     Source = "kept"    // the rule label kept when the title was deleted
	SourceModule   Source = "module"  // a module's name or alias in the title
	SourceProject  Source = "project" // the category the project is filed under
	SourcePlan     Source = "plan"    // the module of the plan's task at that time
)

// Kept is the rule label a span kept when [privacy] keep_titles deleted its
// title. It stands in for the rules, which can no longer see the title.
type Kept struct {
	Category, Project string
}

// Override is a manual label for one span. Empty fields fall through to rules.
type Override struct {
	Category, Project string
}

// Label is the classification of one span. Kind is the category's kind.
// CategoryRule/ProjectRule hold Rule.Ref() for rule-sourced fields.
type Label struct {
	Category, Kind, Project       string
	CategorySource, ProjectSource Source
	CategoryRule, ProjectRule     string
	// Quiet means its distraction time never triggers a distraction notice:
	// the category, or the rule that set it, is marked interrupt = false.
	// It changes nothing else about the label.
	Quiet bool
}

type memoKey struct {
	bundle, app, title, url  string
	dir                      string
	hasTitle, hasURL, hasDir bool
	private                  bool
	era                      int // see Ruleset.era; 0 without dated rules
}

// Label classifies v. Idle spans get an empty label. o may be nil.
//
// Precedence, highest first (TestPrecedence pins each step; change them
// together). Category:
//  1. a manual tag (tock tag, away labels and timeline edits come as o);
//  2. the call: call time, a call app or a meeting site in front during a
//     call ([calls]);
//  3. the kept label of a span whose title was deleted, else a module named
//     in the title or directory;
//  4. the first matching learned rule, then 5. the first matching user rule;
//  6. the local model (ApplyModel, after Label): fills what 1-5 left, and
//     may override a generic rule;
//  7. the call, for any other view in a call that nothing above labelled
//     (a call is context, not an activity);
//  8. the calendar, for meeting time ([calendar]);
//  9. the category the project is filed under.
//
// Project: a manual project; else the kept or module project; else the
// first matching learned, then user rule; else the plan's module when it
// is filed under the label's category. A project that is not manual and is
// filed under another category than the label's is dropped.
func (rs *Ruleset) Label(v SpanView, o *Override) Label {
	var l Label
	switch {
	case v.BundleID != span.IdleBundleID && v.Kept != nil:
		l = rs.keptLabel(&v)
	case v.BundleID != span.IdleBundleID:
		l = rs.ruleLabel(&v)
	default:
		l = Label{CategorySource: SourceNone, ProjectSource: SourceNone}
		o = nil
	}
	// After the memoized rule label, so the memo key needs no InCall.
	// A call is context: it labels what has no category of its own, and
	// the call apps and meeting sites, never other labelled activity.
	if v.InCall && rs.CallCategory != "" && v.BundleID != span.IdleBundleID && (l.Category == "" || rs.isCallView(&v)) {
		l.Category, l.Kind = rs.CallCategory, rs.Kind(rs.CallCategory)
		l.CategorySource, l.CategoryRule = SourceCall, ""
	}
	if IsMeeting(v) && l.Category == "" && rs.CalendarCategory != "" {
		l.Category, l.Kind = rs.CalendarCategory, rs.Kind(rs.CalendarCategory)
		l.CategorySource, l.CategoryRule = SourceCalendar, ""
	}
	if o != nil && o.Category != "" {
		l.Category, l.Kind = o.Category, rs.Kind(o.Category)
		l.CategorySource, l.CategoryRule = SourceManual, ""
	}
	if o != nil && o.Project != "" {
		l.Project = o.Project
		l.ProjectSource, l.ProjectRule = SourceManual, ""
	}
	// A project filed under a category labels only time in that category:
	// a lecture on a video site is not the Video project, and a call titled
	// with a module is not that module. Only a manual project stays.
	if l.Project != "" && l.Category != "" && l.ProjectSource != SourceManual {
		if c := rs.ProjectCategory(l.Project); c != "" && !strings.EqualFold(c, l.Category) {
			l.Project, l.ProjectSource, l.ProjectRule = "", SourceNone, ""
		}
	}
	if l.Project == "" && v.Planned != "" {
		if c := rs.ProjectCategory(v.Planned); c != "" && strings.EqualFold(c, l.Category) {
			l.Project, l.ProjectSource, l.ProjectRule = v.Planned, SourcePlan, ""
		}
	}
	if l.Category == "" && l.Project != "" {
		if c := rs.ProjectCategory(l.Project); c != "" {
			l.Category, l.Kind = c, rs.Kind(c)
			l.CategorySource, l.CategoryRule = SourceProject, ""
		}
	}
	l.Quiet = rs.quiet(l.Category, l.CategoryRule)
	return l
}

// memoMax bounds the label memo and the model memo; past it a memo starts
// over. A day has under a thousand distinct titles; a bigger memo only
// pays during a rebuild of history, and costs a daemon its memory budget
// (50,000 entries held about 25 MB).
const memoMax = 4_096

// keyOf is v's memo key and its cleaned title ("" when it has none).
func keyOf(v *SpanView) (memoKey, string) {
	k := memoKey{bundle: v.BundleID, app: v.App, private: v.Private}
	clean := ""
	if v.Title != nil {
		clean = Clean(*v.Title)
		k.title, k.hasTitle = clean, true
	}
	if v.URL != nil {
		k.url, k.hasURL = *v.URL, true
	}
	if v.Dir != nil {
		k.dir, k.hasDir = *v.Dir, true
	}
	return k, clean
}

// keptLabel is v's kept label over its rule label: a kept field wins, and
// an empty one, or one naming a category or project that no longer exists,
// falls through to the rules, which still see the app and the URL host.
func (rs *Ruleset) keptLabel(v *SpanView) Label {
	k := *v.Kept
	l := rs.ruleLabel(v)
	if k.Category != "" && rs.HasCategory(k.Category) {
		l.Category, l.Kind = k.Category, rs.Kind(k.Category)
		l.CategorySource, l.CategoryRule = SourceKept, ""
	}
	if k.Project != "" && rs.HasProject(k.Project) {
		l.Project = k.Project
		l.ProjectSource, l.ProjectRule = SourceKept, ""
	}
	return l
}

func (rs *Ruleset) ruleLabel(v *SpanView) Label {
	// The memo is keyed on the raw title: cleaning it costs more than the
	// lookup, and the label depends on nothing else but the era.
	k := memoKey{bundle: v.BundleID, app: v.App, private: v.Private, era: rs.era(v.Start)}
	if v.Title != nil {
		k.title, k.hasTitle = *v.Title, true
	}
	if v.URL != nil {
		k.url, k.hasURL = *v.URL, true
	}
	if v.Dir != nil {
		k.dir, k.hasDir = *v.Dir, true
	}
	rs.mu.Lock()
	l, ok := rs.memo[k]
	rs.mu.Unlock()
	if ok {
		return l
	}
	clean := ""
	if v.Title != nil {
		clean = Clean(*v.Title)
	}

	l = Label{CategorySource: SourceNone, ProjectSource: SourceNone}
	dir := ""
	if v.Dir != nil {
		dir = *v.Dir
	}
	if m := rs.moduleFor(clean, dir); m != nil {
		l.Category, l.Kind, l.Project = m.category, rs.Kind(m.category), m.project
		l.CategorySource, l.ProjectSource = SourceModule, SourceModule
		l.CategoryRule, l.ProjectRule = "module:"+m.project, "module:"+m.project
	}
	for _, set := range []struct {
		rules []Rule
		src   Source
	}{{rs.Learned, SourceLearned}, {rs.Rules, SourceRule}} {
		for i := range set.rules {
			r := &set.rules[i]
			needCat := l.Category == "" && r.Category != ""
			needProj := l.Project == "" && r.Project != ""
			if (!needCat && !needProj) || !r.matches(v, clean) {
				continue
			}
			if needCat {
				l.Category, l.Kind = r.Category, rs.Kind(r.Category)
				l.CategorySource, l.CategoryRule = set.src, r.Ref()
			}
			if needProj {
				l.Project = r.Project
				l.ProjectSource, l.ProjectRule = set.src, r.Ref()
			}
		}
	}

	rs.mu.Lock()
	if len(rs.memo) >= memoMax { // a daemon sees new titles for weeks
		clear(rs.memo)
	}
	rs.memo[k] = l
	rs.mu.Unlock()
	return l
}

// LabelAll labels views[i] with overrides[views[i].ID].
func (rs *Ruleset) LabelAll(views []SpanView, overrides map[int64]Override) []Label {
	out := make([]Label, len(views))
	for i := range views {
		var o *Override
		if ov, ok := overrides[views[i].ID]; ok {
			o = &ov
		}
		out[i] = rs.Label(views[i], o)
	}
	return out
}
