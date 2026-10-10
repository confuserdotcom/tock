package main

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
)

type fakeLLM struct {
	calls   int
	got     []byte
	summary string
	query   string
	model   string
}

func (f *fakeLLM) Summarize(_ context.Context, model string, p []byte) (json.RawMessage, error) {
	f.calls++
	f.got, f.model = p, model
	return json.RawMessage(f.summary), nil
}

func (f *fakeLLM) Ask(_ context.Context, model string, p []byte) (json.RawMessage, error) {
	f.calls++
	f.got, f.model = p, model
	return json.RawMessage(f.query), nil
}

func withLLM(te *tenv, f *fakeLLM) {
	te.APIKey = "test-key"
	te.NewLLM = func(key string) LLM { return f }
}

func TestBlocksAndSummary(t *testing.T) {
	te := newTestEnv(t, testRules)
	te.seed(t, daySpans()...)
	code, out := te.run(t, "", "blocks")
	if code != 0 || !strings.Contains(out, "09:00–10:40   1h40  tock (Code) · deep 100% · WezTerm") {
		t.Fatalf("blocks = %d\n%s", code, out)
	}
	f := &fakeLLM{summary: `{"day":"You spent the morning coding tock and ATLAS, then watched a video.","blocks":[{"id":"b1","summary":"Coding in the terminal"}]}`}
	withLLM(te, f)
	code, dry := te.run(t, "", "summary", "--dry-run")
	if code != 0 || f.calls != 0 || !strings.Contains(dry, `"day": "Saturday"`) || strings.Contains(dry, "2026") {
		t.Fatalf("dry-run = %d %d\n%s", code, f.calls, dry)
	}
	code, out = te.run(t, "", "summary")
	if code != 0 || f.calls != 1 || string(f.got)+"\n" != dry || f.model != "claude-haiku-5-5" {
		t.Fatalf("summary = %d %s calls %d", code, te.errb, f.calls)
	}
	if !strings.Contains(out, "You spent the morning coding tock") || !strings.Contains(out, "               Coding in the terminal") {
		t.Fatalf("summary out:\n%s", out)
	}
	// Saved: blocks shows it without a call.
	if _, out := te.run(t, "", "blocks"); !strings.Contains(out, "Coding in the terminal") || f.calls != 1 {
		t.Fatalf("blocks after summary:\n%s", out)
	}
	te.APIKey = ""
	if code, _ := te.run(t, "", "summary"); code != 1 || !strings.Contains(te.errb.String(), "ANTHROPIC_API_KEY") {
		t.Fatalf("no key: %d %s", code, te.errb)
	}
}

func TestAsk(t *testing.T) {
	te := newTestEnv(t, testRules)
	te.seed(t, daySpans()...)
	f := &fakeLLM{query: `{"range":"today","group_by":"site","category":"Entertainment"}`}
	withLLM(te, f)
	code, dry := te.run(t, "", "ask", "--dry-run", "how much", "video today?")
	if code != 0 || f.calls != 0 || !strings.Contains(dry, `"question": "how much video today?"`) || strings.Contains(dry, "youtube") {
		t.Fatalf("dry = %d\n%s", code, dry)
	}
	code, out := te.run(t, "", "ask", "how much video today?")
	if code != 0 || !strings.Contains(out, "Entertainment, today: 12m") || !strings.Contains(out, "youtube.com") {
		t.Fatalf("ask = %d %s\n%s", code, te.errb, out)
	}
	f.query = `{"range":"today","group_by":"total","unsupported":"not about time"}`
	if code, _ := te.run(t, "", "ask", "what is love"); code != 1 || !strings.Contains(te.errb.String(), "cannot answer") {
		t.Fatalf("unsupported = %d %s", code, te.errb)
	}
	if code, _ := te.run(t, "", "ask"); code != 2 {
		t.Fatal("empty question")
	}
}
