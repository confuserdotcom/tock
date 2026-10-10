package classify

import "testing"

func mustParse(t *testing.T, rules, learned string) *Ruleset {
	t.Helper()
	var l []byte
	if learned != "" {
		l = []byte(learned)
	}
	rs, err := Parse([]byte(rules), l)
	if err != nil {
		t.Fatal(err)
	}
	return rs
}

const labelRules = baseRules + `
[[project]]
name = "ATLAS"

[[rule]]
bundle = ["term"]
title = '^tock │'
project = "tock"

[[rule]]
bundle = ["term"]
category = "Code"

[[rule]]
url = '^(www\.)?youtube\.com(/|$)'
category = "Entertainment"

[[rule]]
app = 'Arc'
title = 'secret'
category = "Code"
`

func TestLabelPerFieldFirstMatch(t *testing.T) {
	rs := mustParse(t, labelRules, "")
	l := rs.Label(SpanView{BundleID: "term", App: "WezTerm", Title: sp("tock │ ✳ fix")}, nil)
	if l.Category != "Code" || l.Kind != KindDeep || l.Project != "tock" {
		t.Fatalf("label = %+v, want Code (deep) / tock from two rules", l)
	}
	if l.CategoryRule != "rules.toml#2" || l.ProjectRule != "rules.toml#1" ||
		l.CategorySource != SourceRule || l.ProjectSource != SourceRule {
		t.Fatalf("refs/sources = %+v", l)
	}
	l = rs.Label(SpanView{BundleID: "other", App: "X", Title: sp("tock │ y")}, nil)
	if l.Category != "" || l.Project != "" || l.CategorySource != SourceNone {
		t.Fatalf("unmatched span labelled: %+v", l)
	}
}

func TestLabelAndCaseInsensitive(t *testing.T) {
	rs := mustParse(t, labelRules, "")
	if l := rs.Label(SpanView{BundleID: "b", App: "ARC", Title: sp("My SECRET plan")}, nil); l.Category != "Code" {
		t.Fatalf("case-insensitive app+title rule did not match: %+v", l)
	}
	if l := rs.Label(SpanView{BundleID: "b", App: "Arc", Title: sp("public")}, nil); l.Category != "" {
		t.Fatalf("rule matched with only one of two fields: %+v", l)
	}
	rs = mustParse(t, baseRules+"[[rule]]\napp='(?-i)arc'\ncategory='Code'\n", "")
	if l := rs.Label(SpanView{BundleID: "b", App: "Arc"}, nil); l.Category != "" {
		t.Fatalf("(?-i) did not make the match case-sensitive: %+v", l)
	}
}

func TestLabelNilFieldsNeverMatch(t *testing.T) {
	rs := mustParse(t, labelRules, "")
	if l := rs.Label(SpanView{BundleID: "b", App: "Arc"}, nil); l.Category != "" {
		t.Fatalf("title rule matched a nil title: %+v", l)
	}
	if l := rs.Label(SpanView{BundleID: "b", App: "Arc", Private: true}, nil); l.Category != "" {
		t.Fatalf("private span matched a title/url rule: %+v", l)
	}
}

func TestLabelLearnedFirstAndManualWins(t *testing.T) {
	learned := `
[[rule]]
url = '^(www\.)?youtube\.com/watch'
category = "Code"
project = "ATLAS"

[[rule]]
url = 'youtube'
category = "Entertainment"
`
	rs := mustParse(t, labelRules, learned)
	v := SpanView{ID: 7, BundleID: "arc", App: "Arc", URL: sp("www.youtube.com/watch")}
	l := rs.Label(v, nil)
	if l.Category != "Code" || l.CategorySource != SourceLearned || l.CategoryRule != "learned.toml#1" {
		t.Fatalf("newest learned rule must win: %+v", l)
	}
	l = rs.Label(v, &Override{Project: "tock"})
	if l.Category != "Code" || l.Project != "tock" || l.ProjectSource != SourceManual || l.CategorySource != SourceLearned {
		t.Fatalf("project-only override: %+v", l)
	}
	l = rs.Label(v, &Override{Category: "Entertainment"})
	if l.Category != "Entertainment" || l.Kind != KindDistract || l.CategorySource != SourceManual || l.CategoryRule != "" {
		t.Fatalf("category override: %+v", l)
	}
	all := rs.LabelAll([]SpanView{v, {ID: 8, BundleID: "arc", App: "Arc"}}, map[int64]Override{7: {Category: "Entertainment"}})
	if all[0].Category != "Entertainment" || all[1].Category != "" {
		t.Fatalf("LabelAll = %+v", all)
	}
}

func TestLabelIdleNeverLabelled(t *testing.T) {
	rs := mustParse(t, baseRules+"[[rule]]\napp='.'\ncategory='Code'\n", "")
	l := rs.Label(SpanView{BundleID: "tock.idle", App: "Idle"}, &Override{Category: "Code"})
	if l.Category != "" || l.Project != "" {
		t.Fatalf("idle span labelled: %+v", l)
	}
}

func TestLabelMemoDistinguishesInputs(t *testing.T) {
	rs := mustParse(t, labelRules, "")
	a := rs.Label(SpanView{BundleID: "term", App: "W", Title: sp("tock │ a")}, nil)
	b := rs.Label(SpanView{BundleID: "term", App: "W", Title: sp("other")}, nil)
	c := rs.Label(SpanView{BundleID: "term", App: "W", Title: sp("tock │ ◐ a")}, nil)
	if a.Project != "tock" || b.Project != "" || c.Project != "tock" {
		t.Fatalf("memo mixed up labels: %+v %+v %+v", a, b, c)
	}
}
