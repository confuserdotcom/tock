package main

import (
	"os"
	"strings"
	"testing"
)

func TestTodayReport(t *testing.T) {
	te := newTestEnv(t, testRules)
	te.seed(t, daySpans()...)
	code, out := te.run(t, "", "today")
	if code != 0 {
		t.Fatalf("code %d err %q", code, te.errb.String())
	}
	want := "Today  2h54 tracked · 41m idle\n" +
		"score 74 · focus 2h14 / 4h00 · 1 block, longest 2h14 · 2 switches\n" +
		"Code            2h14  ████████████  tock 1h40\n" +
		"Entertainment    12m  █\n" +
		"Unclassified     28m  ██▌           3 keys → tock inbox\n"
	if out != want {
		t.Fatalf("today output:\n%s\nwant:\n%s", out, want)
	}
}

func TestTodayClipsToDayAndOtherDates(t *testing.T) {
	te := newTestEnv(t, testRules)
	// An open span from 23:00 on 2026-10-10, viewed at 15:00 the next day.
	te.seed(t, seedSpan{1, "23:00", "", "term", "WezTerm", "x", "", false})
	te.now = testNow.AddDate(0, 0, 1)
	code, out := te.run(t, "", "today")
	if code != 0 || !strings.HasPrefix(out, "Today  15h00 tracked · 0s idle\n") {
		t.Fatalf("today (clipped at midnight, open until now): code %d out %q", code, out)
	}
	code, out = te.run(t, "", "today", "--date", "2026-10-10")
	if code != 0 || !strings.HasPrefix(out, "2026-10-10  1h00 tracked · 0s idle\n") {
		t.Fatalf("--date (clipped at the next midnight): code %d out %q", code, out)
	}
	code, out = te.run(t, "", "today", "--date", "2026-10-09")
	if code != 0 || out != "2026-10-09  0s tracked · 0s idle\n(no activity)\n" {
		t.Fatalf("empty day: code %d out %q", code, out)
	}
	if code, _ := te.run(t, "", "today", "--date", "10/10/2026"); code != 2 {
		t.Fatalf("bad --date: code %d", code)
	}
}

func TestTodayKeysNumbersAndSavesListing(t *testing.T) {
	te := newTestEnv(t, testRules)
	te.seed(t, daySpans()...)
	code, out := te.run(t, "", "today", "--keys")
	if code != 0 {
		t.Fatalf("code %d err %q", code, te.errb.String())
	}
	for _, want := range []string{
		"      1   1h40  WezTerm · tock │ x",
		"      2    34m  WezTerm · atlas │ y",
		"      3    12m  Arc · youtube.com/watch",
		"      4    12m  Arc · chatgpt.com/c",
		"      5    10m  Mail · inbox",
		"      6     6m  Arc · labs.x.tv/lab",
	} {
		if !strings.Contains(out, want+"\n") {
			t.Errorf("missing %q in:\n%s", want, out)
		}
	}
	if k, err := resolveListed(te.Env, "3"); err != nil || k != "Arc · youtube.com/watch" {
		t.Fatalf("listing 3 = %q, %v", k, err)
	}
	if fi, err := os.Stat(te.listingPath()); err != nil || fi.Mode().Perm() != 0o600 {
		t.Fatalf("listing file mode: %v %v", fi, err)
	}
}

func TestMissingRulesAndDB(t *testing.T) {
	te := newTestEnv(t, "")
	te.seed(t)
	if code, _ := te.run(t, "", "today"); code != 1 || !strings.Contains(te.errb.String(), "no rules: run tock init") {
		t.Fatalf("no rules: code %d err %q", code, te.errb.String())
	}
	te = newTestEnv(t, testRules)
	if code, _ := te.run(t, "", "today"); code != 1 || !strings.Contains(te.errb.String(), "no data yet") {
		t.Fatalf("no db: code %d err %q", code, te.errb.String())
	}
	if _, err := os.Stat(te.dbPath()); !os.IsNotExist(err) {
		t.Fatal("tock must not create tock.db")
	}
}
