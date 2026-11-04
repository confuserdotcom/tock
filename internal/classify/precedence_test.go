package classify

import (
	"testing"

	"tock/internal/span"
)

// precedenceRules declares one source of each kind, all able to label the
// "x" app, so each case below shows which one wins.
const precedenceRules = `
[[category]]
name = "Manual"
kind = "neutral"

[[category]]
name = "Call"
kind = "neutral"

[[category]]
name = "Study"
kind = "deep"

[[category]]
name = "Rule"
kind = "neutral"

[[category]]
name = "Meet"
kind = "shallow"

[[category]]
name = "Fun"
kind = "distract"

[[project]]
name = "Algebre"
category = "Study"

[[project]]
name = "Clips"
category = "Fun"
match = false

[[project]]
name = "Side"

[calls]
category = "Call"

[calendar]
category = "Meet"

[[rule]]
bundle = ["x"]
category = "Rule"

[[rule]]
bundle = ["x"]
project = "Clips"

[[rule]]
bundle = ["p"]
project = "Clips"
`

const precedenceLearned = `
[[rule]]
bundle = ["learned"]
category = "Fun"
`

// TestPrecedence pins the order in Label's doc, one adjacent pair a case.
func TestPrecedence(t *testing.T) {
	rs, err := Parse([]byte(precedenceRules), []byte(precedenceLearned+"\n[[rule]]\nbundle = [\"x\"]\ntitle = \"learned\"\ncategory = \"Fun\"\n"))
	if err != nil {
		t.Fatal(err)
	}
	view := func(bundle, title string) SpanView {
		return SpanView{ID: 1, Start: 0, End: 1, BundleID: bundle, App: bundle, Title: sp(title)}
	}
	call := func(v SpanView) SpanView { v.InCall = true; return v }
	cases := []struct {
		name    string
		v       SpanView
		o       *Override
		cat     string
		src     Source
		proj    string
		projSrc Source
	}{
		{"manual beats the call app", call(view("com.apple.FaceTime", "Algebre")), &Override{Category: "Manual"}, "Manual", SourceManual, "", SourceNone},
		{"the call app beats a module", call(view("com.apple.FaceTime", "Algebre")), nil, "Call", SourceCall, "", SourceNone},
		{"a module beats a learned rule", view("x", "Algebre learned"), nil, "Study", SourceModule, "Algebre", SourceModule},
		{"a learned rule beats a user rule", view("x", "learned"), nil, "Fun", SourceLearned, "Clips", SourceRule},
		{"a user rule beats the call fill", call(view("x", "notes")), nil, "Rule", SourceRule, "", SourceNone},
		{"the call fills what nothing labels", call(view("other", "notes")), nil, "Call", SourceCall, "", SourceNone},
		{"kept stands in for rules and modules", SpanView{ID: 1, BundleID: "x", App: "x", Kept: &Kept{Category: "Fun", Project: "Clips"}}, nil, "Fun", SourceKept, "Clips", SourceKept},
		{"a meeting is the calendar's", SpanView{ID: 1, BundleID: span.MeetingBundleID, App: span.MeetingApp, Title: sp("1:1")}, nil, "Meet", SourceCalendar, "", SourceNone},
		{"a project alone files its category", view("p", "anything"), nil, "Fun", SourceProject, "Clips", SourceRule},
		{"a rule's project filed elsewhere is dropped", view("x", "notes"), nil, "Rule", SourceRule, "", SourceNone},
		{"a manual project beats a module", view("x", "Algebre"), &Override{Project: "Side"}, "Study", SourceModule, "Side", SourceManual},
	}
	for _, c := range cases {
		l := rs.Label(c.v, c.o)
		if l.Category != c.cat || l.CategorySource != c.src || l.Project != c.proj || l.ProjectSource != c.projSrc {
			t.Errorf("%s: got %s/%s project %s/%s, want %s/%s project %s/%s", c.name,
				l.Category, l.CategorySource, l.Project, l.ProjectSource, c.cat, c.src, c.proj, c.projSrc)
		}
	}
	// The plan's module labels the project only within its own category.
	v := view("other", "notes")
	v.Planned = "Algebre"
	if l := rs.Label(v, &Override{Category: "Study"}); l.Project != "Algebre" || l.ProjectSource != SourcePlan {
		t.Errorf("plan in its category: %+v", l)
	}
	if l := rs.Label(v, &Override{Category: "Fun"}); l.Project != "" {
		t.Errorf("plan outside its category: %+v", l)
	}
}
