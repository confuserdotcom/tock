package main

import (
	"os"
	"strings"
	"testing"

	"tock/internal/tui"
)

func TestFirstRunWelcome(t *testing.T) {
	ran := 0
	old := runProgram
	runProgram = func(Env, tui.Model) error { ran++; return nil }
	t.Cleanup(func() { runProgram = old })

	te := newTestEnv(t, "")
	te.Interactive = true
	code, out := te.run(t, "n\n")
	if code != 0 || !strings.Contains(out, "where your time goes") || !strings.Contains(out, "Run tock init") {
		t.Fatalf("no: exit %d\n%s", code, out)
	}
	if _, err := os.Stat(te.rulesPath()); !os.IsNotExist(err) {
		t.Fatal("no must not write rules")
	}

	// Yes writes the starter rules; without a database it says how to start
	// trackd.
	code, out = te.run(t, "\n")
	if code != 1 || !strings.Contains(out, "wrote "+te.rulesPath()) ||
		!strings.Contains(te.errb.String(), "make install") {
		t.Fatalf("yes: exit %d\n%s%s", code, out, te.errb)
	}
	if _, err := os.Stat(te.rulesPath()); err != nil {
		t.Fatal(err)
	}

	// With rules and a database the TUI starts.
	te.seed(t, daySpans()...)
	if code, _ := te.run(t, ""); code != 0 || ran != 1 {
		t.Fatalf("exit %d, ran %d", code, ran)
	}

	// Not on a terminal: usage, no prompt.
	te2 := newTestEnv(t, "")
	if code, out := te2.run(t, "y\n"); code != 0 || !strings.Contains(out, "usage:") || strings.Contains(out, "where your time goes") {
		t.Fatalf("non-interactive: exit %d\n%s", code, out)
	}
}
