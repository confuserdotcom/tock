package classify

import (
	"testing"

	"tock/internal/span"
)

const statsRules = `
[[category]]
name = "Code"
kind = "deep"

[[category]]
name = "Study"
kind = "deep"

[[project]]
name = "tock"

[[rule]]
bundle = ["term"]
category = "Code"

[[rule]]
bundle = ["term"]
title = '^x'
category = "Study"

[[rule]]
url = '^never'
category = "Study"

[[rule]]
bundle = ["term"]
title = '^tock'
project = "tock"
`

func TestRuleStats(t *testing.T) {
	rs, err := Parse([]byte(statsRules), []byte("[[rule]]\nbundle = [\"other\"]\ncategory = \"Study\"\n"))
	if err != nil {
		t.Fatal(err)
	}
	const min = 60_000
	views := []SpanView{
		{ID: 1, Start: 0, End: 10 * min, BundleID: "term", App: "T", Title: sp("tock a")},
		{ID: 2, Start: 10 * min, End: 15 * min, BundleID: "term", App: "T", Title: sp("x")},
		{ID: 3, Start: 15 * min, End: 22 * min, BundleID: span.IdleBundleID, App: span.IdleApp},
		{ID: 4, Start: 22 * min, End: 24 * min, BundleID: "term", App: "T", Title: sp("tock b")},
	}
	got := rs.Stats(views, rs.LabelAll(views, nil))
	want := []struct {
		ref              string
		matched, decided int64
		status           string
	}{
		{"learned.toml#1", 0, 0, StatusUnused},
		{"rules.toml#1", 17 * min, 17 * min, ""},
		{"rules.toml#2", 5 * min, 0, StatusShadowed},
		{"rules.toml#3", 0, 0, StatusUnused},
		{"rules.toml#4", 12 * min, 12 * min, ""},
	}
	if len(got) != len(want) {
		t.Fatalf("got %d stats, want %d", len(got), len(want))
	}
	for i, w := range want {
		g := got[i]
		if g.Rule.Ref() != w.ref || g.MatchedMS != w.matched || g.DecidedMS != w.decided || g.Status() != w.status {
			t.Errorf("%d: got %s matched=%d decided=%d %q, want %+v", i, g.Rule.Ref(), g.MatchedMS, g.DecidedMS, g.Status(), w)
		}
	}
}

func TestRuleStatsCountsOnceWhenBothFields(t *testing.T) {
	rs, err := Parse([]byte(statsRules+"\n[[rule]]\nbundle = [\"b\"]\ncategory = \"Code\"\nproject = \"tock\"\n"), nil)
	if err != nil {
		t.Fatal(err)
	}
	views := []SpanView{{ID: 1, Start: 0, End: 1000, BundleID: "b", App: "B"}}
	got := rs.Stats(views, rs.LabelAll(views, nil))
	if s := got[len(got)-1]; s.DecidedMS != 1000 || s.MatchedMS != 1000 {
		t.Fatalf("got %+v", s)
	}
}

func TestRuleStatsManualOverride(t *testing.T) {
	rs, err := Parse([]byte(statsRules), nil)
	if err != nil {
		t.Fatal(err)
	}
	views := []SpanView{{ID: 1, Start: 0, End: 1000, BundleID: "term", App: "T", Title: sp("q")}}
	labels := rs.LabelAll(views, map[int64]Override{1: {Category: "Study"}})
	got := rs.Stats(views, labels)
	if s := got[0]; s.MatchedMS != 1000 || s.DecidedMS != 0 || s.Status() != StatusShadowed {
		t.Fatalf("manual label must not count as decided: %+v", s)
	}
}
