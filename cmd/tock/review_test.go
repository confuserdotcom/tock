package main

import (
	"strings"
	"testing"
	"time"

	"tock/internal/store"
)

func TestReview(t *testing.T) {
	te := newTestEnv(t, goalRules)
	te.seed(t, daySpans()...)
	code, out := te.run(t, "", "review")
	want := "Week of Mon 5 Oct\n" +
		"tracked 2h54 · focus 2h14 · score 74 avg · 1 workday · 0.7 switches/h\n" +
		"\n" +
		"goals\n" +
		"  Code ≥2h/day                  met 1 of 1 day\n" +
		"  project tock ≥10h/week        1h40/10h00 short 8h20\n" +
		"  Entertainment ≤10m/day        over 1 of 1 day\n" +
		"\n" +
		"top distractions\n" +
		"  youtube.com         12m\n" +
		"\n" +
		"best day  Sat 10 Oct · score 74\n"
	if code != 0 || out != want {
		t.Fatalf("code %d\n%s\nwant\n%s", code, out, want)
	}
}

func TestReviewPlan(t *testing.T) {
	te := newTestEnv(t, testRules)
	te.seed(t, daySpans()[:7]...) // no open span: the clock moves on below
	if code, out := te.run(t, "", "review", "--plan", "  ship tier 2  "); code != 0 ||
		!strings.HasSuffix(out, "\nsaved plan for the week of Mon 12 Oct\n") {
		t.Fatalf("--plan: code %d\n%s", code, out)
	}
	st, err := store.Open(te.dbPath())
	if err != nil {
		t.Fatal(err)
	}
	if p, _ := st.Plan("2026-10-12"); p != "ship tier 2" {
		t.Fatalf("plan = %q", p)
	}
	st.Close()

	// Interactive: the answer replaces the plan; an empty answer saves nothing.
	te.Interactive = true
	if code, out := te.run(t, "write\x07 the report\n", "review"); code != 0 ||
		!strings.HasSuffix(out, "\nnext week's focus (enter to skip): saved plan for the week of Mon 12 Oct\n") {
		t.Fatalf("prompt: code %d\n%s", code, out)
	}
	if code, out := te.run(t, "\n", "review"); code != 0 || strings.Contains(out, "saved") {
		t.Fatalf("empty answer: code %d\n%s", code, out)
	}
	te.Interactive = false

	// On a Wednesday the default is last week; --week picks the week, which
	// shows the plan saved for it.
	te.now = time.Date(2026, 10, 14, 9, 0, 0, 0, testZone)
	if code, out := te.run(t, "", "review"); code != 0 || !strings.HasPrefix(out, "Week of Mon 5 Oct\n") {
		t.Fatalf("weekday default: code %d\n%s", code, out)
	}
	if code, out := te.run(t, "", "review", "--week", "2026-10-13"); code != 0 ||
		!strings.HasPrefix(out, "Week of Mon 12 Oct\ntracked 0s · focus 0s · no workdays\nplan: write? the report\n\ntop distractions  none\n") {
		t.Fatalf("this week: code %d\n%s", code, out)
	}
	if code, _ := te.run(t, "", "review", "--week", "13/10"); code != 2 {
		t.Fatalf("bad week: code %d", code)
	}
}
