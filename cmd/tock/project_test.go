package main

import (
	"database/sql"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestProjectCommand(t *testing.T) {
	te := newTestEnv(t, testRules)
	te.seed(t, daySpans()...)
	vault := t.TempDir()
	for _, d := range []string{"Analyse 1", "Alge\u0300bre 1", "_Système", ".git"} {
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
	if code, out := te.run(t, "", "project", "set", "Algèbre 1", "--alias", "alg1"); code != 0 || out != "Algèbre 1 (module of Study; also alg1)\n" {
		t.Fatalf("set NFC: %d %q %s", code, out, te.errb)
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

func TestProjectRefile(t *testing.T) {
	te := newTestEnv(t, testRules+"\n[[project]]\nname = \"Thermo\"\ncategory = \"Study\"\n[[project]]\nname = \"Licence\"\n")
	te.seed(t, daySpans()...)
	// span 1's title is "tock │ ✳ x": rename it so it names the module.
	db, err := sql.Open("sqlite", "file:"+te.dbPath())
	if err != nil {
		t.Fatal(err)
	}
	for _, q := range []string{`UPDATE span SET title = 'Thermo cours' WHERE id = 1`,
		`INSERT INTO override(span_id, category, project, at) VALUES(1, 'Study', 'Licence', 1), (2, 'Study', 'Licence', 1)`} {
		if _, err := db.Exec(q); err != nil {
			t.Fatal(err)
		}
	}
	db.Close()
	if code, out := te.run(t, "", "project", "refile", "Licence"); code != 0 || out != "moved 1 span to modules: Thermo 1\n" {
		t.Fatalf("refile: %d %q %s", code, out, te.errb)
	}
}

// A freelancer sets a project's rate, client and billable flag from the
// CLI, without editing rules.toml.
func TestProjectRateFlags(t *testing.T) {
	te := newTestEnv(t, testRules)
	te.seed(t, daySpans()...)
	steps := []struct {
		args []string
		code int
		out  string
	}{
		{[]string{"project", "add", "Orbit", "--rate", "80", "--client", "Acme Ltd"}, 0, "added project Orbit (rate 80/h; client Acme Ltd)\n"},
		{[]string{"project", "set", "orb", "--rate", "95.5"}, 0, "Orbit (rate 95.5/h; client Acme Ltd)\n"},
		{[]string{"project", "set", "orb", "--billable", "false"}, 0, "Orbit (rate 95.5/h, not billed; client Acme Ltd)\n"},
		{[]string{"project", "set", "orb", "--billable", "true", "--client", "none"}, 0, "Orbit (rate 95.5/h)\n"},
		{[]string{"project", "set", "orb", "--rate", "none"}, 0, "Orbit\n"},
		{[]string{"project", "set", "orb", "--rate", "eighty"}, 2, ""},
		{[]string{"project", "set", "orb", "--rate", "-3"}, 2, ""},
		{[]string{"project", "set", "orb", "--billable", "maybe"}, 2, ""},
	}
	for _, s := range steps {
		code, out := te.run(t, "", s.args...)
		if code != s.code || (s.code == 0 && out != s.out) {
			t.Fatalf("%v: %d %q, want %d %q; stderr %s", s.args, code, out, s.code, s.out, te.errb)
		}
	}
	te.run(t, "", "project", "set", "orb", "--rate", "60")
	rules, _ := os.ReadFile(te.rulesPath())
	if !strings.Contains(string(rules), "rate = 60") {
		t.Fatalf("rules.toml lacks the rate:\n%s", rules)
	}
	// With Billing off, rates are hidden, so they are not set either.
	if err := os.WriteFile(filepath.Join(te.ConfigDir, "config.toml"), []byte("[billing]\nenabled = false\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if code, _ := te.run(t, "", "project", "set", "orb", "--rate", "70"); code != 1 || !strings.Contains(te.errb.String(), "tock choice billing on") {
		t.Fatalf("billing off: %d %s", code, te.errb)
	}
	if code, out := te.run(t, "", "project", "set", "orb", "--alias", "orb2"); code != 0 || strings.Contains(out, "rate") {
		t.Fatalf("billing off hides the rate: %d %q %s", code, out, te.errb)
	}
}
