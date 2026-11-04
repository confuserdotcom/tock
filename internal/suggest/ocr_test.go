package suggest

import (
	"strings"
	"testing"
	"time"

	"tock/internal/classify"
	"tock/internal/config"
	"tock/internal/report"
)

// TestNoScreenWordsInPayload: words read on screen never leave the machine,
// in the inbox batch or the day summary (OCR-05).
func TestNoScreenWordsInPayload(t *testing.T) {
	day := time.Date(2026, 10, 10, 0, 0, 0, 0, time.UTC)
	at := func(h int) int64 { return day.Add(time.Duration(h) * time.Hour).UnixMilli() }
	title := "zsh"
	views := []classify.SpanView{
		{ID: 1, Start: at(9), End: at(10), BundleID: "term", App: "WezTerm", Title: &title, Screen: []string{"zorglubword"}},
	}
	labels := []classify.Label{{Category: "Code", Kind: "deep"}}
	f := filter(t, config.DefaultLLM())
	sum := PrepareSummary(day, report.Blocks(views, labels), views, labels, f)
	items := classify.Group(views, func(int) bool { return true })
	rs, err := classify.Parse([]byte("[[category]]\nname = \"Code\"\nkind = \"deep\"\n"), nil)
	if err != nil {
		t.Fatal(err)
	}
	in := Prepare(items, nil, f, rs)
	for name, b := range map[string][]byte{"summary": sum.Payload, "inbox": in.Payload} {
		if len(b) == 0 {
			t.Fatalf("%s payload empty", name)
		}
		if strings.Contains(string(b), "zorglubword") {
			t.Errorf("%s payload leaks a screen word:\n%s", name, b)
		}
	}
}
