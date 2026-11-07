package main

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"tock/internal/suggest"
)

type fakeClient struct {
	calls  int
	model  string
	got    []byte
	answer string
	err    error
}

func (f *fakeClient) Suggest(_ context.Context, model string, payload []byte) (json.RawMessage, error) {
	f.calls++
	f.model, f.got = model, payload
	return json.RawMessage(f.answer), f.err
}

func withClient(te *tenv, fc *fakeClient) {
	te.APIKey = "test-key"
	te.NewClient = func(key string) suggest.Client {
		if key != "test-key" {
			panic("wrong key")
		}
		return fc
	}
}

// optIn turns Claude on, as tock choice claude on does: a backend alone
// is not consent.
func optIn(t *testing.T, te *tenv) { te.writeConfig(t, "[llm]\nenabled = true\n") }

func TestSuggestCommand(t *testing.T) {
	te := newTestEnv(t, testRules)
	te.seed(t, daySpans()...)
	optIn(t, te)
	fc := &fakeClient{answer: `{"suggestions":[{"id":"k1","category":"Study","project":"tock"}]}`}
	withClient(te, fc)

	code, dry := te.run(t, "", "suggest", "--dry-run")
	if code != 0 || !strings.Contains(dry, `"host": "chatgpt.com"`) || strings.Contains(dry, "Proofs") || fc.calls != 0 {
		t.Fatalf("dry-run: code %d calls %d\n%s", code, fc.calls, dry)
	}
	if code, out := te.run(t, "", "suggest"); code != 0 || out != "sent 3 keys · 1 suggested\n" {
		t.Fatalf("suggest: code %d out %q err %q", code, out, te.errb.String())
	}
	if fc.calls != 1 || fc.model != "claude-haiku-5-5" || string(fc.got)+"\n" != dry {
		t.Fatalf("client got model %q, payload equal to dry-run: %v", fc.model, string(fc.got)+"\n" == dry)
	}
	// Every sent key is cached: nothing left, whatever the hour.
	if _, out := te.run(t, "", "suggest"); out != "nothing to send\n" || fc.calls != 1 {
		t.Fatalf("everything cached: %q calls %d", out, fc.calls)
	}
	// A new key waits for the hour; --force sends it now.
	te.seed(t, seedSpan{9, "13:00", "13:05", "notes", "Notes", "Draft", "", false})
	if _, out := te.run(t, "", "suggest"); out != "next batch after 16:00 (tock suggest --force)\n" || fc.calls != 1 {
		t.Fatalf("within the hour: %q calls %d", out, fc.calls)
	}
	fc.answer = `{"suggestions":[]}`
	if _, out := te.run(t, "", "suggest", "--force"); out != "sent 1 key · 0 suggested\n" || fc.calls != 2 {
		t.Fatalf("force: %q calls %d", out, fc.calls)
	}
	if _, out := te.run(t, "", "inbox"); !strings.Contains(out, "  1    12m  Arc · chatgpt.com  \"Proofs\"  → Study/tock\n") ||
		strings.Contains(out, "Mail · inbox  \"Inbox\"  →") {
		t.Fatalf("inbox suggestion column:\n%s", out)
	}
}

func TestSuggestWithoutKeyAndErrors(t *testing.T) {
	te := newTestEnv(t, testRules)
	te.seed(t, daySpans()...)
	optIn(t, te)
	if code, _ := te.run(t, "", "suggest"); code != 1 || !strings.HasPrefix(te.errb.String(), "tock: no Claude: install Claude Code") {
		t.Fatalf("no key: code %d err %q", code, te.errb.String())
	}
	if code, _ := te.run(t, "", "suggest", "--dry-run", "--force"); code != 2 {
		t.Fatal("--dry-run with --force must be a usage error")
	}
	fc := &fakeClient{err: errors.New("HTTP 401 authentication_error")}
	withClient(te, fc)
	if code, _ := te.run(t, "", "suggest"); code != 1 || te.errb.String() != "tock: suggestions failed: HTTP 401 authentication_error\n" {
		t.Fatalf("api error: code %d err %q", code, te.errb.String())
	}
	// The failed attempt still holds the hour: no retry storm.
	if _, out := te.run(t, "", "suggest"); !strings.HasPrefix(out, "next batch after 16:00") || fc.calls != 1 {
		t.Fatalf("after failure: %q calls %d", out, fc.calls)
	}
}

// fakeAI is the claude CLI backend in tests.
type fakeAI struct{ fakeClient }

func (f *fakeAI) Summarize(context.Context, string, []byte) (json.RawMessage, error) {
	return nil, errors.New("not used")
}

func (f *fakeAI) Ask(context.Context, string, []byte) (json.RawMessage, error) {
	return nil, errors.New("not used")
}

// TestSuggestThroughCLI: with no API key, suggestions go through the claude
// CLI (the user's subscription).
func TestSuggestThroughCLI(t *testing.T) {
	te := newTestEnv(t, testRules)
	te.seed(t, daySpans()...)
	optIn(t, te)
	fa := &fakeAI{fakeClient{answer: `{"suggestions":[{"id":"k1","category":"Study","project":"tock"}]}`}}
	te.ClaudeCLI = "/x/claude"
	te.NewCLI = func(path string) AI {
		if path != "/x/claude" {
			panic("wrong path")
		}
		return fa
	}
	if code, out := te.run(t, "", "suggest"); code != 0 || out != "sent 3 keys · 1 suggested\n" || fa.calls != 1 {
		t.Fatalf("suggest: code %d out %q err %q calls %d", code, out, te.errb.String(), fa.calls)
	}
	if _, out := te.run(t, "", "doctor"); !strings.Contains(out, "suggestions: on via your Claude subscription (claude CLI)") {
		t.Fatalf("doctor:\n%s", out)
	}
}
