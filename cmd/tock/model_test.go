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
	// the study words the Study category starts with
	code, out = te.run(t, "", "model", "why", "arc", "Exercices de thermodynamique")
	if code != 0 || !strings.Contains(out, "title          Study") || !strings.Contains(out, "thermo +") {
		t.Fatalf("model why: code %d\n%s%s", code, out, te.errb)
	}
	if code, _ := te.run(t, "", "model", "what"); code != 2 {
		t.Fatalf("bad args: code %d", code)
	}
}
