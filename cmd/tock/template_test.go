package main

import (
	"os"
	"strings"
	"testing"
)

func TestSetupTemplateApplyTwice(t *testing.T) {
	te := newTestEnv(t, testRules)
	te.seed(t, seedSpan{id: 1, start: "09:00", end: "10:00", bundle: "term", app: "WezTerm", title: "x"})
	code, out := te.run(t, "", "setup", "--template", "client", "--yes")
	if code != 0 {
		t.Fatalf("exit %d: %s %s", code, out, te.errb)
	}
	for _, want := range []string{"categories to add: Meeting, Admin & Planning", "rules to add (4)", "from now on", "tock undo takes them back"} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in:\n%s", want, out)
		}
	}
	b, _ := os.ReadFile(te.rulesPath())
	code, out = te.run(t, "", "setup", "--template", "client", "--yes")
	if code != 0 || !strings.Contains(out, "nothing to change") {
		t.Fatalf("second run exit %d:\n%s", code, out)
	}
	if b2, _ := os.ReadFile(te.rulesPath()); string(b2) != string(b) {
		t.Fatal("second run changed rules.toml")
	}
	if code, out = te.run(t, "", "undo"); code != 0 {
		t.Fatalf("undo exit %d: %s", code, out)
	}
	if b3, _ := os.ReadFile(te.rulesPath()); string(b3) != testRules {
		t.Fatal("undo did not restore rules.toml")
	}
}

func TestSetupTemplateNeedsYesOffTerminal(t *testing.T) {
	te := newTestEnv(t, testRules)
	te.seed(t, seedSpan{id: 1, start: "09:00", end: "10:00", bundle: "term", app: "WezTerm", title: "x"})
	code, out := te.run(t, "", "setup", "--template", "own")
	if code == 0 || !strings.Contains(te.errb.String(), "--yes") {
		t.Fatalf("exit %d out %s err %s", code, out, te.errb)
	}
	if b, _ := os.ReadFile(te.rulesPath()); string(b) != testRules {
		t.Fatal("wrote without --yes")
	}
}

func TestSetupTemplateBeforeTrackd(t *testing.T) {
	te := newTestEnv(t, testRules)
	code, out := te.run(t, "", "setup", "--template", "own", "--yes")
	if code != 0 {
		t.Fatalf("exit %d: %s %s", code, out, te.errb)
	}
	if strings.Contains(out, "undo") {
		t.Fatalf("offers undo without a database:\n%s", out)
	}
	if b, _ := os.ReadFile(te.rulesPath()); !strings.Contains(string(b), "dev.zed.Zed") {
		t.Fatal("not applied")
	}
}

func TestSetupTemplateUnknown(t *testing.T) {
	te := newTestEnv(t, testRules)
	if code, _ := te.run(t, "", "setup", "--template", "student", "--yes"); code == 0 {
		t.Fatal("unknown template accepted")
	}
}

func TestChooseTemplates(t *testing.T) {
	te := newTestEnv(t, "")
	if got := chooseTemplates(te.Env, true); strings.Join(got, ",") != "own" {
		t.Fatalf("off a terminal: %v", got)
	}
	te.Interactive = true
	te.Stdin = strings.NewReader("1, 2\n")
	if got := chooseTemplates(te.Env, true); strings.Join(got, ",") != "client,own" {
		t.Fatalf("1,2: %v", got)
	}
	te.Stdin = strings.NewReader("\n")
	if got := chooseTemplates(te.Env, true); strings.Join(got, ",") != "own" {
		t.Fatalf("default: %v", got)
	}
}
