package report

import (
	"testing"
	"time"

	"tock/internal/classify"
	"tock/internal/span"
)

// zone is fixed so day and hour boundaries are deterministic.
var zone = time.FixedZone("T", 2*3600)

// day0 is local midnight of the test day.
var day0 = time.Date(2026, 10, 10, 0, 0, 0, 0, zone)

func sp(s string) *string { return &s }

// hm returns unix ms of "HH:MM" on day0.
func hm(t *testing.T, s string) int64 {
	t.Helper()
	tm, err := time.ParseInLocation("2006-01-02 15:04", "2026-10-10 "+s, zone)
	if err != nil {
		t.Fatal(err)
	}
	return tm.UnixMilli()
}

// sv is a span view; kind "idle" makes an idle span.
type sv struct {
	id              int64
	start, end      string
	app, title, url string
	cat, kind, proj string
	private         bool
}

func build(t *testing.T, spans ...sv) ([]classify.SpanView, []classify.Label) {
	t.Helper()
	var vs []classify.SpanView
	var ls []classify.Label
	for _, s := range spans {
		v := classify.SpanView{ID: s.id, Start: hm(t, s.start), End: hm(t, s.end), BundleID: s.app, App: s.app, Private: s.private}
		if s.kind == "idle" {
			v.BundleID, v.App = span.IdleBundleID, "Idle"
		}
		if s.title != "" {
			v.Title = sp(s.title)
		}
		if s.url != "" {
			v.URL = sp(s.url)
		}
		vs = append(vs, v)
		l := classify.Label{Category: s.cat, Project: s.proj}
		if s.kind != "idle" {
			l.Kind = s.kind
		}
		ls = append(ls, l)
	}
	return vs, ls
}
