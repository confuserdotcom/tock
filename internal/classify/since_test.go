package classify

import (
	"strings"
	"testing"
	"time"
)

// sinceRules: a dated rule says "Code" from 2026-11-20 14:05 local time; an
// undated one says "Entertainment" for the same title before that.
const sinceRules = baseRules + `
[[rule]]
title = 'acme'
category = "Code"
since = 2026-11-20T14:05:00

[[rule]]
title = 'acme'
category = "Entertainment"
`

func localMS(t *testing.T, s string) int64 {
	t.Helper()
	tm, err := time.ParseInLocation("2006-01-02T15:04:05", s, time.Local)
	if err != nil {
		t.Fatal(err)
	}
	return tm.UnixMilli()
}

func TestSinceParsedInLocalZone(t *testing.T) {
	rs := mustParse(t, sinceRules, "")
	if got, want := rs.Rules[0].SinceMS, localMS(t, "2026-11-20T14:05:00"); got != want {
		t.Fatalf("SinceMS = %d, want %d", got, want)
	}
	if rs.Rules[1].SinceMS != 0 {
		t.Fatalf("undated rule SinceMS = %d", rs.Rules[1].SinceMS)
	}
}

func TestSinceAcceptsDateAndOffset(t *testing.T) {
	rs := mustParse(t, baseRules+`
[[rule]]
title = 'a'
category = "Code"
since = 2026-11-20

[[rule]]
title = 'b'
category = "Code"
since = 2026-11-20T10:00:00Z
`, "")
	if got, want := rs.Rules[0].SinceMS, localMS(t, "2026-11-20T00:00:00"); got != want {
		t.Fatalf("date since = %d, want local midnight %d", got, want)
	}
	if got, want := rs.Rules[1].SinceMS, time.Date(2026, 11, 20, 10, 0, 0, 0, time.UTC).UnixMilli(); got != want {
		t.Fatalf("offset since = %d, want %d", got, want)
	}
}

func TestSinceRejectsNonDate(t *testing.T) {
	_, err := Parse([]byte(baseRules+`
[[rule]]
title = 'a'
category = "Code"
since = "tomorrow"
`), nil)
	if err == nil || !strings.Contains(err.Error(), "since") {
		t.Fatalf("err = %v, want a since error", err)
	}
}

func TestSinceInLearnedFile(t *testing.T) {
	rs := mustParse(t, baseRules, `
[[rule]]
title = 'a'
category = "Code"
since = 2026-11-20T14:05:00
`)
	if rs.Learned[0].SinceMS == 0 {
		t.Fatal("learned rule lost its since")
	}
}

// The same title gets one label before since and another after it, in
// one labelling pass over one Ruleset (the memo must not carry the first
// answer across the date).
func TestSinceSameTitleTwoLabelsOnePass(t *testing.T) {
	rs := mustParse(t, sinceRules, "")
	at := localMS(t, "2026-11-20T14:05:00")
	mk := func(id, start int64) SpanView {
		return SpanView{ID: id, Start: start, End: start + 60_000, BundleID: "b", App: "B", Title: sp("acme board")}
	}
	views := []SpanView{mk(1, at-120_000), mk(2, at), mk(3, at-60_000), mk(4, at+3_600_000)}
	got := rs.LabelAll(views, nil)
	want := []string{"Entertainment", "Code", "Entertainment", "Code"}
	for i := range want {
		if got[i].Category != want[i] {
			t.Errorf("view %d: %q, want %q", views[i].ID, got[i].Category, want[i])
		}
	}
	if got[1].CategoryRule != "rules.toml#1" || got[0].CategoryRule != "rules.toml#2" {
		t.Errorf("refs %q %q", got[1].CategoryRule, got[0].CategoryRule)
	}
}

// With no dated rules every view is in era 0, so the memo key is what it
// was and one entry serves a title all day.
func TestSinceNoDatedRulesOneMemoEntry(t *testing.T) {
	rs := mustParse(t, labelRules, "")
	for i := int64(0); i < 50; i++ {
		rs.Label(SpanView{ID: i, Start: i * 3_600_000, End: i*3_600_000 + 1000, BundleID: "term", App: "T", Title: sp("tock │ x")}, nil)
	}
	rs.mu.Lock()
	n := len(rs.memo)
	rs.mu.Unlock()
	if n != 1 {
		t.Fatalf("memo has %d entries, want 1", n)
	}
}

func TestSinceStatsSplitByEra(t *testing.T) {
	rs := mustParse(t, sinceRules, "")
	at := localMS(t, "2026-11-20T14:05:00")
	views := []SpanView{
		{ID: 1, Start: at - 600_000, End: at - 300_000, BundleID: "b", App: "B", Title: sp("acme")},
		{ID: 2, Start: at + 1000, End: at + 61_000, BundleID: "b", App: "B", Title: sp("acme")},
	}
	st := rs.Stats(views, rs.LabelAll(views, nil))
	if st[0].MatchedMS != 60_000 {
		t.Fatalf("dated rule matched %d ms, want 60000 (only after since)", st[0].MatchedMS)
	}
	if st[1].MatchedMS != 360_000 {
		t.Fatalf("undated rule matched %d ms, want 360000", st[1].MatchedMS)
	}
}

func TestRenderRuleWritesSinceRoundTrip(t *testing.T) {
	at := localMS(t, "2026-11-20T14:05:00")
	text := renderRule(Rule{Title: "acme", Category: "Code", SinceMS: at}, "from template")
	if !strings.Contains(text, "since = 2026-11-20T14:05:00\n") {
		t.Fatalf("no since line:\n%s", text)
	}
	rs := mustParse(t, baseRules+"\n"+text, "")
	if rs.Rules[0].SinceMS != at {
		t.Fatalf("round trip %d, want %d", rs.Rules[0].SinceMS, at)
	}
}
