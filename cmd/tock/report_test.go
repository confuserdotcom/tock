package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestReportHTML(t *testing.T) {
	te := newTestEnv(t, goalRules)
	te.seed(t, daySpans()...)
	code, out := te.run(t, "", "report")
	if code != 0 {
		t.Fatalf("code %d: %s", code, te.errb)
	}
	for _, want := range []string{
		"<!doctype html>", "<title>tock · week of 5 Oct 2026</title>", "Mon 5 Oct – Sun 11 Oct 2026",
		"<b>2h54</b><span>tracked</span>", "<b>74</b><span>average score</span>",
		"<td>Code</td><td class=\"num\">2h14</td>", "Apps and sites", "youtube.com",
		"Entertainment ≤10m/day", "over 1 of 1 day", "class=\"k-deep\"",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("page lacks %q", want)
		}
	}
	for _, bad := range []string{"<script", "http://", "https://", "XOR Equivalence"} {
		if strings.Contains(out, bad) {
			t.Errorf("page has %q", bad)
		}
	}
}

func TestReportFileAndOpen(t *testing.T) {
	te := newTestEnv(t, testRules)
	te.seed(t, daySpans()...)
	var opened string
	te.Open = func(u string) error { opened = u; return nil }
	path := filepath.Join(t.TempDir(), "w.html")
	if code, out := te.run(t, "", "report", "--week", "2026-10-07", "-o", path, "--open"); code != 0 || out != "wrote "+path+"\n" {
		t.Fatalf("code %d out %q err %s", code, out, te.errb)
	}
	if b, err := os.ReadFile(path); err != nil || !strings.Contains(string(b), "Week of 5 Oct 2026") {
		t.Fatalf("file: %v", err)
	}
	if opened != "file://"+path {
		t.Errorf("opened %q", opened)
	}
	if code, _ := te.run(t, "", "report", "--week", "oct"); code != 2 {
		t.Errorf("bad week: code %d", code)
	}
}
