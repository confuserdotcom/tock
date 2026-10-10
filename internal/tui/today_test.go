package tui

import (
	"strings"
	"testing"
	"time"

	"tock/internal/apptest"
)

func TestTodayShowsTheDay(t *testing.T) {
	m, _ := seeded(t)
	mustContain(t, m,
		"tock   1 today   2 week   3 inbox 3", "Sat 10 Oct 15:00 · trackd down",
		"trackd is not running",
		"Today  tracked 2h54 · deep 2h14 · unlabelled 28m · distract 12m · idle 41m",
		"Code            2h14  ████████████  tock 1h40",
		"Unclassified     28m  ██▌           3 keys → 3 inbox",
		"09:00–10:40   1h40  Code / tock         WezTerm     tock │ ✳ x",
		"11:41–12:22    41m  idle",
	)
	if l := line(t, m, "14:50–15:00"); !strings.HasPrefix(l, "▶") || !strings.Contains(l, "—") {
		t.Fatalf("cursor must start on the open last row: %q", l)
	}
	if l := line(t, m, " deep       "); !strings.HasPrefix(l, " deep       ████░░ ") {
		// Code: 09h 60m, 10h 40m+20m, 11h 14m; two cells per hour from 09
		t.Fatalf("heatmap deep row: %q", l)
	}
}

func TestRefreshShowsNewSpansAndFollows(t *testing.T) {
	m, e := seeded(t)
	e.Seed(t, apptest.Span{ID: 9, Start: "15:00", End: "", Bundle: "term", App: "WezTerm", Title: "tock │ new"})
	e.Clock = e.Clock.Add(2 * time.Minute)
	e.Heartbeat(t, e.Clock.UnixMilli())
	m = step(t, m, tickMsg{})
	mustNotContain(t, m, "trackd is not running")
	mustContain(t, m, "trackd ok", "15:00–15:02     2m  Code / tock")
	if l := line(t, m, "15:00–15:02"); !strings.HasPrefix(l, "▶") {
		t.Fatalf("cursor on the last row must follow new rows: %q", l)
	}
}

func TestCursorStaysOnItsRowAcrossRefresh(t *testing.T) {
	m, e := seeded(t)
	m = press(t, m, "k", "k")
	if l := line(t, m, "11:41–12:22"); !strings.HasPrefix(l, "▶") {
		t.Fatalf("k k: %q", l)
	}
	e.Seed(t, apptest.Span{ID: 9, Start: "15:00", End: "", Bundle: "term", App: "WezTerm", Title: "x"})
	e.Clock = e.Clock.Add(time.Minute)
	m = step(t, m, tickMsg{})
	if l := line(t, m, "11:41–12:22"); !strings.HasPrefix(l, "▶") {
		t.Fatalf("cursor moved on refresh: %q", l)
	}
}

func TestTagRow(t *testing.T) {
	m, _ := seeded(t)
	m = press(t, m, "t")
	mustContain(t, m, "Category", "Communication")
	m = typeText(t, m, "comm")
	mustNotContain(t, m, "Entertainment")
	m = press(t, m, "enter")
	mustContain(t, m, "Project for Communication", "(no project)", "ATLAS")
	mustNotContain(t, m, "Old") // archived
	m = press(t, m, "enter")
	mustContain(t, m, "tagged 1 span (10m) → Communication", "3 inbox 2")
	if l := line(t, m, "14:50–15:00"); !strings.Contains(l, "Communication") {
		t.Fatalf("row not relabelled: %q", l)
	}
}

func TestProjectOnly(t *testing.T) {
	m, _ := seeded(t)
	m = press(t, m, "g", "j") // the ATLAS │ y row (Code, no project)
	m = press(t, m, "p")
	mustContain(t, m, "Project")
	mustNotContain(t, m, "(no project)")
	m = typeText(t, m, "at")
	m = press(t, m, "enter")
	mustContain(t, m, "tagged 1 span (34m) → project ATLAS")
	if l := line(t, m, "10:40–11:14"); !strings.Contains(l, "Code / ATLAS") {
		t.Fatalf("project not set: %q", l)
	}
}

func TestIdleRowsRefuseLabels(t *testing.T) {
	m, _ := seeded(t)
	m = press(t, m, "k", "k", "t")
	mustContain(t, m, "idle time has no label")
	if m.picker != nil {
		t.Fatal("picker opened on an idle row")
	}
}

func TestPickerEscCancels(t *testing.T) {
	m, _ := seeded(t)
	m = press(t, m, "t", "esc")
	if m.picker != nil {
		t.Fatal("esc must close the picker")
	}
	mustContain(t, m, "Today")
}

func TestFilter(t *testing.T) {
	m, _ := seeded(t)
	m = press(t, m, "/")
	m = typeText(t, m, "arc")
	m = press(t, m, "enter")
	mustContain(t, m, "11:14–11:26", "12:22–12:25")
	mustNotContain(t, m, "09:00–10:40", "14:50–15:00")
	m = press(t, m, "/", "esc")
	mustContain(t, m, "09:00–10:40")
}

func TestDayNavigation(t *testing.T) {
	m, e := seeded(t)
	e.Seed(t, apptest.Span{ID: 9, Start: "09:00", End: "10:00", Bundle: "term", App: "WezTerm", Title: "tock │ y", Day: -1})
	m = press(t, m, "h")
	mustContain(t, m, "Fri 9 Oct  tracked 1h00", "09:00–10:00")
	m = press(t, m, "l")
	mustContain(t, m, "Today  tracked 2h54")
	m = press(t, m, "l")
	mustContain(t, m, "Today  tracked 2h54")
	m = press(t, m, "h", "h", ".")
	mustContain(t, m, "Today  tracked 2h54")
}

func TestMidnightMovesToTheNewDay(t *testing.T) {
	m, e := seeded(t)
	e.Clock = time.Date(2026, 10, 11, 0, 5, 0, 0, apptest.Zone)
	m = step(t, m, tickMsg{})
	// The open Mail span (since 14:50) now counts from midnight.
	mustContain(t, m, "Sun 11 Oct 00:05", "Today  tracked 5m", "00:00–00:05")
}

func TestTooSmall(t *testing.T) {
	m, _ := seeded(t)
	m = step(t, m, struct{}{}) // no-op
	next, _ := m.Update(windowSize(59, 30))
	if got := screen(next.(Model)); got != "terminal too small (60×16 needed)" {
		t.Fatalf("got %q", got)
	}
}
