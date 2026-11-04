package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestInboxListsAndIgnores(t *testing.T) {
	te := newTestEnv(t, testRules)
	te.seed(t, daySpans()...)
	code, out := te.run(t, "", "inbox")
	want := "Inbox  last 7 days · 3 keys · 28m unlabelled\n" +
		"  1    12m  Arc · chatgpt.com  \"Proofs\"\n" +
		"  2    10m  Mail · inbox  \"Inbox\"\n" +
		"  3     6m  Arc · labs.x.tv  \"Low Level Labs\"\n" +
		"tag: tock tag <n> <Category>[/<Project>]   accept →: tock tag <n>   hide: tock ignore <n>\n"
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

// Time labelled on one chatgpt.com page suggests the label for the others;
// tock tag <n> without a label accepts it.
func TestInboxGuessAccepted(t *testing.T) {
	te := newTestEnv(t, testRules)
	os.WriteFile(filepath.Join(te.ConfigDir, "config.toml"), []byte("[model]\nenabled = false\n[inbox]\nauto_accept = 0\n"), 0o600) // the model or auto-accept would label it outright
	te.seed(t, append(daySpans(), seedSpan{9, "13:00", "13:20", "arc", "Arc", "Settings", "chatgpt.com/settings", false})...)
	if code, _ := te.run(t, "", "tag", "--span", "9", "Study"); code != 0 {
		t.Fatalf("tag --span: %s", te.errb)
	}
	code, out := te.run(t, "", "inbox")
	if code != 0 || !strings.Contains(out, `"Proofs"  → Study (same site, 100%)`) {
		t.Fatalf("inbox: code %d\n%s", code, out)
	}
	code, out = te.run(t, "y\n", "tag", "1")
	if code != 0 || !strings.Contains(out, "learned: Study") {
		t.Fatalf("tag 1: code %d\n%s%s", code, out, te.errb)
	}
	if code, _ := te.run(t, "", "inbox"); code != 0 {
		t.Fatal(te.errb.String())
	}
	if code, _ := te.run(t, "", "tag", "1"); code != 1 || !strings.Contains(te.errb.String(), "no suggestion for Mail · inbox") {
		t.Fatalf("no suggestion: code %d %q", code, te.errb.String())
	}
}

// With the model on, the label of one chatgpt.com page carries to another
// page there whose title is study, without a trip to the inbox.
func TestInboxModelLabels(t *testing.T) {
	te := newTestEnv(t, testRules)
	te.seed(t, append(daySpans(), seedSpan{9, "13:00", "13:20", "arc", "Arc", "Settings", "chatgpt.com/settings", false})...)
	if code, _ := te.run(t, "", "tag", "--span", "9", "Study"); code != 0 {
		t.Fatalf("tag --span: %s", te.errb)
	}
	code, out := te.run(t, "", "inbox")
	if code != 0 || strings.Contains(out, "Proofs") {
		t.Fatalf("inbox: code %d\n%s", code, out)
	}
}

// TestInboxSiteRow: every page of a site is one row that counts its pages.
func TestInboxSiteRow(t *testing.T) {
	te := newTestEnv(t, testRules)
	te.seed(t,
		seedSpan{1, "09:00", "09:10", "arc", "Arc", "Opening book", "app.noctie.ai/opening-book", false},
		seedSpan{2, "09:10", "09:15", "arc", "Arc", "Puzzles", "app.noctie.ai/puzzles", false},
		seedSpan{3, "09:15", "09:16", "arc", "Arc", "PDF", "doc-0o-bs.googleusercontent.com/viewer/a", false},
		seedSpan{4, "09:16", "09:17", "arc", "Arc", "PDF", "doc-1k-aa.googleusercontent.com/viewer/b", false},
		seedSpan{5, "09:17", "09:30", "term", "Term", "x", "", false},
	)
	_, out := te.run(t, "", "inbox")
	for _, want := range []string{
		"  1    15m  Arc · app.noctie.ai · 2 pages  \"Puzzles\"\n",
		"  2     2m  Arc · googleusercontent.com  \"PDF\"\n",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("inbox lacks %q:\n%s", want, out)
		}
	}
}

// TestInboxNoiseFloor: keys under [inbox] min_minutes are summed in one line;
// --all lists them.
func TestInboxNoiseFloor(t *testing.T) {
	te := newTestEnv(t, testRules)
	te.seed(t,
		seedSpan{1, "09:00", "09:10", "arc", "Arc", "Noctie", "app.noctie.ai/x", false},
		seedSpan{2, "09:10", "09:11", "arc", "Arc", "Kitea", "kitea.com/x", false},
		seedSpan{3, "09:11", "09:12", "arc", "Arc", "Steelcase", "steelcase.com/x", false},
		seedSpan{4, "09:12", "09:30", "term", "Term", "x", "", false},
	)
	_, out := te.run(t, "", "inbox")
	want := "Inbox  last 7 days · 1 key · 10m unlabelled\n" +
		"  1    10m  Arc · app.noctie.ai  \"Noctie\"\n" +
		"+ 2 small keys under 2m (2m) · tock inbox --all\n" +
		"tag: tock tag <n> <Category>[/<Project>]   accept →: tock tag <n>   hide: tock ignore <n>\n"
	if out != want {
		t.Fatalf("inbox:\n%s\nwant:\n%s", out, want)
	}
	if _, out := te.run(t, "", "inbox", "--all"); !strings.Contains(out, "3 keys · 12m") || !strings.Contains(out, "steelcase.com") {
		t.Fatalf("--all:\n%s", out)
	}
}

// TestInboxAutoAccept: a key whose same-site guess is confident is labelled
// by a learned rule before the list prints; --auto lists it for review.
func TestInboxAutoAccept(t *testing.T) {
	te := newTestEnv(t, testRules)
	te.seed(t,
		seedSpan{1, "09:00", "09:20", "safari", "Safari", "", "chess.com/play", false},
		seedSpan{2, "09:20", "09:25", "arc", "Arc", "", "chess.com/analysis", false},
		seedSpan{3, "09:25", "09:40", "arc", "Arc", "Noctie", "app.noctie.ai/x", false},
	)
	if code, _ := te.run(t, "", "tag", "--span", "1", "Entertainment"); code != 0 {
		t.Fatalf("tag: %s", te.errb.String())
	}
	_, out := te.run(t, "", "inbox")
	if strings.Contains(out, "chess.com") || !strings.Contains(out, "auto-labelled 1 key (5m) today · tock inbox --auto") {
		t.Fatalf("inbox:\n%s", out)
	}
	b, err := os.ReadFile(te.learnedPath())
	if err != nil || !strings.Contains(string(b), `chess\\.com`) || !strings.Contains(string(b), `category = "Entertainment"`) {
		t.Fatalf("learned.toml: %s %v", b, err)
	}
	_, out = te.run(t, "", "inbox", "--auto")
	want := "Auto-labelled  last 7 days · 1 key\n" +
		"  1     5m  Arc · chess.com  → Entertainment (same site, 100%)\n" +
		"wrong? tock tag <n> <Category>[/<Project>] relabels it and replaces the rule\n"
	if out != want {
		t.Fatalf("--auto:\n%s\nwant:\n%s", out, want)
	}
	// Re-tagging an auto-labelled key works from that listing and sticks.
	if code, out := te.run(t, "y\n", "tag", "1", "Study"); code != 0 || !strings.Contains(out, "learned: Study") {
		t.Fatalf("retag: %q %s", out, te.errb.String())
	}
	if _, out := te.run(t, "", "inbox"); strings.Contains(out, "chess.com") || strings.Contains(out, "auto-labelled 2") {
		t.Fatalf("after retag:\n%s", out)
	}
}
