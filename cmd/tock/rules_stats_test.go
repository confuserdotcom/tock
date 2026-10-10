package main

import (
	"strings"
	"testing"
)

func TestRulesStats(t *testing.T) {
	te := newTestEnv(t, testRules+"\n[[rule]]\nurl = '^never'\ncategory = \"Study\"\n")
	te.seed(t, daySpans()...)
	code, out := te.run(t, "", "rules", "stats")
	if code != 0 {
		t.Fatalf("exit %d: %s", code, te.errb)
	}
	lines := strings.Split(strings.TrimRight(out, "\n"), "\n")
	want := []string{
		"last 30 days · time decided",
		"1      1h40  project tock          bundle=[term] title='^tock │'",
		"2      2h14  Code                  bundle=[term]",
		"3       12m  Entertainment         url='^(www\\.)?youtube\\.com(/|$)'",
		"4        0m  Study                 url='^never'  unused",
	}
	if strings.Join(lines, "\n") != strings.Join(want, "\n") {
		t.Fatalf("got:\n%s\nwant:\n%s", out, strings.Join(want, "\n"))
	}
	if code, _ := te.run(t, "", "rules", "stats", "--days", "0"); code != 2 {
		t.Fatalf("--days 0 exit %d, want 2", code)
	}
	if code, _ := te.run(t, "", "rules", "nope"); code != 2 {
		t.Fatalf("bad subcommand exit %d, want 2", code)
	}
}
