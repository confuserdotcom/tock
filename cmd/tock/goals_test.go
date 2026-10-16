package main

import (
	"strconv"
	"strings"
	"testing"

	"tock/internal/store"
)

const goalRules = testRules + `
[[goal]]
category = "code"
at_least = "2h"

[[goal]]
project = "tock"
at_least = "10h"
per = "week"

[[goal]]
category = "entertainment"
at_most = "10m"
`

func TestTodayGoals(t *testing.T) {
	te := newTestEnv(t, goalRules)
	te.seed(t, daySpans()...)
	code, out := te.run(t, "", "today")
	if code != 0 {
		t.Fatalf("code %d err %q", code, te.errb.String())
	}
	if !strings.Contains(out, "2 switches\ngoals  Code 2h14/2h00 · project tock 1h40/10h00 wk · Entertainment 12m/10m over\nCode ") {
		t.Fatalf("today output:\n%s", out)
	}
}

func TestStatusGoals(t *testing.T) {
	te := newTestEnv(t, goalRules)
	te.seed(t, daySpans()...)
	st, err := store.Open(te.dbPath())
	if err != nil {
		t.Fatal(err)
	}
	hb := testNow.UnixMilli() - 5_000
	if err := st.SetState(store.KeyHeartbeat, strconv.FormatInt(hb, 10), hb); err != nil {
		t.Fatal(err)
	}
	st.Close()
	if code, out := te.run(t, "", "status"); code != 0 || out != "2h54 · score 74 · focus 2h14 · goals 1/3 · over Entertainment · now unlabelled\n" {
		t.Fatalf("code %d out %q", code, out)
	}
}

func TestGoalSetListRemove(t *testing.T) {
	te := newTestEnv(t, goalRules)
	te.seed(t, daySpans()...)
	if code, out := te.run(t, "", "goal", "set", "tock", "12h/week"); code != 0 || out != "goal project tock ≥12h/week\n" {
		t.Fatalf("set: code %d %q %q", code, out, te.errb.String())
	}
	if code, out := te.run(t, "", "goal", "set", "comm", "30m", "--max"); code != 0 || out != "goal Communication ≤30m/day\n" {
		t.Fatalf("limit: code %d %q %q", code, out, te.errb.String())
	}
	code, out := te.run(t, "", "goal")
	if code != 0 || !strings.Contains(out, "project tock ≥12h/week") || strings.Contains(out, "10h/week") ||
		!strings.Contains(out, "Communication ≤30m/day") {
		t.Fatalf("list: code %d\n%s", code, out)
	}
	if code, out := te.run(t, "", "goal", "rm", "tock", "week"); code != 0 || out != "removed the goal on project tock\n" {
		t.Fatalf("rm: code %d %q %q", code, out, te.errb.String())
	}
	if code, _ := te.run(t, "", "goal", "rm", "tock", "week"); code != 1 || !strings.Contains(te.errb.String(), "no goal project tock ≥ per week") {
		t.Fatalf("rm twice: code %d %q", code, te.errb.String())
	}
	if code, _ := te.run(t, "", "goal", "set", "tock", "200h/week"); code != 1 || !strings.Contains(te.errb.String(), "not between 1m and 168h") {
		t.Fatalf("too long: code %d %q", code, te.errb.String())
	}
	if code, _ := te.run(t, "", "goal", "set", "nope", "1h"); code != 1 {
		t.Fatalf("unknown label: code %d", code)
	}
}
