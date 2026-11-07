package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// withBothBackends sets an API key and a claude CLI, each with a fake that
// counts calls: having a backend must never be consent.
func withBothBackends(te *tenv) (*fakeClient, *fakeAI) {
	fc := &fakeClient{answer: `{"suggestions":[]}`}
	withClient(te, fc)
	fa := &fakeAI{fakeClient{answer: `{"suggestions":[]}`}}
	te.ClaudeCLI = "/x/claude"
	te.NewCLI = func(string) AI { return fa }
	return fc, fa
}

func TestClaudeOffWithoutOptIn(t *testing.T) {
	te := newTestEnv(t, testRules)
	te.seed(t, daySpans()...)
	fc, fa := withBothBackends(te)

	code, out := te.run(t, "", "suggest")
	if code != 0 || !strings.Contains(out, "tock choice claude on") {
		t.Fatalf("suggest: code %d out %q err %q", code, out, te.errb.String())
	}
	if code, _ := te.run(t, "", "ask", "how much video?"); code != 1 || !strings.Contains(te.errb.String(), "tock choice claude on") {
		t.Fatalf("ask: code %d err %q", code, te.errb.String())
	}
	if code, out := te.run(t, "", "summary"); code != 0 || !strings.Contains(out, "tock choice claude on") {
		t.Fatalf("summary: code %d out %q err %q", code, out, te.errb.String())
	}
	if fc.calls != 0 || fa.calls != 0 {
		t.Fatalf("Claude called without opt-in: key %d, cli %d", fc.calls, fa.calls)
	}
	if _, out := te.run(t, "", "doctor"); !strings.Contains(out, "suggestions: off, Claude is opt-in (tock choice claude on)") {
		t.Fatalf("doctor:\n%s", out)
	}
}

func TestChoiceClaudeOnSaysWhatLeaves(t *testing.T) {
	te := newTestEnv(t, testRules)
	te.seed(t, daySpans()...)
	fc, _ := withBothBackends(te)

	code, out := te.run(t, "", "choice", "claude", "on")
	if code != 0 || !strings.Contains(out, "Claude: on") || !strings.Contains(out, "Anthropic") || !strings.Contains(out, "--dry-run") {
		t.Fatalf("choice claude on: code %d\n%s", code, out)
	}
	if got := readFile(t, te.configPath()); !strings.Contains(got, "enabled = true") {
		t.Fatalf("config.toml:\n%s", got)
	}
	if code, _ := te.run(t, "", "suggest", "--force"); code != 0 || fc.calls != 1 {
		t.Fatalf("after opt-in: code %d calls %d err %q", code, fc.calls, te.errb.String())
	}
}

// An upgrade turns Claude off for someone who had it through a backend
// alone; the TUI says so once.
func TestClaudeOptInNoticeOnce(t *testing.T) {
	te := newTestEnv(t, testRules)
	if got := llmOptInNotice(te.Env); got != "" {
		t.Fatalf("no backend: notice %q", got)
	}
	withBothBackends(te)
	got := llmOptInNotice(te.Env)
	if !strings.Contains(got, "tock choice claude on") {
		t.Fatalf("first notice = %q", got)
	}
	if again := llmOptInNotice(te.Env); again != "" {
		t.Fatalf("second notice = %q", again)
	}

	te2 := newTestEnv(t, testRules)
	withBothBackends(te2)
	te2.writeConfig(t, "[llm]\nenabled = false\n")
	if got := llmOptInNotice(te2.Env); got != "" {
		t.Fatalf("an explicit choice needs no notice: %q", got)
	}
	if _, err := os.Stat(filepath.Join(te2.StateDir, llmNoticeFile)); err == nil {
		t.Fatal("marker written without a notice")
	}
}
