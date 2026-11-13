package main

import (
	"strings"
	"testing"
)

func TestModelCommand(t *testing.T) {
	te := newTestEnv(t, testRules)
	te.seed(t, daySpans()...)
	code, out := te.run(t, "", "model")
	if code != 0 || !strings.HasPrefix(out, "Model  ") {
		t.Fatalf("model: code %d\n%s%s", code, out, te.errb)
	}
	if strings.Contains(out, "Your labels") {
		t.Fatalf("agreement shown with no hand labels:\n%s", out)
	}
	// the study words the Study category starts with
	code, out = te.run(t, "", "model", "why", "arc", "Exercices de thermodynamique")
	if code != 0 || !strings.Contains(out, "title          Study") || !strings.Contains(out, "thermo +") {
		t.Fatalf("model why: code %d\n%s%s", code, out, te.errb)
	}
	if code, _ := te.run(t, "", "tag", "--span", "1", "Study"); code != 0 {
		t.Fatalf("tag: %s", te.errb)
	}
	code, out = te.run(t, "", "model")
	if code != 0 || !strings.Contains(out, "Your labels  1 span") || !strings.Contains(out, "agrees") {
		t.Fatalf("model after a hand label: code %d\n%s%s", code, out, te.errb)
	}
	if code, _ := te.run(t, "", "model", "what"); code != 2 {
		t.Fatalf("bad args: code %d", code)
	}
}
