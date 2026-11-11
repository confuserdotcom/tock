package main

import (
	"strings"
	"testing"
)

// `tock <command> --help` (or -h, or `tock <command> help`) prints that
// command's lines, exits 0 and needs no rules and no database: it is what a
// new user tries first.
func TestCommandHelp(t *testing.T) {
	te := newTestEnv(t, "") // no rules.toml, no database
	for _, args := range [][]string{
		{"export", "--help"},
		{"export", "-h"},
		{"invoice", "--help"},
		{"invoice", "help"},
		{"invoice", "issue", "--help"},
		{"tag", "-help"},
		{"bug-report", "--help"},
	} {
		code, out := te.run(t, "", args...)
		if code != 0 {
			t.Errorf("%v: exit %d, stderr %q", args, code, te.errb.String())
			continue
		}
		if !strings.Contains(out, "tock "+args[0]) {
			t.Errorf("%v: help does not show the command:\n%s", args, out)
		}
		if strings.Contains(out, "Every day:") {
			t.Errorf("%v: printed the whole usage instead of the command's:\n%s", args, out)
		}
		if strings.Contains(te.errb.String(), "help requested") {
			t.Errorf("%v: leaked the flag package's error: %q", args, te.errb.String())
		}
	}
	if _, out := te.run(t, "", "invoice", "--help"); !strings.Contains(out, "tock invoice verify") {
		t.Errorf("invoice help lacks its subcommands:\n%s", out)
	}
}

// A bad flag names the problem and shows that command's lines, not all
// hundred lines of usage.
func TestUsageErrorShowsCommandLines(t *testing.T) {
	te := newTestEnv(t, "")
	code, _ := te.run(t, "", "export", "--nope")
	if code != 2 {
		t.Fatalf("exit %d", code)
	}
	e := te.errb.String()
	if !strings.Contains(e, "flag provided but not defined: -nope") || !strings.Contains(e, "tock export [--from D]") {
		t.Fatalf("stderr:\n%s", e)
	}
	if strings.Contains(e, "Every day:") {
		t.Fatalf("whole usage printed:\n%s", e)
	}
}

// A word after the command that only looks like help is left alone: a
// category named "help" can still be tagged, and a note can say --help.
func TestHelpOnlyBeforeDoubleDash(t *testing.T) {
	if !wantsHelp([]string{"issue", "--help"}) || wantsHelp([]string{"--", "--help"}) || wantsHelp([]string{"1", "help"}) {
		t.Fatal("wantsHelp")
	}
}
