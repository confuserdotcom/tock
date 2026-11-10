package main

import (
	"strconv"
	"strings"
	"testing"
	"time"

	"tock/internal/service"
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
	if code, _ := te.run(t, "", "status"); code != 1 || te.errb.String() != "tock: no data yet: is trackd running? (tock doctor says how to start it)\n" {
		t.Fatalf("no db: code %d err %q", code, te.errb.String())
	}
}

// AC7: trackd stopped because a newer tock raised compat; status says so
// and prints the fix for this install on stderr, keeping stdout one line.
func TestStatusTrackdTooOld(t *testing.T) {
	te := newTestEnv(t, testRules)
	te.seed(t, daySpans()...)
	setStates(t, te, map[string]string{store.KeyTrackdStopped: store.StoppedSchemaNewer})
	old := installMethod
	installMethod = func() service.Method { return service.Brew }
	t.Cleanup(func() { installMethod = old })
	code, out := te.run(t, "", "status")
	if code != 0 || out != "old! trackd down · 2h54\n" {
		t.Fatalf("code %d out %q", code, out)
	}
	if got := te.errb.String(); got != "trackd is older than the database; update it: tock setup\n" {
		t.Fatalf("stderr %q", got)
	}
}

// AC2 (tock half): a write failure in the last 24 h marks the status line,
// and doctor names write_failed with a time.
func TestStatusAndDoctorWriteFailed(t *testing.T) {
	te := newTestEnv(t, testRules)
	te.seed(t, daySpans()...)
	at := strconv.FormatInt(testNow.UnixMilli()-3600_000, 10)
	setStates(t, te, map[string]string{store.KeyApplyFailed: "2", store.KeyLastErrorMS: at,
		store.KeyLastError: "store: apply op 0 (insert): disk I/O error"})
	if code, out := te.run(t, "", "status"); code != 0 || out != "write failed! trackd down · 2h54\n" {
		t.Fatalf("status: code %d out %q", code, out)
	}
	_, out := te.run(t, "", "doctor")
	when := testNow.Add(-time.Hour).Format("Mon 2 Jan 15:04")
	if !strings.Contains(out, "!!  writes: write_failed at "+when+" (2 in all): store: apply op 0 (insert): disk I/O error") {
		t.Fatalf("doctor:\n%s", out)
	}
	// A day later it is history, not a problem.
	te.now = te.now.Add(25 * time.Hour)
	if _, out := te.run(t, "", "doctor"); !strings.Contains(out, "ok  writes: none failed in 24 h (2 in all") {
		t.Fatalf("doctor a day later:\n%s", out)
	}
}

func TestStatusDropped(t *testing.T) {
	te := newTestEnv(t, testRules)
	te.seed(t, daySpans()...)
	setStates(t, te, map[string]string{store.KeySamplesDropped: "5",
		store.KeyDroppedMS: strconv.FormatInt(testNow.UnixMilli()-60_000, 10)})
	if code, out := te.run(t, "", "status"); code != 0 || out != "dropped! trackd down · 2h54\n" {
		t.Fatalf("status: code %d out %q", code, out)
	}
}
