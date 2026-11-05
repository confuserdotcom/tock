package main

import (
	"strings"
	"testing"
	"time"

	"tock/internal/store"
)

// A first run: trackd is up but has recorded nothing yet. today and inbox
// say so instead of looking broken or claiming everything is labelled.
func TestFirstRun(t *testing.T) {
	te := newTestEnv(t, testRules)
	te.seed(t)
	setStates(t, te, map[string]string{store.KeyHeartbeat: nowMS()})
	if _, out := te.run(t, "", "today"); !strings.Contains(out, "Nothing recorded yet. trackd is recording") {
		t.Errorf("today: %q", out)
	}
	if _, out := te.run(t, "", "inbox"); !strings.HasPrefix(out, "Nothing recorded yet.") {
		t.Errorf("inbox: %q", out)
	}
	te.now = testNow.Add(10 * time.Minute) // no heartbeat since
	if _, out := te.run(t, "", "today"); !strings.Contains(out, "trackd is not running. Run tock doctor") {
		t.Errorf("today with trackd down: %q", out)
	}
}
