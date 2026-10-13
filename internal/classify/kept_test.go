package classify

import "testing"

func TestKeptLabel(t *testing.T) {
	rs := mustParse(t, labelRules, "")
	v := SpanView{ID: 1, BundleID: "term", App: "WezTerm", Kept: &Kept{Category: "Code", Project: "tock"}}
	l := rs.Label(v, nil)
	if l.Category != "Code" || l.Project != "tock" || l.CategorySource != SourceKept || l.ProjectSource != SourceKept || l.Kind == "" {
		t.Fatalf("kept label = %+v", l)
	}
	// Manual overrides and calls still win over a kept label.
	if l := rs.Label(v, &Override{Project: "ATLAS"}); l.Project != "ATLAS" || l.Category != "Code" {
		t.Errorf("override over kept = %+v", l)
	}
	// Empty or stale fields fall through to the rules: term is Code by app.
	v.Kept = &Kept{Category: "Gone", Project: "gone"}
	if l := rs.Label(v, nil); l.Category != "Code" || l.CategorySource != SourceRule || l.Project != "" {
		t.Errorf("stale kept = %+v", l)
	}
	v.Kept = &Kept{Project: "ATLAS"}
	if l := rs.Label(v, nil); l.Category != "Code" || l.Project != "ATLAS" || l.ProjectSource != SourceKept {
		t.Errorf("project-only kept = %+v", l)
	}
}
