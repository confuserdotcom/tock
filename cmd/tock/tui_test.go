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

func TestPlanCommandLine(t *testing.T) {
	var tab string
	old := runProgram
	runProgram = func(_ Env, m tui.Model) error { tab = m.TabName(); return nil }
	t.Cleanup(func() { runProgram = old })

	te := newTestEnv(t, "")
	if code, _ := te.run(t, "", "plan"); code != 1 || !strings.Contains(te.errb.String(), "no plan file") {
		t.Fatalf("no file: exit %d %s", code, te.errb)
	}
	file := te.ConfigDir + "/../todo.txt"
	os.MkdirAll(te.ConfigDir, 0o755)
	os.WriteFile(te.ConfigDir+"/config.toml", []byte("[plan]\nfile = \""+file+"\"\n"), 0o644)
	os.WriteFile(file, []byte("(A) a due:"+testNow.Format("2006-01-02")+"\nx done one\nb\n"), 0o644)
	te.out.Reset()
	if code, out := te.run(t, "", "plan"); code != 0 || out != "Today\n  (A) a due:"+testNow.Format("2006-01-02")+"\nNo date\n  b\n" {
		t.Fatalf("piped: exit %d\n%q", code, out)
	}

	te2 := newTestEnv(t, testRules)
	te2.seed(t, daySpans()...)
	te2.Interactive = true
	if code, _ := te2.run(t, "", "plan"); code != 0 || tab != "plan" {
		t.Fatalf("interactive: exit %d tab %q", code, tab)
	}
}
