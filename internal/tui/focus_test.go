package tui

import (
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
)

func TestFocusTabStartAndStop(t *testing.T) {
	m, e := seeded(t)
	e.Heartbeat(t, e.Clock.UnixMilli())
	m = press(t, m, "4")
	mustContain(t, m, "focus", "no session · s starts one", "Today", "Earlier this week")
	m = press(t, m, "s")
	mustContain(t, m, "Start a focus session", "25 min", "50 min", "90 min", "custom")
	m = press(t, m, "enter")
	mustContain(t, m, "session started: 25 min", "25:00 left of 25m", "◷ 25:00")
	m = press(t, m, "s")
	mustContain(t, m, "a session is already running")
	e.Clock = e.Clock.Add(5 * time.Minute)
	m = step(t, m, tickMsg{})
	mustContain(t, m, "20:00 left of 25m", "◷ 20:00")
	m = press(t, m, "x")
	mustContain(t, m, "stopped: 15:00  manual    5m  deep   0%  0 int", "no session · s starts one")
	mustContain(t, m, "15:00  manual    5m")
	mustNotContain(t, m, "◷")
}

func TestFocusTabCustomLength(t *testing.T) {
	m, e := seeded(t)
	e.Heartbeat(t, e.Clock.UnixMilli())
	m = press(t, m, "4", "s", "0", "enter")
	mustContain(t, m, "length 1 to 240 minutes", "Start a focus session")
	m = press(t, m, "backspace", "4", "5", "enter")
	mustContain(t, m, "session started: 45 min", "45:00 left of 45m")
}

func TestFocusTabEscAndStopWithoutSession(t *testing.T) {
	m, e := seeded(t)
	e.Heartbeat(t, e.Clock.UnixMilli())
	m = press(t, m, "4", "s", "esc")
	mustNotContain(t, m, "Start a focus session")
	m = press(t, m, "x")
	mustContain(t, m, "no session running · s starts one")
}

func TestFocusStartWithTrackdDown(t *testing.T) {
	m, _ := seeded(t)
	m = press(t, m, "4", "s", "enter")
	mustContain(t, m, "session started: 25 min · trackd down, no alerts")
}

// The header countdown redraws every second on any tab while a session runs.
func TestFocusClockRunsOnEveryTab(t *testing.T) {
	m, e := seeded(t)
	e.Heartbeat(t, e.Clock.UnixMilli())
	m = press(t, m, "4", "s", "enter", "1")
	ticks := 0
	m.clockTick = func() tea.Cmd { ticks++; return func() tea.Msg { return nil } }
	next, cmd := m.Update(clockMsg{})
	if cmd == nil || ticks != 1 {
		t.Fatalf("clock on today tab: cmd=%v ticks=%d, want a next tick", cmd, ticks)
	}
	m = next.(Model)
	m = press(t, m, "4", "x")
	if _, cmd := m.Update(clockMsg{}); cmd != nil {
		t.Fatal("clock must stop once no session runs")
	}
}
