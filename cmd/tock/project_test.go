package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestProjectCommand(t *testing.T) {
	te := newTestEnv(t, testRules)
	te.seed(t, daySpans()...)
	vault := t.TempDir()
	for _, d := range []string{"Analyse 1", "Algèbre 1", "_Système", ".git"} {
		os.MkdirAll(filepath.Join(vault, d), 0o700)
	}
	if code, out := te.run(t, "", "project", "add", "Thermodynamique", "--category", "stu", "--alias", "thermo, Précis"); code != 0 ||
		out != "added project Thermodynamique (module of Study; also thermo, Précis)\n" {
		t.Fatalf("add: %d %q %s", code, out, te.errb)
	}
	if code, out := te.run(t, "", "project", "scan", vault, "--category", "Study"); code != 0 || out != "added 2 modules: Algèbre 1, Analyse 1\n" {
		t.Fatalf("scan: %d %q %s", code, out, te.errb)
	}
	if _, out := te.run(t, "", "project", "scan", vault, "--category", "Study"); out != "every folder already has a project\n" {
		t.Fatalf("rescan: %q", out)
	}
	if code, out := te.run(t, "", "project", "set", "ana", "--alias", "ana1"); code != 0 || out != "Analyse 1 (module of Study; also ana1)\n" {
		t.Fatalf("set: %d %q %s", code, out, te.errb)
	}
	if code, out := te.run(t, "", "project", "set", "Thermo", "--category", "none", "--alias", "none"); code != 0 || out != "Thermodynamique\n" {
		t.Fatalf("unfile: %d %q %s", code, out, te.errb)
	}
	code, out := te.run(t, "", "project")
	if code != 0 || !strings.Contains(out, "Study") || !strings.Contains(out, "  Analyse 1") || !strings.Contains(out, "no category") {
		t.Fatalf("list: %d\n%s%s", code, out, te.errb)
	}
	if code, _ := te.run(t, "", "project", "rm", "Thermodynamique"); code != 0 {
		t.Fatalf("rm: %s", te.errb)
	}
}
