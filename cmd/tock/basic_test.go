package main

import (
	"os"
	"strings"
	"testing"

	"tock/internal/classify"
	"tock/internal/store"
)

func TestFmtDur(t *testing.T) {
	for ms, want := range map[int64]string{0: "0s", 45_000: "45s", 60_000: "1m", 41*60_000 + 59_000: "41m", 3_600_000: "1h00", 2*3_600_000 + 14*60_000: "2h14"} {
		if got := fmtDur(ms); got != want {
			t.Errorf("fmtDur(%d) = %q, want %q", ms, got, want)
		}
	}
}

func TestBar(t *testing.T) {
	cases := []struct {
		ms, max int64
		want    string
	}{
		{100, 100, strings.Repeat("█", 12)},
		{50, 100, strings.Repeat("█", 6)},
		{1, 100, "▌"},
		{0, 100, ""},
		{13, 24, strings.Repeat("█", 6) + "▌"},
	}
	for _, c := range cases {
		if got := bar(c.ms, c.max); got != c.want {
			t.Errorf("bar(%d,%d) = %q, want %q", c.ms, c.max, got, c.want)
		}
	}
}

func TestSafeStripsControls(t *testing.T) {
	if got := safe("a\x1b[31mb\x07"); got != "a?[31mb?" {
		t.Fatalf("safe = %q", got)
	}
}

func TestUsageAndUnknownCommand(t *testing.T) {
	te := newTestEnv(t, "")
	if code, out := te.run(t, ""); code != 0 || !strings.Contains(out, "usage:") {
		t.Fatalf("no args: code %d out %q", code, out)
	}
	if code, _ := te.run(t, "", "frobnicate"); code != 2 || !strings.Contains(te.errb.String(), `unknown command "frobnicate"`) {
		t.Fatalf("unknown: code %d err %q", code, te.errb.String())
	}
	if code, _ := te.run(t, "", "init", "extra"); code != 2 {
		t.Fatalf("bad arguments: code %d", code)
	}
}

func TestInitWritesStarterOnce(t *testing.T) {
	te := newTestEnv(t, "")
	if code, out := te.run(t, "", "init"); code != 0 || !strings.Contains(out, "wrote ") {
		t.Fatalf("init: code %d out %q err %q", code, out, te.errb.String())
	}
	data, err := os.ReadFile(te.rulesPath())
	if err != nil || string(data) != string(classify.Starter) {
		t.Fatalf("rules.toml is not the starter file (err %v)", err)
	}
	for p, want := range map[string]os.FileMode{te.rulesPath(): 0o600, te.ConfigDir: 0o700} {
		if fi, err := os.Stat(p); err != nil || fi.Mode().Perm() != want {
			t.Errorf("%s mode = %v, want %v", p, fi.Mode().Perm(), want)
		}
	}
	if code, _ := te.run(t, "", "init"); code != 1 || !strings.Contains(te.errb.String(), "already exists") {
		t.Fatalf("second init: code %d err %q", code, te.errb.String())
	}
}

func TestRulesCheck(t *testing.T) {
	te := newTestEnv(t, testRules)
	te.seed(t, daySpans()...)
	code, out := te.run(t, "", "rules", "check")
	if code != 0 || !strings.Contains(out, "rules.toml: 4 categories, 2 projects, 3 rules") || !strings.Contains(out, "learned.toml: 0 rules") {
		t.Fatalf("check: code %d out %q err %q", code, out, te.errb.String())
	}
	st, err := store.Open(te.dbPath())
	if err != nil {
		t.Fatal(err)
	}
	if err := st.SetOverrides([]store.Override{{SpanID: 4, Category: "Study", At: 1}}); err != nil {
		t.Fatal(err)
	}
	st.Close()
	renamed := strings.Replace(testRules, `name = "Study"`, `name = "Learning"`, 1)
	if err := os.WriteFile(te.rulesPath(), []byte(renamed), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, out := te.run(t, "", "rules", "check"); !strings.Contains(out, `warning: 1 manual tags name undeclared category "Study"`) {
		t.Fatalf("stale override not flagged: %q", out)
	}
	if err := os.WriteFile(te.rulesPath(), []byte(testRules+"[[rule]]\napp='x'\ncategory='Nope'\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if code, _ := te.run(t, "", "rules", "check"); code != 1 || !strings.Contains(te.errb.String(), `rules.toml: rule #4 (app='x'): unknown category "Nope"`) {
		t.Fatalf("invalid rules: code %d err %q", code, te.errb.String())
	}
}
