package ask

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"tock/internal/classify"
)

const rules = `
[[category]]
name = "Code"
kind = "deep"

[[category]]
name = "Video"
kind = "distract"

[[project]]
name = "tock"
`

func ruleset(t *testing.T) *classify.Ruleset {
	rs, err := classify.Parse([]byte(rules), nil)
	if err != nil {
		t.Fatal(err)
	}
	return rs
}

var loc = time.FixedZone("T", 2*3600)
var today = time.Date(2026, 10, 10, 15, 0, 0, 0, loc) // a Saturday

func TestPayload(t *testing.T) {
	b, err := Payload("  how much   youtube\nlast week? ", today, ruleset(t))
	if err != nil {
		t.Fatal(err)
	}
	s := string(b)
	if !strings.Contains(s, `"today": "2026-10-10 Saturday"`) || !strings.Contains(s, `"question": "how much youtube last week?"`) ||
		!strings.Contains(s, `"name": "Video"`) || !strings.Contains(s, `"tock"`) {
		t.Fatalf("payload:\n%s", s)
	}
	if _, err := Payload(" ", today, ruleset(t)); err == nil {
		t.Fatal("empty question accepted")
	}
	if _, err := Payload(strings.Repeat("x", 501), today, ruleset(t)); err == nil {
		t.Fatal("long question accepted")
	}
}

func TestParseAndBounds(t *testing.T) {
	rs := ruleset(t)
	q, err := Parse(json.RawMessage(`{"range":"last_week","group_by":"day","site":"youtube.com","category":""}`), rs)
	if err != nil || q.Site == nil || q.Category != nil || q.Limit != 10 {
		t.Fatalf("Parse = %+v, %v", q, err)
	}
	f, to := q.Bounds(today)
	if f.Format("2006-01-02") != "2026-09-28" || to.Format("2006-01-02") != "2026-10-05" {
		t.Fatalf("last week = %s %s", f, to)
	}
	for _, c := range []struct{ rng, from, to string }{
		{`{"range":"today","group_by":"total"}`, "2026-10-10", "2026-10-11"},
		{`{"range":"yesterday","group_by":"total"}`, "2026-10-09", "2026-10-10"},
		{`{"range":"this_week","group_by":"total"}`, "2026-10-05", "2026-10-12"},
		{`{"range":"last_month","group_by":"total"}`, "2026-09-01", "2026-10-01"},
		{`{"range":"last_days","days":3,"group_by":"total"}`, "2026-10-08", "2026-10-11"},
		{`{"range":"dates","from":"2026-10-01","to":"2026-10-02","group_by":"total"}`, "2026-10-01", "2026-10-03"},
	} {
		q, err := Parse(json.RawMessage(c.rng), rs)
		if err != nil {
			t.Fatalf("%s: %v", c.rng, err)
		}
		f, to := q.Bounds(today)
		if f.Format("2006-01-02") != c.from || to.Format("2006-01-02") != c.to {
			t.Errorf("%s = %s – %s", c.rng, f.Format("2006-01-02"), to.Format("2006-01-02"))
		}
	}
	for _, bad := range []string{`{"range":"soon","group_by":"total"}`, `{"range":"today","group_by":"mood"}`,
		`{"range":"today","group_by":"total","category":"Nope"}`, `{"range":"last_days","days":0,"group_by":"total"}`,
		`{"range":"today","group_by":"total","unsupported":"not about time"}`, `not json`} {
		if _, err := Parse(json.RawMessage(bad), rs); err == nil {
			t.Errorf("Parse(%s) accepted", bad)
		}
	}
}

func TestRun(t *testing.T) {
	day := time.Date(2026, 10, 10, 0, 0, 0, 0, loc)
	at := func(h int) int64 { return day.Add(time.Duration(h) * time.Hour).UnixMilli() }
	u := func(s string) *string { return &s }
	vs := []classify.SpanView{
		{Start: at(9), End: at(11), App: "WezTerm", BundleID: "term"},
		{Start: at(11), End: at(12), App: "Arc", BundleID: "arc", URL: u("www.youtube.com/watch")},
		{Start: at(12), End: at(13), App: "Arc", BundleID: "arc", URL: u("m.youtube.com/x")},
		{Start: at(13), End: at(14), App: "Idle", BundleID: "tock.idle"},
	}
	ls := []classify.Label{{Category: "Code", Kind: "deep", Project: "tock"}, {Category: "Video", Kind: "distract"}, {Category: "Video", Kind: "distract"}, {}}
	q, _ := Parse(json.RawMessage(`{"range":"today","group_by":"site","site":"youtube.com"}`), ruleset(t))
	f, to := q.Bounds(today)
	r := Run(q, f, to, vs, ls)
	if r.MS != 2*3600_000 || r.TrackedMS != 4*3600_000 || len(r.Rows) != 2 || r.Rows[0].Name != "m.youtube.com" {
		t.Fatalf("Run = %+v", r)
	}
	lines := strings.Join(Lines(q, r), "\n")
	if !strings.Contains(lines, "youtube.com, today: 2h00 (50% of 4h00 tracked)") {
		t.Fatalf("lines:\n%s", lines)
	}
	q, _ = Parse(json.RawMessage(`{"range":"today","group_by":"kind"}`), ruleset(t))
	if r := Run(q, f, to, vs, ls); len(r.Rows) != 2 || r.Rows[0].Name != "deep" || r.MS != r.TrackedMS {
		t.Fatalf("by kind = %+v", r)
	}
}
