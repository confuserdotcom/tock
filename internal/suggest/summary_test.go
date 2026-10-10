package suggest

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"tock/internal/classify"
	"tock/internal/config"
	"tock/internal/report"
)

func TestPrepareSummary(t *testing.T) {
	loc := time.FixedZone("T", 2*3600)
	day := time.Date(2026, 10, 10, 0, 0, 0, 0, loc)
	at := func(h, m int) int64 { return day.Add(time.Duration(h)*time.Hour + time.Duration(m)*time.Minute).UnixMilli() }
	s := func(x string) *string { return &x }
	views := []classify.SpanView{
		{Start: at(9, 0), End: at(9, 50), BundleID: "term", App: "WezTerm", Title: s("tock │ go test ./... 3")},
		{Start: at(9, 50), End: at(10, 0), BundleID: "arc", App: "Arc", URL: s("github.com/x/tock/pull/1")},
		{Start: at(10, 0), End: at(10, 5), BundleID: "com.apple.mail", App: "Mail", Title: s("Re: salary")},
		{Start: at(10, 5), End: at(10, 20), BundleID: "arc", App: "Arc", Private: true},
	}
	labels := make([]classify.Label, len(views))
	for i := range labels {
		labels[i] = classify.Label{Category: "Code", Kind: "deep", Project: "tock"}
	}
	blocks := report.Blocks(views, labels)
	b := PrepareSummary(day, blocks, views, labels, filter(t, config.DefaultLLM()))
	got := string(b.Payload)
	for _, leak := range []string{"salary", "2026", "Mail"} {
		if strings.Contains(got, leak) {
			t.Errorf("payload leaks %q:\n%s", leak, got)
		}
	}
	var p SummaryPayload
	if err := json.Unmarshal(b.Payload, &p); err != nil {
		t.Fatal(err)
	}
	if p.Day != "Saturday" || len(p.Blocks) != 1 || p.Blocks[0].From != "09:00" || p.Blocks[0].Label != "tock (Code), deep" ||
		len(p.Blocks[0].Items) != 2 || p.Blocks[0].Withheld != 20 || b.Starts["b1"] != at(9, 0) {
		t.Fatalf("payload = %s", got)
	}
	if p.Blocks[0].Items[1].Host != "github.com/x" {
		t.Fatalf("host item = %+v", p.Blocks[0].Items[1])
	}

	sum, err := ParseSummary(json.RawMessage(`{"day":"You worked on tock.\n\u0007Mostly tests.","blocks":[{"id":"b1","summary":"Tests"},{"id":"b9","summary":"x"}]}`), b.Starts)
	if err != nil || sum.Day != "You worked on tock. ?Mostly tests." || len(sum.Blocks) != 1 || sum.Blocks[at(9, 0)] != "Tests" {
		t.Fatalf("ParseSummary = %+v, %v", sum, err)
	}
	if _, err := ParseSummary(json.RawMessage(`{"day":" "}`), b.Starts); err == nil {
		t.Fatal("empty day accepted")
	}
}
