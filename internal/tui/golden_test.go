package tui

import (
	"bytes"
	"testing"
	"time"

	"github.com/charmbracelet/x/exp/golden"
	"github.com/charmbracelet/x/exp/teatest/v2"

	"tock/internal/app"
	"tock/internal/apptest"
	"tock/internal/store"
)

// Phase 3 criterion 4: each tab, the picker, the learn dialog, the score
// panel and the trackd-down banner match their snapshots at 100×30 without
// colour.
// Regenerate with: go test ./internal/tui -run Golden -update

func TestGoldenToday(t *testing.T) {
	m, e := seeded(t)
	e.Heartbeat(t, e.Clock.UnixMilli())
	m = step(t, m, tickMsg{})
	golden.RequireEqual(t, screen(m))
}

// Phase 3 criterion 3: with trackd stopped the history still shows, under
// the banner.
func TestGoldenTodayTrackdDown(t *testing.T) {
	m, _ := seeded(t)
	golden.RequireEqual(t, screen(m))
}

// Phase 4 criterion 2: i on today explains the score.
func TestGoldenScore(t *testing.T) {
	m, e := seeded(t)
	e.Heartbeat(t, e.Clock.UnixMilli())
	m = press(t, m, "r", "i")
	golden.RequireEqual(t, screen(m))
}

func TestGoldenWeek(t *testing.T) {
	m, e := seeded(t)
	e.Heartbeat(t, e.Clock.UnixMilli())
	e.Seed(t, apptest.Span{ID: 9, Start: "09:00", End: "11:00", Bundle: "term", App: "WezTerm", Title: "tock │ y", Day: -5})
	m = press(t, m, "2")
	golden.RequireEqual(t, screen(m))
}

func TestGoldenInbox(t *testing.T) {
	m, e := seeded(t)
	e.Heartbeat(t, e.Clock.UnixMilli())
	m = press(t, m, "3")
	golden.RequireEqual(t, screen(m))
}

// Phase 6: the inbox with one suggestion.
func TestGoldenInboxSuggested(t *testing.T) {
	m, _ := withSuggestions(t, &fakeSuggester{answer: studyTock})
	m = press(t, m, "3")
	golden.RequireEqual(t, screen(m))
}

func TestGoldenPicker(t *testing.T) {
	m, e := seeded(t)
	e.Heartbeat(t, e.Clock.UnixMilli())
	m = press(t, m, "r", "t")
	m = typeText(t, m, "c")
	golden.RequireEqual(t, screen(m))
}

func TestGoldenLearn(t *testing.T) {
	m, e := seeded(t)
	e.Heartbeat(t, e.Clock.UnixMilli())
	m = press(t, m, "3", "enter")
	m = typeText(t, m, "stu")
	m = press(t, m, "enter", "enter")
	golden.RequireEqual(t, screen(m))
}

// TestProgramRuns drives the real Bubble Tea program: it loads, renders the
// today tab and quits on q with the same screen as the snapshot.
func TestProgramRuns(t *testing.T) {
	e := apptest.New(t, apptest.Rules)
	e.Seed(t, apptest.DaySpans()...)
	a, err := app.Open(app.Paths{DB: e.DB, Rules: e.Rules, Learned: e.Learned, Config: e.Config}, e.NowFunc())
	if err != nil {
		t.Fatal(err)
	}
	defer a.Close()
	m := New(a, e.NowFunc())
	m.tick = nil
	m.clockTick = nil
	tm := teatest.NewTestModel(t, m, teatest.WithInitialTermSize(100, 30))
	teatest.WaitFor(t, tm.Output(), func(b []byte) bool { return bytes.Contains(b, []byte("Today")) },
		teatest.WithDuration(5*time.Second))
	tm.Send(key("q"))
	final := tm.FinalModel(t, teatest.WithFinalTimeout(5*time.Second)).(Model)
	want, _ := seeded(t)
	if got := screen(final); got != screen(want) {
		t.Fatalf("program screen:\n%s\nwant:\n%s", got, screen(want))
	}
}

// Phase 5 criterion 1: the running session's countdown, with history.
func TestGoldenFocus(t *testing.T) {
	m, e := seeded(t)
	e.Heartbeat(t, e.Clock.UnixMilli())
	st, err := store.Open(e.DB)
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range []store.Session{
		{Kind: store.SessionAuto, StartMS: apptest.At(t, "09:00", 0), EndMS: apptest.At(t, "11:14", 0), DeepMS: 134 * 60_000},
		{Kind: store.SessionAuto, StartMS: apptest.At(t, "10:00", -2), EndMS: apptest.At(t, "10:40", -2), DeepMS: 38 * 60_000},
	} {
		if _, err := st.AddAutoSession(s); err != nil {
			t.Fatal(err)
		}
	}
	st.Close()
	m = press(t, m, "4", "s", "enter")
	golden.RequireEqual(t, screen(m))
}

func TestGoldenFocusStart(t *testing.T) {
	m, e := seeded(t)
	e.Heartbeat(t, e.Clock.UnixMilli())
	m = press(t, m, "4", "s")
	golden.RequireEqual(t, screen(m))
}

// Phase 7: the rules tab.
func TestGoldenRules(t *testing.T) {
	m, e := seeded(t)
	e.Heartbeat(t, e.Clock.UnixMilli())
	m = press(t, m, "5")
	golden.RequireEqual(t, screen(m))
}

func TestGoldenHelp(t *testing.T) {
	m, e := seeded(t)
	e.Heartbeat(t, e.Clock.UnixMilli())
	m = press(t, m, "r", "?")
	golden.RequireEqual(t, screen(m))
}

func TestGoldenPalette(t *testing.T) {
	m, e := seeded(t)
	e.Heartbeat(t, e.Clock.UnixMilli())
	m = press(t, m, "r", ":")
	golden.RequireEqual(t, screen(m))
}

func TestGoldenTodayEmpty(t *testing.T) {
	e := apptest.New(t, apptest.Rules)
	e.Seed(t, apptest.Span{ID: 1, Start: "09:00", End: "10:00", Bundle: "term", App: "WezTerm", Title: "x", Day: -1})
	e.Heartbeat(t, e.Clock.UnixMilli())
	m := newModel(t, e)
	golden.RequireEqual(t, screen(m))
}

func TestGoldenTodaySuggested(t *testing.T) {
	m, _ := withSuggestions(t, &fakeSuggester{answer: studyTock})
	m = press(t, m, "1", "r")
	golden.RequireEqual(t, screen(m))
}

func TestGoldenInboxZero(t *testing.T) {
	e := apptest.New(t, apptest.Rules)
	e.Seed(t, apptest.Span{ID: 1, Start: "09:00", End: "10:00", Bundle: "term", App: "WezTerm", Title: "tock │ x"})
	e.Heartbeat(t, e.Clock.UnixMilli())
	m := newModel(t, e)
	m = press(t, m, "3")
	golden.RequireEqual(t, screen(m))
}

func TestGoldenFocusIdle(t *testing.T) {
	m, e := seeded(t)
	e.Heartbeat(t, e.Clock.UnixMilli())
	m = press(t, m, "r", "4")
	golden.RequireEqual(t, screen(m))
}
