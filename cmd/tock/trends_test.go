package main

import (
	"strings"
	"testing"
	"time"
)

func TestTrends(t *testing.T) {
	te := newTestEnv(t, testRules)
	te.seed(t, daySpans()[:7]...) // no open span
	te.now = testNow.AddDate(0, 1, 0)
	code, out := te.run(t, "", "trends", "--month", "2026-10")
	want := "October 2026 vs September\n" +
		"tracked     2h44  new\n" +
		"focus       2h14  new\n" +
		"score     74 avg · 1 workday   vs —   ·········▆·····················\n" +
		"switches  0.7/h   vs —\n" +
		"\n" +
		"Code               2h14  new\n" +
		"Entertainment       12m  new\n" +
		"Unclassified        18m  new\n"
	if code != 0 || out != want {
		t.Fatalf("code %d\n%s\nwant\n%s", code, out, want)
	}
	code, out = te.run(t, "", "trends")
	if code != 0 || !strings.HasPrefix(out, "November 2026 · 1–10 vs October 1–10\ntracked       0s   −2h44  gone\n") ||
		!strings.Contains(out, "score     —   vs 74   ··········\n") || !strings.Contains(out, "Code                 0s   −2h14  gone\n") {
		t.Fatalf("code %d\n%s", code, out)
	}
	te.now = time.Date(2026, 10, 31, 12, 0, 0, 0, testZone)
	if code, out := te.run(t, "", "trends"); code != 0 || !strings.HasPrefix(out, "October 2026 · 1–31 vs September 1–30\n") {
		t.Fatalf("31st: code %d\n%s", code, out)
	}
}

func TestTrendsErrors(t *testing.T) {
	te := newTestEnv(t, testRules)
	te.seed(t, daySpans()[:7]...)
	if code, _ := te.run(t, "", "trends", "--month", "2026-13"); code != 2 {
		t.Fatalf("bad month: code %d", code)
	}
	if code, _ := te.run(t, "", "trends", "--month", "2026-11"); code != 1 ||
		te.errb.String() != "tock: that month has not started\n" {
		t.Fatalf("future: code %d %q", code, te.errb.String())
	}
}
