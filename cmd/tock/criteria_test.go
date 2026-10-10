package main

import (
	"os"
	"strings"
	"testing"
)

// TestEditingRulesRelabelsHistory is phase 2 success criterion 1: editing
// rules.toml changes the labels of already recorded spans on the next
// command, with no restart and no rewrite of the database.
func TestEditingRulesRelabelsHistory(t *testing.T) {
	te := newTestEnv(t, testRules)
	te.seed(t, daySpans()...)
	if _, out := te.run(t, "", "today"); !strings.Contains(out, "Unclassified     28m") {
		t.Fatalf("before:\n%s", out)
	}
	edited := testRules + `
[[rule]]
url = '^chatgpt\.com/'
category = "Study"
`
	if err := os.WriteFile(te.rulesPath(), []byte(edited), 0o600); err != nil {
		t.Fatal(err)
	}
	_, out := te.run(t, "", "today")
	if !strings.Contains(out, "Study            12m") || !strings.Contains(out, "Unclassified     16m") {
		t.Fatalf("after editing rules.toml:\n%s", out)
	}
}
