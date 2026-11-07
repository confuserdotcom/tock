package tui

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

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
	e.WriteConfig(t, "[llm]\nenabled = true\n") // Claude is opt-in
	return newModelWith(t, e, f), e
}

// Phase 6 criterion 1: suggestions arrive without a key press and show in
// the inbox; the hourly claim stops a second call.
func TestInboxShowsSuggestions(t *testing.T) {
	f := &fakeSuggester{answer: studyTock}
	m, _ := withSuggestions(t, f)
	m = press(t, m, "3")
	mustContain(t, m, "28m unlabelled · 3 keys · last 7 days · 1 suggested", "→ Study / tock")
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
	mustContain(t, m, "learned: Study / tock now covers 2 spans (12m)", "inbox ²")
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

func TestInboxEnterAcceptsSuggestion(t *testing.T) {
	m, _ := withSuggestions(t, &fakeSuggester{answer: studyTock})
	m = press(t, m, "3")
	for i := 0; i < 3 && !strings.Contains(line(t, m, "→ Study / tock"), "▸"); i++ {
		m = press(t, m, "j")
	}
	m = press(t, m, "enter")
	if m.learn == nil {
		t.Fatalf("enter on a suggested key must open the learn dialog:\n%s", screen(m))
	}
}

func TestPickerOffersSuggestion(t *testing.T) {
	m, _ := withSuggestions(t, &fakeSuggester{answer: studyTock})
	m = press(t, m, "3")
	for i := 0; i < 3 && !strings.Contains(line(t, m, "→ Study / tock"), "▸"); i++ {
		m = press(t, m, "j")
	}
	m = press(t, m, "o") // tag once, through the picker
	mustContain(t, m, "★ suggested")
	m = press(t, m, "enter")
	mustContain(t, m, "→ Study / tock")
	if m.picker != nil {
		t.Fatal("a quick option must finish the pick")
	}
}

// Without [llm] enabled = true the TUI never calls Claude, though a
// backend is there: an hour of ticks makes no call.
func TestNoBackgroundSuggestWithoutOptIn(t *testing.T) {
	e := apptest.New(t, apptest.Rules)
	e.Seed(t, apptest.DaySpans()...)
	e.Heartbeat(t, e.Clock.UnixMilli())
	e.WriteConfig(t, "")
	f := &fakeSuggester{answer: studyTock}
	m := newModelWith(t, e, f)
	for i := 0; i < 70; i++ {
		e.Clock = e.Clock.Add(time.Minute)
		m = step(t, m, tickMsg{})
	}
	if f.calls != 0 {
		t.Fatalf("%d Claude calls without opt-in", f.calls)
	}
}
