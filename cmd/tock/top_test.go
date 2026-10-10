package main

import (
	"strings"
	"testing"
)

func TestTop(t *testing.T) {
	te := newTestEnv(t, testRules)
	te.seed(t, daySpans()...)
	code, out := te.run(t, "", "top")
	if code != 0 {
		t.Fatalf("exit %d: %s", code, te.errb)
	}
	for _, want := range []string{
		"Top  Sun 4 Oct – Sat 10 Oct · ",
		"\nCategories\n  Code ",
		"\nApps\n  WezTerm ",
		"mostly Code",
		"\nSites\n  chatgpt.com ", "youtube.com    12m",
		"(unlabelled)",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("top lacks %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "Idle") || strings.Contains(out, "Projects") {
		t.Errorf("idle time or an unasked section:\n%s", out)
	}
	code, out = te.run(t, "", "top", "--by", "site", "--limit", "1")
	if code != 0 || strings.Contains(out, "Apps") || !strings.Contains(out, "… 2 more, ") {
		t.Fatalf("--by site --limit 1: exit %d\n%s", code, out)
	}
	if code, _ := te.run(t, "", "top", "--by", "window"); code != 2 {
		t.Fatal("unknown section accepted")
	}
	if code, _ := te.run(t, "", "top", "--week", "--month"); code != 2 {
		t.Fatal("--week with --month accepted")
	}
}
