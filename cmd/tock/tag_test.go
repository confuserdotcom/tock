package main

import (
	"os"
	"strings"
	"testing"
)

// TestTagLearnAppliesToHistory is phase 2 success criterion 4: tagging an
// inbox item proposes a rule with its hit count and, once accepted, labels
// past spans.
func TestTagLearnAppliesToHistory(t *testing.T) {
	te := newTestEnv(t, testRules)
	te.seed(t, append(daySpans(),
		seedSpan{9, "13:00", "13:05", "safari", "Safari", "Chat", "chatgpt.com/c/9", false})...)
	te.run(t, "", "inbox") // 1 = Arc · chatgpt.com/c
	code, out := te.run(t, "y\n", "tag", "1", "stu/atl")
	if code != 0 {
		t.Fatalf("tag: code %d err %q", code, te.errb.String())
	}
	for _, want := range []string{
		`rule: url='^(www\.)?chatgpt\.com(/|$)'  ->  Study / ATLAS`,
		"matches 3 spans (17m): 2 this key, 1 other unlabelled (5m), 0 relabelled",
		"[y] accept  [s] more specific  [n] cancel: ",
		"learned: Study / ATLAS now covers 3 spans (17m)",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in:\n%s", want, out)
		}
	}
	data, err := os.ReadFile(te.learnedPath())
	if err != nil || !strings.Contains(string(data), `# learned 2026-10-10 from "Arc · chatgpt.com/c", 2 spans`) {
		t.Fatalf("learned.toml:\n%s (err %v)", data, err)
	}
	_, out = te.run(t, "", "today")
	if !strings.Contains(out, "Study            17m") || !strings.Contains(out, "ATLAS 17m") {
		t.Fatalf("history not relabelled:\n%s", out)
	}
	if _, out := te.run(t, "", "inbox"); strings.Contains(out, "chatgpt") {
		t.Fatalf("learned key still in inbox:\n%s", out)
	}
}

func TestTagMoreSpecificAndCancel(t *testing.T) {
	te := newTestEnv(t, testRules)
	te.seed(t, daySpans()...)
	te.run(t, "", "inbox")
	code, out := te.run(t, "s\nn\n", "tag", "1", "Study")
	if code != 0 || !strings.Contains(out, `rule: url='^(www\.)?chatgpt\.com/c(/|$)'`) || !strings.HasSuffix(out, "cancelled\n") {
		t.Fatalf("s then n: code %d out:\n%s", code, out)
	}
	if _, err := os.Stat(te.learnedPath()); !os.IsNotExist(err) {
		t.Fatal("cancel must not write learned.toml")
	}
	if code, out := te.run(t, "", "tag", "1", "Study"); code != 0 || !strings.HasSuffix(out, "cancelled\n") {
		t.Fatalf("EOF must cancel: %q", out)
	}
}

func TestTagOnceAndSpan(t *testing.T) {
	te := newTestEnv(t, testRules)
	te.seed(t, daySpans()...)
	te.run(t, "", "inbox")
	if code, out := te.run(t, "", "tag", "--once", "3", "Code"); code != 0 || out != "tagged 1 spans (6m) → Code\n" {
		t.Fatalf("--once: code %d out %q err %q", code, out, te.errb.String())
	}
	if code, out := te.run(t, "", "tag", "--span", "1", "/ATLAS"); code != 0 || out != "tagged span 1 → project ATLAS\n" {
		t.Fatalf("--span: code %d out %q", code, out)
	}
	_, out := te.run(t, "", "today")
	if !strings.Contains(out, "Code            2h20") || !strings.Contains(out, "ATLAS 1h40") {
		t.Fatalf("manual tags not applied:\n%s", out)
	}
	if code, _ := te.run(t, "", "tag", "--span", "999", "Code"); code != 1 || !strings.Contains(te.errb.String(), "no span with id 999") {
		t.Fatalf("missing span: %q", te.errb.String())
	}
}

// TestTagSkipsUnsafeCandidates: the url-only candidates would relabel the
// Chrome span (Entertainment by rule), so the first one offered is
// bundle+host.
func TestTagSkipsUnsafeCandidates(t *testing.T) {
	te := newTestEnv(t, testRules)
	te.seed(t,
		seedSpan{1, "09:00", "09:10", "arc", "Arc", "A", "www.youtube.com/watch", false},
		seedSpan{2, "09:10", "09:20", "chrome", "Chrome", "C", "www.youtube.com/watch", false},
	)
	te.run(t, "", "today", "--keys") // 1 = Arc · youtube.com/watch (Entertainment)
	code, out := te.run(t, "y\n", "tag", "1", "Study")
	if code != 0 {
		t.Fatalf("code %d err %q", code, te.errb.String())
	}
	if !strings.Contains(out, `rule: bundle=[arc] url='^(www\.)?youtube\.com(/|$)'  ->  Study`) ||
		!strings.Contains(out, "matches 1 spans (10m): 1 this key, 0 other unlabelled (0s), 0 relabelled") ||
		!strings.Contains(out, "[y] accept  [n] cancel: ") {
		t.Fatalf("expected only the bundle+host candidate:\n%s", out)
	}
	_, out = te.run(t, "", "today")
	if !strings.Contains(out, "Study            10m") || !strings.Contains(out, "Entertainment    10m") {
		t.Fatalf("only the Arc span should move to Study:\n%s", out)
	}
}

// TestTagNoSafeRuleOffersOnce: every candidate relabels some other span, so
// tag offers to label just these spans.
func TestTagNoSafeRuleOffersOnce(t *testing.T) {
	te := newTestEnv(t, testRules)
	te.seed(t,
		seedSpan{1, "09:00", "09:10", "arc", "Arc", "A", "www.youtube.com/watch", false},
		seedSpan{2, "09:10", "09:20", "arc", "Arc", "B", "www.youtube.com/watch", false},
		seedSpan{3, "09:20", "09:30", "arc", "Arc", "Home", "www.youtube.com", false},
		seedSpan{4, "09:30", "09:40", "chrome", "Chrome", "C", "www.youtube.com/watch", false},
	)
	te.run(t, "", "today", "--keys") // 1 = Arc · youtube.com/watch
	code, out := te.run(t, "o\n", "tag", "1", "Study")
	if code != 0 {
		t.Fatalf("code %d err %q", code, te.errb.String())
	}
	if !strings.Contains(out, "no safe rule") || !strings.HasSuffix(out, "tagged 2 spans (20m) → Study\n") {
		t.Fatalf("out:\n%s", out)
	}
	if _, err := os.Stat(te.learnedPath()); !os.IsNotExist(err) {
		t.Fatal("no rule must be learned")
	}
}

func TestTagPrivateNeedsOnce(t *testing.T) {
	te := newTestEnv(t, testRules)
	te.seed(t, seedSpan{1, "09:00", "09:10", "arc", "Arc", "", "", true})
	te.run(t, "", "inbox")
	if code, _ := te.run(t, "", "tag", "1", "Study"); code != 1 || !strings.Contains(te.errb.String(), "use tock tag --once") {
		t.Fatalf("private learn: %q", te.errb.String())
	}
	if code, out := te.run(t, "", "tag", "--once", "1", "Study"); code != 0 || !strings.HasPrefix(out, "tagged 1 spans") {
		t.Fatalf("private --once: %q %q", out, te.errb.String())
	}
}

func TestTagBadNames(t *testing.T) {
	te := newTestEnv(t, testRules)
	te.seed(t, daySpans()...)
	te.run(t, "", "inbox")
	if code, _ := te.run(t, "", "tag", "1", "c"); code != 1 || !strings.Contains(te.errb.String(), "ambiguous category") {
		t.Fatalf("ambiguous: %q", te.errb.String())
	}
	if code, _ := te.run(t, "", "tag", "1", "/"); code != 2 {
		t.Fatalf("empty label must be a usage error")
	}
	if code, _ := te.run(t, "", "tag", "1"); code != 2 {
		t.Fatalf("missing label must be a usage error")
	}
}
