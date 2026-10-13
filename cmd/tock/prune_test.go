package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPrune(t *testing.T) {
	te := newTestEnv(t, testRules)
	te.seed(t, daySpans()...)
	if code, _ := te.run(t, "", "prune"); code != 2 || !strings.Contains(te.errb.String(), "keep_titles") {
		t.Fatalf("off: code %d err %q", code, te.errb)
	}
	// testNow's day is today: nothing is older than one day.
	if code, out := te.run(t, "", "prune", "--days", "1", "--dry-run"); code != 0 ||
		out != "would delete the titles of 0 spans before 2026-10-09\n" {
		t.Fatalf("dry run: code %d out %q err %q", code, out, te.errb)
	}
	te.now = te.now.AddDate(0, 0, 3)
	if code, out := te.run(t, "", "prune", "--days", "1"); code != 0 ||
		out != "deleted the titles of 6 spans before 2026-10-12\n" {
		t.Fatalf("prune: code %d out %q err %q", code, out, te.errb)
	}
	if code, out := te.run(t, "", "today", "--date", "2026-10-10"); code != 0 || !strings.Contains(out, "Code ") {
		t.Fatalf("labels after prune: %s", out)
	}
	if err := os.WriteFile(filepath.Join(te.ConfigDir, "config.toml"), []byte("[privacy]\nkeep_titles = 2\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if code, out := te.run(t, "", "doctor"); !strings.Contains(out, "window titles kept 2 days") {
		t.Errorf("doctor (code %d):\n%s", code, out)
	}
	if code, _ := te.run(t, "", "prune", "--days", "-3"); code != 2 {
		t.Errorf("negative days: code %d", code)
	}
}
