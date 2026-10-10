package tui

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"tock/internal/apptest"
)

type fakeSuggester struct {
	calls  int
	answer string
	err    error
}

func (f *fakeSuggester) Suggest(context.Context, string, []byte) (json.RawMessage, error) {
	f.calls++
	return json.RawMessage(f.answer), f.err
}

const studyTock = `{"suggestions":[{"id":"k1","category":"Study","project":"tock"}]}`

func withSuggestions(t *testing.T, f *fakeSuggester) (Model, *apptest.Env) {
	t.Helper()
	e := apptest.New(t, apptest.Rules)
	e.Seed(t, apptest.DaySpans()...)
	e.Heartbeat(t, e.Clock.UnixMilli())
	return newModelWith(t, e, f), e
}

// Phase 6 criterion 1: suggestions arrive without a key press and show in
// the inbox; the hourly claim stops a second call.
func TestInboxShowsSuggestions(t *testing.T) {
	f := &fakeSuggester{answer: studyTock}
	m, _ := withSuggestions(t, f)
	m = press(t, m, "3")
	mustContain(t, m, "3 keys · 28m unlabelled · 1 suggested", "→ Study / tock")
	if l := line(t, m, "Mail · inbox"); strings.Contains(l, "→") {
		t.Fatalf("Mail has no suggestion: %q", l)
	}
	m = press(t, m, "r")
	if f.calls != 1 {
		t.Fatalf("calls = %d, want 1", f.calls)
	}
}

func TestInboxAcceptSuggestion(t *testing.T) {
	m, _ := withSuggestions(t, &fakeSuggester{answer: studyTock})
	m = press(t, m, "3", "a")
	mustContain(t, m, "Learn a rule", "->  Study / tock")
	m = press(t, m, "y")
	mustContain(t, m, "learned: Study / tock now covers 2 spans (12m)", "3 inbox 2")
	m = press(t, m, "a")
	mustContain(t, m, "no suggestion · enter to label")
}

func TestSuggestFailureFlashesOnce(t *testing.T) {
	f := &fakeSuggester{err: errors.New("HTTP 401 authentication_error")}
	m, _ := withSuggestions(t, f)
	mustContain(t, m, "suggestions failed: HTTP 401 authentication_error")
	m = press(t, m, "3")
	mustNotContain(t, m, "→")
	if f.calls != 1 {
		t.Fatalf("calls = %d, want 1", f.calls)
	}
}

func TestNoSuggesterNoColumn(t *testing.T) {
	m, e := seeded(t)
	e.Heartbeat(t, e.Clock.UnixMilli())
	m = press(t, m, "3")
	mustNotContain(t, m, "suggested", "→")
}
