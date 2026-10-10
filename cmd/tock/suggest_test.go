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

func TestSuggestCommand(t *testing.T) {
	te := newTestEnv(t, testRules)
	te.seed(t, daySpans()...)
	fc := &fakeClient{answer: `{"suggestions":[{"id":"k1","category":"Study","project":"tock"}]}`}
	withClient(te, fc)

	code, dry := te.run(t, "", "suggest", "--dry-run")
	if code != 0 || !strings.Contains(dry, `"host": "chatgpt.com/c"`) || strings.Contains(dry, "Proofs") || fc.calls != 0 {
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
	if _, out := te.run(t, "", "inbox"); !strings.Contains(out, "  1    12m  Arc · chatgpt.com/c  \"Proofs\"  → Study/tock\n") ||
		strings.Contains(out, "Mail · inbox  \"Inbox\"  →") {
		t.Fatalf("inbox suggestion column:\n%s", out)
	}
}

func TestSuggestWithoutKeyAndErrors(t *testing.T) {
	te := newTestEnv(t, testRules)
	te.seed(t, daySpans()...)
	if code, _ := te.run(t, "", "suggest"); code != 1 || te.errb.String() != "tock: ANTHROPIC_API_KEY is not set\n" {
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
