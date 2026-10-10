package main

import (
	"os"
	"strings"
	"testing"
)

const dupTestRules = testRules + `
[[category]]
name = "Studies"
kind = "deep"

[[rule]]
app = 'Preview'
category = "Studies"
`

func TestRulesDupesAndMerge(t *testing.T) {
	te := newTestEnv(t, dupTestRules)
	te.seed(t, daySpans()...)
	code, out := te.run(t, "", "rules", "dupes")
	if code != 0 || !strings.Contains(out, "category  Study (unused)  ~  Studies (unused)") ||
		!strings.Contains(out, `tock rules merge "Studies" "Study"`) {
		t.Fatalf("exit %d:\n%s%s", code, out, te.errb)
	}
	if _, out := te.run(t, "", "doctor"); !strings.Contains(out, "1 name pair look alike (tock rules dupes)") {
		t.Fatalf("doctor:\n%s", out)
	}
	code, out = te.run(t, "", "rules", "merge", "studies", "study")
	if code != 0 || !strings.Contains(out, "merged category Studies into Study") {
		t.Fatalf("exit %d:\n%s%s", code, out, te.errb)
	}
	body, _ := os.ReadFile(te.rulesPath())
	if strings.Contains(string(body), "Studies") {
		t.Fatalf("rules.toml:\n%s", body)
	}
	if bak, _ := os.ReadFile(te.rulesPath() + ".bak"); !strings.Contains(string(bak), "Studies") {
		t.Fatal("no backup of the old rules.toml")
	}
	if _, out := te.run(t, "", "rules", "dupes"); !strings.Contains(out, "no look-alike") {
		t.Fatalf("after merge:\n%s", out)
	}
	if code, _ := te.run(t, "", "rules", "merge", "Code", "Nope"); code == 0 {
		t.Fatal("merged into an undeclared name")
	}
}
