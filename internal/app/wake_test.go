package app

import (
	"errors"
	"strings"
	"testing"

	"tock/internal/apptest"
	"tock/internal/classify"
	"tock/internal/focus"
	"tock/internal/store"
)

func TestSetPaper(t *testing.T) {
	e := distracted(t)
	a := open(t, e)
	e.Clock = apptest.Now
	if err := a.SetPaper(0); !errors.Is(err, store.ErrNoSession) {
		t.Fatalf("no session: %v", err)
	}
	s := startAt(t, e, a, "14:50", 25)
	if err := a.SetPaper(s.ID + 1); !errors.Is(err, store.ErrNoSession) {
		t.Fatalf("other session: %v", err)
	}
	if err := a.SetPaper(0); err != nil {
		t.Fatal(err)
	}
	st, err := a.focusState()
	if err != nil || st.PaperSession != s.ID {
		t.Fatalf("paper = %d, %v", st.PaperSession, err)
	}
}

func TestStartSessionPaper(t *testing.T) {
	e := distracted(t)
	a := open(t, e)
	e.Clock = apptest.Now
	s, err := a.StartSessionPaper(25, true)
	if err != nil {
		t.Fatal(err)
	}
	if st, _ := a.focusState(); st.PaperSession != s.ID {
		t.Fatalf("paper = %d, want %d", st.PaperSession, s.ID)
	}
}

// This is work files the site under the label for good, and tock undo
// takes it back.
func TestWakeIsWork(t *testing.T) {
	e := distracted(t)
	a := open(t, e)
	e.Clock = apptest.Now
	key := "Arc · youtube.com"
	rule, err := a.WakeIsWork(key, classify.Want{Category: "Study"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(rule, "youtube") {
		t.Fatalf("rule = %q", rule)
	}
	if got := labelAt(t, a, "14:56"); got.Category != "Study" {
		t.Fatalf("label after = %+v", got)
	}
	if _, err := a.Undo(); err != nil {
		t.Fatal(err)
	}
	if got := labelAt(t, a, "14:56"); got.Category != "Entertainment" {
		t.Fatalf("label after undo = %+v", got)
	}
	if _, err := a.WakeIsWork(key, classify.Want{}); err == nil {
		t.Fatal("no label: no error")
	}
}

func TestEnqueueWake(t *testing.T) {
	e := distracted(t)
	a := open(t, e)
	e.Clock = apptest.Now
	ev := focus.Event{Kind: focus.Notify, Topic: focus.TopicWake, Title: focus.TitleWake, Body: "b",
		OffFrom: 123, Key: "Arc · youtube.com", Target: classify.Want{Category: "Study", Project: "p"},
		Sess: focus.Session{ID: 4}}
	if err := a.Enqueue(ev); err != nil {
		t.Fatal(err)
	}
	ns, err := a.TakeNotices()
	if err != nil || len(ns) != 1 {
		t.Fatalf("notices = %v, %v", ns, err)
	}
	_, d, err := a.Notice(ns[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	if d.From != 123 || d.Key != ev.Key || d.Category != "Study" || d.Project != "p" || d.Session != 4 || d.SnoozeMin != 10 || d.Idle {
		t.Fatalf("data = %+v", d)
	}
}

func labelAt(t *testing.T, a *App, hhmm string) classify.Label {
	t.Helper()
	_, l, _ := rowAt(t, a, hhmm)
	return l
}
