package tui

import (
	"encoding/json"
	"strings"
	"testing"

	"tock/internal/app"
	"tock/internal/apptest"
)

func TestSummaryLine(t *testing.T) {
	e := apptest.New(t, apptest.Rules)
	e.Seed(t, apptest.DaySpans()...)
	m := newModel(t, e)
	if s := screen(m); strings.Contains(s, "summary  ") {
		t.Fatalf("summary line before any summary:\n%s", s)
	}
	m.src.do(func(a *app.App) {
		b, err := a.SummaryBatch(apptest.Now)
		if err != nil {
			t.Fatal(err)
		}
		raw := json.RawMessage(`{"day":"You coded tock all morning.","blocks":[]}`)
		if _, err := a.SaveSummary(apptest.Now, b, "m", raw); err != nil {
			t.Fatal(err)
		}
	})
	m = press(t, m, "r")
	if s := screen(m); !strings.Contains(s, "summary  You coded tock all morning.") {
		t.Fatalf("today lacks the summary line:\n%s", s)
	}
}
