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
	if !wantsHelp("invoice", []string{"issue", "--help"}) || wantsHelp("edit", []string{"--", "--help"}) || wantsHelp("tag", []string{"1", "help"}) {
		t.Fatal("wantsHelp")
	}
	// tock ask takes your words: only a lone help asks for help.
	if wantsHelp("ask", []string{"help", "with", "invoices"}) || wantsHelp("ask", []string{"what", "does", "-h", "do"}) || !wantsHelp("ask", []string{"--help"}) {
		t.Fatal("wantsHelp ask")
	}
}

// A command with its own usage text shows all of it: project's set options
// and refile are not in the one-line summary.
func TestOwnUsageHelp(t *testing.T) {
	te := newTestEnv(t, "")
	for name := range ownUsage {
		code, out := te.run(t, "", name, "--help")
		if code != 0 || !strings.HasPrefix(out, "usage:\n  ") || strings.Contains(out, name+": usage") || strings.Contains(out, "Every day:") {
			t.Errorf("%s --help: %d\n%s", name, code, out)
		}
	}
	if _, out := te.run(t, "", "project", "--help"); !strings.Contains(out, "--billable") || !strings.Contains(out, "refile") {
		t.Errorf("project help:\n%s", out)
	}
}

// An unknown command gets one line and a near match, not the whole usage.
func TestUnknownCommandSuggests(t *testing.T) {
	te := newTestEnv(t, "")
	for _, c := range []struct{ cmd, want string }{
		{"exprt", `did you mean "export"?`},
		{"invoices", `did you mean "invoice"?`},
		{"week", "the week is in the terminal UI (tock) and the dashboard (tock web)"},
		{"frobnicate", "tock help lists every command"},
	} {
		code, _ := te.run(t, "", c.cmd)
		e := te.errb.String()
		if code != 2 || !strings.Contains(e, `unknown command "`+c.cmd+`"`) || !strings.Contains(e, c.want) || strings.Contains(e, "Every day:") {
			t.Errorf("%s: %d\n%s", c.cmd, code, e)
		}
	}
}
