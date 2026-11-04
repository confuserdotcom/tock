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
	// A call is context: labelled activity keeps its label.
	v := SpanView{ID: 1, Start: 0, End: 1, BundleID: "term", App: "T", InCall: true}
	if l := rs.Label(v, nil); l.Category != "Code" || l.CategorySource != SourceRule || l.Project != "tock" {
		t.Fatalf("labelled activity in call: %+v", l)
	}
	// Unlabelled activity during a call is the call.
	u := SpanView{ID: 3, Start: 0, End: 1, BundleID: "other", App: "O", InCall: true}
	l := rs.Label(u, nil)
	if l.Category != "Meeting" || l.Kind != KindShallow || l.CategorySource != SourceCall || l.CategoryRule != "" {
		t.Fatalf("unlabelled in call: %+v", l)
	}
	u.InCall = false
	if l := rs.Label(u, nil); l.Category != "" {
		t.Fatalf("out of call (memo must not leak): %+v", l)
	}
	// A call app or a meeting site in front is the call, whatever its rule.
	for _, c := range []SpanView{
		{ID: 4, BundleID: "com.apple.FaceTime", App: "FaceTime", InCall: true},
		{ID: 5, BundleID: "arc", App: "Arc", URL: sp("meet.google.com/abc-defg-hij"), InCall: true},
		{ID: 6, BundleID: "arc", App: "Arc", URL: sp("us02web.zoom.us/j/1"), InCall: true},
	} {
		if l := rs.Label(c, nil); l.Category != "Meeting" || l.CategorySource != SourceCall {
			t.Fatalf("call app %s: %+v", c.App, l)
		}
	}
	if l := rs.Label(SpanView{ID: 7, BundleID: "com.apple.FaceTime", App: "FaceTime"}, nil); l.CategorySource == SourceCall {
		t.Fatalf("call app out of a call: %+v", l)
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

// TestCallKeepsDistraction: a distraction stays one during a call; a video
// chat site with the camera on is still Entertainment.
func TestCallKeepsDistraction(t *testing.T) {
	rs, err := Parse([]byte(callRules+`
[[category]]
name = "Fun"
kind = "distract"

[[rule]]
url = "^umingle\\.com"
category = "Fun"
`), nil)
	if err != nil {
		t.Fatal(err)
	}
	v := SpanView{ID: 1, Start: 0, End: 1, BundleID: "arc", App: "Arc", URL: sp("umingle.com/chat"), InCall: true}
	if l := rs.Label(v, nil); l.Category != "Fun" || l.CategorySource != SourceRule {
		t.Fatalf("distraction in call: %+v", l)
	}
}

// TestProjectNeedsItsCategory: a rule's project filed under a category is
// dropped when the label's category is another one.
func TestProjectNeedsItsCategory(t *testing.T) {
	rs, err := Parse([]byte(baseRules+`
[[category]]
name = "Fun"
kind = "distract"

[[category]]
name = "Learn"
kind = "deep"

[[project]]
name = "Video"
category = "Fun"

[[rule]]
url = "^yt\\.com"
title = "lecture"
category = "Learn"

[[rule]]
url = "^yt\\.com"
category = "Fun"
project = "Video"
`), nil)
	if err != nil {
		t.Fatal(err)
	}
	v := SpanView{ID: 1, Start: 0, End: 1, BundleID: "arc", App: "Arc", URL: sp("yt.com/w"), Title: sp("MIT lecture 3")}
	if l := rs.Label(v, nil); l.Category != "Learn" || l.Project != "" {
		t.Fatalf("lecture: %+v", l)
	}
	v.Title = sp("cats")
	if l := rs.Label(v, nil); l.Category != "Fun" || l.Project != "Video" {
		t.Fatalf("cats: %+v", l)
	}
}

// TestCallApps: [calls] apps replaces the built-in call apps; [] means none.
func TestCallApps(t *testing.T) {
	custom := strings.Replace(callRules, "[calls]\ncategory = \"meeting\"\n",
		"[calls]\ncategory = \"meeting\"\napps = [\"chat.app\"]\n", 1) +
		"[[rule]]\nbundle = [\"com.apple.FaceTime\"]\ncategory = \"Code\"\n"
	rs, err := Parse([]byte(custom), nil)
	if err != nil {
		t.Fatal(err)
	}
	if l := rs.Label(SpanView{ID: 1, BundleID: "chat.app", App: "Chat", InCall: true}, nil); l.CategorySource != SourceCall {
		t.Fatalf("listed app: %+v", l)
	}
	if l := rs.Label(SpanView{ID: 2, BundleID: "com.apple.FaceTime", App: "FaceTime", InCall: true}, nil); l.Category != "Code" {
		t.Fatalf("unlisted app keeps its rule: %+v", l)
	}
	// Meeting sites stay call time whatever the list says.
	if l := rs.Label(SpanView{ID: 3, BundleID: "arc", App: "Arc", URL: sp("meet.google.com/x"), InCall: true}, nil); l.CategorySource != SourceCall {
		t.Fatalf("meeting site: %+v", l)
	}
	if _, err := Parse([]byte(strings.Replace(custom, `apps = ["chat.app"]`, `apps = [""]`, 1)), nil); err == nil {
		t.Fatal("empty app id must be rejected")
	}
}

// TestMemoBounded: the label memo starts over instead of growing for ever.
func TestMemoBounded(t *testing.T) {
	rs, err := Parse([]byte(callRules), nil)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < memoMax+10; i++ {
		rs.Label(SpanView{ID: 1, BundleID: "term", App: "T", Title: sp(fmt.Sprint(i))}, nil)
	}
	if n := len(rs.memo); n > memoMax {
		t.Fatalf("memo holds %d", n)
	}
}
