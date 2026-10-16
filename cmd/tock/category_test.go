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
