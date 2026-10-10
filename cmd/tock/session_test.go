package main

import (
	"strconv"
	"testing"
	"time"

	"tock/internal/store"
)

func heartbeat(t *testing.T, te *tenv) {
	t.Helper()
	st, err := store.Open(te.dbPath())
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	hb := te.now.UnixMilli()
	if err := st.SetState(store.KeyHeartbeat, strconv.FormatInt(hb, 10), hb); err != nil {
		t.Fatal(err)
	}
}

func TestSessionCommands(t *testing.T) {
	te := newTestEnv(t, testRules)
	te.seed(t, daySpans()...)
	if code, out := te.run(t, "", "session"); code != 0 || out != "no session\n" {
		t.Fatalf("show none: %d %q", code, out)
	}
	code, out := te.run(t, "", "session", "start")
	if code != 0 || out != "session started: 25 min, ends 15:25\n" || te.errb.String() != "warning: trackd not running, no alerts\n" {
		t.Fatalf("start: %d %q %q", code, out, te.errb.String())
	}
	if code, _ := te.run(t, "", "session", "start", "50"); code != 1 || te.errb.String() != "tock: session already running\n" {
		t.Fatalf("double start: %d %q", code, te.errb.String())
	}
	te.now = testNow.Add(5 * time.Minute)
	if code, out := te.run(t, "", "session"); code != 0 || out != "focus 20:00 left of 25m · deep 0% · 0 interruptions\n" {
		t.Fatalf("show: %d %q", code, out)
	}
	te.now = testNow.Add(10 * time.Minute)
	if code, out := te.run(t, "", "session", "stop"); code != 0 || out != "stopped: 15:00  manual   10m  deep   0%  0 int\n" {
		t.Fatalf("stop: %d %q", code, out)
	}
	if code, _ := te.run(t, "", "session", "stop"); code != 1 || te.errb.String() != "tock: no session\n" {
		t.Fatalf("stop none: %d %q", code, te.errb.String())
	}
}

func TestSessionStartLengthWithTrackdUp(t *testing.T) {
	te := newTestEnv(t, testRules)
	te.seed(t, daySpans()...)
	heartbeat(t, te)
	if code, out := te.run(t, "", "session", "start", "50"); code != 0 || out != "session started: 50 min, ends 15:50\n" || te.errb.String() != "" {
		t.Fatalf("start 50: %d %q %q", code, out, te.errb.String())
	}
	if code, out := te.run(t, "", "status"); code != 0 || out != "2h54 · score 74 · focus 2h14 · now unlabelled · ◷ 50m\n" {
		t.Fatalf("status: %d %q", code, out)
	}
}

func TestSessionUsage(t *testing.T) {
	te := newTestEnv(t, testRules)
	te.seed(t, daySpans()...)
	for _, args := range [][]string{
		{"session", "start", "0"}, {"session", "start", "241"}, {"session", "start", "abc"},
		{"session", "start", "1", "2"}, {"session", "stop", "now"}, {"session", "pause"},
		{"recap", "yesterday"}, {"recap", "2026-10-09", "x"},
	} {
		if code, _ := te.run(t, "", args...); code != 2 {
			t.Errorf("%q: code %d, want 2", args, code)
		}
	}
}

func TestRecapCommand(t *testing.T) {
	te := newTestEnv(t, testRules)
	te.seed(t, daySpans()...)
	if code, out := te.run(t, "", "recap"); code != 0 || out != "2h14 focus, score 74 (target 4h00). Top project: tock.\n" {
		t.Fatalf("recap: %d %q", code, out)
	}
	if code, out := te.run(t, "", "recap", "2026-10-09"); code != 0 || out != "No activity.\n" {
		t.Fatalf("recap yesterday: %d %q", code, out)
	}
}
