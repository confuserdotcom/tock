package main

import (
	"strings"
	"testing"
)

func TestBudgetCmd(t *testing.T) {
	te := newTestEnv(t, testRules)
	te.seed(t, daySpans()...)
	if code, out := te.run(t, "", "budget"); code != 0 || !strings.Contains(out, "no project has a budget") {
		t.Fatalf("none: %d %s", code, out)
	}
	if code, out := te.run(t, "", "project", "set", "tock", "--budget", "10h", "--budget-since", "2026-10-10"); code != 0 || !strings.Contains(out, "budget 10h from 2026-10-10") {
		t.Fatalf("set: %d %s%s", code, out, te.errb)
	}
	code, out := te.run(t, "", "budget")
	if code != 0 || !strings.Contains(out, "tock    1h40 / 10h      16%  ███░░░") || !strings.Contains(out, "/day · runs out ") {
		t.Fatalf("budget: %d\n%s", code, out)
	}
	if code, out := te.run(t, "", "project", "set", "tock", "--budget", "1h"); code != 0 {
		t.Fatalf("lower: %d %s", code, out)
	}
	if _, out := te.run(t, "", "budget"); !strings.Contains(out, "over by 40m") {
		t.Fatalf("over:\n%s", out)
	}
	if code, _ := te.run(t, "", "project", "set", "tock", "--budget", "lots"); code != 2 {
		t.Fatal("bad budget must be usage")
	}
	te.run(t, "", "project", "set", "tock", "--budget", "none")
	if _, out := te.run(t, "", "budget"); !strings.Contains(out, "no project has a budget") {
		t.Fatalf("dropped:\n%s", out)
	}
}
