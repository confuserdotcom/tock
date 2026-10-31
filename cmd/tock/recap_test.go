package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestRecapStart(t *testing.T) {
	te := newTestEnv(t, testRules)
	te.seed(t, daySpans()...)
	if code, out := te.run(t, "", "recap", "start"); code != 0 || out != "Yesterday: no activity.\n" {
		t.Fatalf("recap start: %d %q", code, out)
	}

	file := filepath.Join(t.TempDir(), "todo.txt")
	if err := os.WriteFile(file, []byte("Standup due:2026-10-10 @10h00\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(te.ConfigDir, "config.toml"), []byte("[plan]\nfile = \""+file+"\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if code, out := te.run(t, "", "recap", "start"); code != 0 || out != "Yesterday: no activity. Today: 10:00 Standup.\n" {
		t.Fatalf("recap start with a plan: %d %q", code, out)
	}
	if code, _ := te.run(t, "", "recap", "start", "x"); code != 2 {
		t.Fatalf("recap start x: exit %d, want 2", code)
	}
}

func TestRecapMidday(t *testing.T) {
	te := newTestEnv(t, testRules)
	te.seed(t, daySpans()...)
	code, out := te.run(t, "", "recap", "midday")
	if code != 0 || out != "2h54 tracked, 73% of 4h00 target · top: Code · focus 2h14.\n" {
		t.Fatalf("recap midday: %d %q", code, out)
	}
	if code, _ := te.run(t, "", "recap", "midday", "x"); code != 2 {
		t.Fatalf("recap midday x: exit %d, want 2", code)
	}
}

func TestRecapEndOff(t *testing.T) {
	te := newTestEnv(t, testRules)
	te.seed(t, daySpans()...)
	if err := os.WriteFile(filepath.Join(te.ConfigDir, "config.toml"), []byte("[routine]\nend = false\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	code, out := te.run(t, "", "recap")
	if code != 0 || out != "2h14 focus, score 74 (target 4h00). Top project: tock.\n" {
		t.Fatalf("recap with end = false: %d %q", code, out)
	}
}
