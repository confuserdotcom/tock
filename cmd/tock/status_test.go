package main

import (
	"strconv"
	"testing"

	"tock/internal/store"
)

func TestStatus(t *testing.T) {
	te := newTestEnv(t, testRules)
	te.seed(t, daySpans()...)
	if code, out := te.run(t, "", "status"); code != 0 || out != "trackd down · 2h54\n" {
		t.Fatalf("down: code %d out %q", code, out)
	}
	st, err := store.Open(te.dbPath())
	if err != nil {
		t.Fatal(err)
	}
	hb := testNow.UnixMilli() - 5_000
	if err := st.SetState(store.KeyHeartbeat, strconv.FormatInt(hb, 10), hb); err != nil {
		t.Fatal(err)
	}
	st.Close()
	if code, out := te.run(t, "", "status"); code != 0 || out != "2h54 · score 74 · focus 2h14 · now unlabelled\n" {
		t.Fatalf("up: code %d out %q", code, out)
	}
	if code, _ := te.run(t, "", "status", "x"); code != 2 {
		t.Fatalf("extra argument: code %d", code)
	}
}

func TestStatusNeedsData(t *testing.T) {
	te := newTestEnv(t, testRules)
	if code, _ := te.run(t, "", "status"); code != 1 || te.errb.String() != "tock: no data yet: is trackd running? (make install)\n" {
		t.Fatalf("no db: code %d err %q", code, te.errb.String())
	}
}
