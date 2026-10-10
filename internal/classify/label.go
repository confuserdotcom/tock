package classify

import "tock/internal/span"

// Source says where a label field came from.
type Source string

const (
	SourceNone     Source = "none"
	SourceRule     Source = "rule"
	SourceLearned  Source = "learned"
	SourceManual   Source = "manual"
	SourceCall     Source = "call"
	SourceCalendar Source = "calendar"
)

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
}

type memoKey struct {
	bundle, app, title, url  string
	dir                      string
	hasTitle, hasURL, hasDir bool
	private                  bool
}

// Label classifies v. Idle spans get an empty label. o may be nil. Each field
// is decided separately: manual override, else (category only) CallCategory
// for a view in a call, else the first matching learned rule that sets the
// field, else the first matching user rule, else (category only)
// CalendarCategory for a meeting view, else empty.
func (rs *Ruleset) Label(v SpanView, o *Override) Label {
	var l Label
	if v.BundleID != span.IdleBundleID {
		l = rs.ruleLabel(&v)
	} else {
		l = Label{CategorySource: SourceNone, ProjectSource: SourceNone}
		o = nil
	}
	// After the memoized rule label, so the memo key needs no InCall.
	if v.InCall && rs.CallCategory != "" && v.BundleID != span.IdleBundleID {
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
	return l
}

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

func (rs *Ruleset) ruleLabel(v *SpanView) Label {
	k, clean := keyOf(v)
	rs.mu.Lock()
	l, ok := rs.memo[k]
	rs.mu.Unlock()
	if ok {
		return l
	}

	l = Label{CategorySource: SourceNone, ProjectSource: SourceNone}
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
