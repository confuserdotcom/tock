package main

import (
	"strings"
	"testing"
)

func TestInboxListsAndIgnores(t *testing.T) {
	te := newTestEnv(t, testRules)
	te.seed(t, daySpans()...)
	code, out := te.run(t, "", "inbox")
	want := "Inbox  last 7 days · 3 keys · 28m unlabelled\n" +
		"  1    12m  Arc · chatgpt.com/c  \"Proofs\"\n" +
		"  2    10m  Mail · inbox  \"Inbox\"\n" +
		"  3     6m  Arc · labs.x.tv/lab  \"Low Level Labs\"\n" +
		"tag: tock tag <n> <Category>[/<Project>]   hide: tock ignore <n>\n"
	if code != 0 || out != want {
		t.Fatalf("inbox: code %d\n%s\nwant:\n%s", code, out, want)
	}
	if code, out := te.run(t, "", "ignore", "2"); code != 0 || !strings.HasPrefix(out, "ignored: Mail · inbox") {
		t.Fatalf("ignore 2: code %d out %q err %q", code, out, te.errb.String())
	}
	_, out = te.run(t, "", "inbox")
	if strings.Contains(out, "Mail") || !strings.Contains(out, "2 keys · 18m") {
		t.Fatalf("ignored key still listed:\n%s", out)
	}
	_, out = te.run(t, "", "today")
	if !strings.Contains(out, "Unclassified     28m") || !strings.Contains(out, "2 keys → tock inbox") {
		t.Fatalf("ignored time must still count as Unclassified, key count must drop:\n%s", out)
	}
	if code, out := te.run(t, "", "ignore", "--undo", "Mail · inbox"); code != 0 || !strings.HasPrefix(out, "showing again") {
		t.Fatalf("undo: code %d out %q", code, out)
	}
	if _, out := te.run(t, "", "inbox"); !strings.Contains(out, "Mail · inbox") {
		t.Fatalf("undo did not bring the key back:\n%s", out)
	}
}

func TestInboxWindowAndEmpty(t *testing.T) {
	te := newTestEnv(t, testRules)
	spans := daySpans()
	spans[7].end = "15:00" // no open span: it would stretch to "now"
	te.seed(t, spans...)
	te.now = testNow.AddDate(0, 0, 10)
	if _, out := te.run(t, "", "inbox"); out != "Inbox empty: everything in the last 7 days is labelled.\n" {
		t.Fatalf("old spans outside window: %q", out)
	}
	if _, out := te.run(t, "", "inbox", "--days", "30"); !strings.Contains(out, "3 keys") {
		t.Fatalf("--days 30: %q", out)
	}
	if code, _ := te.run(t, "", "inbox", "--days", "0"); code != 2 {
		t.Fatalf("--days 0 must be a usage error")
	}
}

func TestListingExpiresAndBadNumbers(t *testing.T) {
	te := newTestEnv(t, testRules)
	te.seed(t, daySpans()...)
	if code, _ := te.run(t, "", "ignore", "1"); code != 1 || !strings.Contains(te.errb.String(), "listing expired") {
		t.Fatalf("no listing yet: %q", te.errb.String())
	}
	te.run(t, "", "inbox")
	if code, _ := te.run(t, "", "ignore", "9"); code != 1 || !strings.Contains(te.errb.String(), "no item 9") {
		t.Fatalf("item 9: %q", te.errb.String())
	}
	if code, _ := te.run(t, "", "ignore", "x"); code != 2 {
		t.Fatalf("non-number: code %d", code)
	}
	te.now = te.now.Add(61 * 60 * 1e9)
	if code, _ := te.run(t, "", "ignore", "1"); code != 1 || !strings.Contains(te.errb.String(), "listing expired") {
		t.Fatalf("stale listing: %q", te.errb.String())
	}
}
