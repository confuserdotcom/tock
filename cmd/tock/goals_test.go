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
