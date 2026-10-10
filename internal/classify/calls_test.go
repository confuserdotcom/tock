package classify

import (
	"fmt"
	"strings"
	"testing"

	"tock/internal/span"
)

const callRules = baseRules + `
[[category]]
name = "Meeting"
kind = "shallow"

[calls]
category = "meeting"

[[rule]]
bundle = ["term"]
category = "Code"

[[rule]]
bundle = ["term"]
project = "tock"
`

func pieces(vs []SpanView) string {
	var b []string
	for _, v := range vs {
		s := fmt.Sprintf("%d:%d-%d %s", v.ID, v.Start, v.End, v.BundleID)
		if v.InCall {
			s += " call"
		}
		b = append(b, s)
	}
	return strings.Join(b, ", ")
}

func TestMarkCalls(t *testing.T) {
	title := sp("t")
	for _, c := range []struct {
		name  string
		views []SpanView
		calls []Interval
		want  string
	}{
		{"inside", []SpanView{{ID: 1, Start: 0, End: 100, BundleID: "a", Title: title}}, []Interval{{30, 60}},
			"1:0-30 a, 1:30-60 a call, 1:60-100 a"},
		{"idle becomes call", []SpanView{{ID: 2, Start: 0, End: 100, BundleID: span.IdleBundleID}}, []Interval{{-50, 200}},
			"2:0-100 tock.call call"},
		{"two calls", []SpanView{{ID: 3, Start: 0, End: 100, BundleID: "a"}}, []Interval{{10, 20}, {50, 60}},
			"3:0-10 a, 3:10-20 a call, 3:20-50 a, 3:50-60 a call, 3:60-100 a"},
		{"across views", []SpanView{{ID: 4, Start: 0, End: 50, BundleID: "a"}, {ID: 5, Start: 50, End: 100, BundleID: "b"}}, []Interval{{40, 70}},
			"4:0-40 a, 4:40-50 a call, 5:50-70 b call, 5:70-100 b"},
		{"outside", []SpanView{{ID: 6, Start: 0, End: 100, BundleID: "a"}}, []Interval{{100, 200}},
			"6:0-100 a"},
		{"no calls", []SpanView{{ID: 7, Start: 0, End: 100, BundleID: "a"}}, nil,
			"7:0-100 a"},
	} {
		if got := pieces(MarkCalls(c.views, c.calls)); got != c.want {
			t.Errorf("%s: got %s, want %s", c.name, got, c.want)
		}
	}
	vs := MarkCalls([]SpanView{{ID: 2, Start: 0, End: 100, BundleID: span.IdleBundleID, App: span.IdleApp}}, []Interval{{0, 100}})
	if vs[0].App != span.CallApp || vs[0].Title != nil || vs[0].URL != nil {
		t.Fatalf("call view = %+v", vs[0])
	}
}

func TestCallLabel(t *testing.T) {
	rs, err := Parse([]byte(callRules), nil)
	if err != nil {
		t.Fatal(err)
	}
	if rs.CallCategory != "Meeting" {
		t.Fatalf("CallCategory = %q", rs.CallCategory)
	}
	v := SpanView{ID: 1, Start: 0, End: 1, BundleID: "term", App: "T", InCall: true}
	l := rs.Label(v, nil)
	if l.Category != "Meeting" || l.Kind != KindShallow || l.CategorySource != SourceCall || l.CategoryRule != "" || l.Project != "tock" {
		t.Fatalf("in call: %+v", l)
	}
	v.InCall = false
	if l := rs.Label(v, nil); l.Category != "Code" {
		t.Fatalf("out of call (memo must not leak): %+v", l)
	}
	v.InCall = true
	if l := rs.Label(v, &Override{Category: "Entertainment"}); l.Category != "Entertainment" || l.CategorySource != SourceManual {
		t.Fatalf("override: %+v", l)
	}
	call := SpanView{ID: 2, Start: 0, End: 1, BundleID: span.CallBundleID, App: span.CallApp, InCall: true}
	if l := rs.Label(call, nil); l.Category != "Meeting" {
		t.Fatalf("call view: %+v", l)
	}
	if l := rs.Label(call, &Override{Project: "tock"}); l.Project != "tock" || l.Category != "Meeting" {
		t.Fatalf("call view override: %+v", l)
	}

	plain, err := Parse([]byte(strings.Replace(callRules, "[calls]\ncategory = \"meeting\"\n", "", 1)), nil)
	if err != nil {
		t.Fatal(err)
	}
	if plain.CallCategory != "" {
		t.Fatal("no [calls] must leave CallCategory empty")
	}
	if l := plain.Label(SpanView{ID: 1, BundleID: "term", InCall: true}, nil); l.Category != "Code" {
		t.Fatalf("without [calls]: %+v", l)
	}
}

func TestCallsUnknownCategory(t *testing.T) {
	_, err := Parse([]byte(baseRules+"[calls]\ncategory = \"Nope\"\n"), nil)
	if err == nil || !strings.Contains(err.Error(), `[calls]: unknown category "Nope"`) {
		t.Fatalf("err = %v", err)
	}
	if _, err := Parse([]byte(baseRules+"[calls]\nother = 1\n"), nil); err == nil {
		t.Fatal("unknown [calls] key must fail")
	}
}
