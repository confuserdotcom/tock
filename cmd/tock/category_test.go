package main

import (
	"os"
	"strings"
	"testing"
)

func TestCategoryCommand(t *testing.T) {
	te := newTestEnv(t, testRules)
	te.seed(t, daySpans()...)
	if code, out := te.run(t, "", "category", "add", "Reading", "neutral", "--afk", "10m"); code != 0 || out != "added category Reading (neutral)\n" {
		t.Fatalf("add: %d %q %s", code, out, te.errb)
	}
	if code, out := te.run(t, "", "category", "set", "read", "--kind", "browse"); code != 0 || out != "Reading: browse, afk 10m\n" {
		t.Fatalf("set: %d %q %s", code, out, te.errb)
	}
	if code, _ := te.run(t, "", "category", "set", "read", "--afk", "1m"); code != 1 {
		t.Fatal("afk 1m accepted")
	}
	if code, out := te.run(t, "", "category", "rename", "Reading", "Books"); code != 0 || !strings.HasPrefix(out, "renamed category Reading to Books") {
		t.Fatalf("rename: %d %q %s", code, out, te.errb)
	}
	code, out := te.run(t, "", "category")
	if code != 0 || !strings.Contains(out, "Books") || !strings.Contains(out, "browse") || !strings.Contains(out, "afk 10m") {
		t.Fatalf("list: %d\n%s%s", code, out, te.errb)
	}
	if code, _ := te.run(t, "", "category", "rm", "Books"); code != 0 {
		t.Fatalf("rm: %s", te.errb)
	}
	b, _ := os.ReadFile(te.rulesPath())
	if strings.Contains(string(b), "Books") {
		t.Fatalf("rules still name Books:\n%s", b)
	}
	if code, _ := te.run(t, "", "category", "add", "X"); code != 2 {
		t.Fatal("add without kind is not a usage error")
	}
}

func TestCategorySetInterrupt(t *testing.T) {
	te := newTestEnv(t, testRules)
	te.seed(t, daySpans()...)
	if code, out := te.run(t, "", "category", "add", "Reading", "neutral", "--afk", "10m"); code != 0 {
		t.Fatalf("add: %d %q %s", code, out, te.errb)
	}
	if code, out := te.run(t, "", "category", "set", "Reading", "--kind", "browse", "--interrupt", "off"); code != 0 ||
		out != "Reading: browse, afk 10m, no distraction notices\n" {
		t.Fatalf("off: %d %q %s", code, out, te.errb)
	}
	b, _ := os.ReadFile(te.rulesPath())
	if !strings.Contains(string(b), "name = \"Reading\"\ninterrupt = false\n") {
		t.Fatalf("rules.toml:\n%s", b)
	}
	// Another flag on a quiet category keeps the segment.
	if code, out := te.run(t, "", "category", "set", "read", "--afk", "20m"); code != 0 ||
		out != "Reading: browse, afk 20m, no distraction notices\n" {
		t.Fatalf("afk on a quiet category: %d %q %s", code, out, te.errb)
	}
	if code, out := te.run(t, "", "category", "set", "Reading", "--interrupt", "on"); code != 0 ||
		out != "Reading: browse, afk 20m, distraction notices on\n" {
		t.Fatalf("on: %d %q %s", code, out, te.errb)
	}
	// A normal category's line is unchanged.
	if code, out := te.run(t, "", "category", "set", "Reading", "--kind", "neutral"); code != 0 ||
		out != "Reading: neutral, afk 20m\n" {
		t.Fatalf("normal: %d %q %s", code, out, te.errb)
	}
	te.errb.Reset()
	if code, _ := te.run(t, "", "category", "set", "Reading", "--interrupt", "maybe"); code != 2 ||
		!strings.Contains(te.errb.String(), "category set: --interrupt \"maybe\", want on or off") {
		t.Fatalf("maybe: %d %q", code, te.errb.String())
	}
	if code, _ := te.run(t, "", "category", "set", "Reading"); code != 2 {
		t.Fatal("category set with no flag is not a usage error")
	}
}

func TestCategorySetInterruptPrefix(t *testing.T) {
	te := newTestEnv(t, testRules)
	te.seed(t, daySpans()...)
	if code, out := te.run(t, "", "category", "add", "Reading", "neutral", "--afk", "10m"); code != 0 {
		t.Fatalf("add: %d %q %s", code, out, te.errb)
	}
	if code, out := te.run(t, "", "category", "set", "read", "--interrupt", "off"); code != 0 ||
		out != "Reading: neutral, afk 10m, no distraction notices\n" {
		t.Fatalf("prefix off: %d %q %s", code, out, te.errb)
	}
}
