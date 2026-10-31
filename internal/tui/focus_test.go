package tui

import (
	"fmt"
	"strconv"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"tock/internal/apptest"
	"tock/internal/focus"
	"tock/internal/store"
)

func TestFocusTabStartAndStop(t *testing.T) {
	m, e := seeded(t)
	e.Heartbeat(t, e.Clock.UnixMilli())
	m = press(t, m, "4")
	mustContain(t, m, "focus", "No session running.", "Today", "Earlier this week")
	m = press(t, m, "s")
	mustContain(t, m, "Start a focus session", "25 min", "50 min", "90 min", "custom")
	m = press(t, m, "enter")
	mustContain(t, m, "session started: 25 min", "left of 25m", "◷ 25:00")
	m = press(t, m, "s")
	mustContain(t, m, "a session is already running")
	e.Clock = e.Clock.Add(5 * time.Minute)
	m = step(t, m, tickMsg{})
	mustContain(t, m, "left of 25m", "◷ 20:00")
	m = press(t, m, "x")
	mustContain(t, m, "stopped: 15:00  manual    5m  deep   0%  0 int", "No session running.")
	mustContain(t, m, "15:00  manual    5m")
	mustNotContain(t, m, "◷")
}

func TestFocusTabCustomLength(t *testing.T) {
	m, e := seeded(t)
	e.Heartbeat(t, e.Clock.UnixMilli())
	m = press(t, m, "4", "s", "0", "enter")
	mustContain(t, m, "length 1 to 240 minutes", "Start a focus session")
	m = press(t, m, "backspace", "4", "5", "enter")
	mustContain(t, m, "session started: 45 min", "left of 45m")
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

// seedManual adds a finished manual session [from, to) on day (relative to the
// test clock's day) with counters, and returns its id.
func seedManual(t *testing.T, e *apptest.Env, from, to string, day int, c store.Session) int64 {
	t.Helper()
	st, err := store.Open(e.DB)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	start, end := apptest.At(t, from, day), apptest.At(t, to, day)
	s, err := st.StartSession(start, end-start)
	if err != nil {
		t.Fatal(err)
	}
	c.ID, c.EndMS = s.ID, end
	if ok, err := st.EndSession(c); err != nil || !ok {
		t.Fatalf("end session: %v %v", ok, err)
	}
	return s.ID
}

// seedAuto adds a finished auto session.
func seedAuto(t *testing.T, e *apptest.Env, from, to string, day int, c store.Session) {
	t.Helper()
	st, err := store.Open(e.DB)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	c.Kind, c.StartMS, c.EndMS = store.SessionAuto, apptest.At(t, from, day), apptest.At(t, to, day)
	if _, err := st.AddAutoSession(c); err != nil {
		t.Fatal(err)
	}
}

// seedRating rates session id in the database.
func seedRating(t *testing.T, e *apptest.Env, id int64, rating int, note string) {
	t.Helper()
	st, err := store.Open(e.DB)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	if err := st.RateSession(id, rating, &note, apptest.At(t, "14:00", 0)); err != nil {
		t.Fatal(err)
	}
}

const min1 = 60_000

// a manual session with counters: 50 minutes, 42 of them deep.
var counters = store.Session{DeepMS: 42 * min1, DistractMS: 3 * min1, LongestMS: 32 * min1, Interruptions: 1, Switches: 4}

func TestFocusCardCursor(t *testing.T) {
	m, e := seeded(t)
	e.Heartbeat(t, e.Clock.UnixMilli())
	seedManual(t, e, "09:10", "10:00", 0, counters)
	seedManual(t, e, "07:00", "07:50", 0, counters)
	seedManual(t, e, "10:00", "10:40", -2, counters)
	m = press(t, m, "4")
	mustContain(t, m, "Last session", "09:10–10:00")
	mustNotContain(t, m, "Session · ")
	m = press(t, m, "j")
	mustContain(t, m, "Session · Sat 07:00", "07:00–07:50")
	if l := line(t, m, "07:00  manual"); !strings.Contains(l, "▸") {
		t.Fatalf("selected row lacks the cursor: %q", l)
	}
	m = press(t, m, "down")
	mustContain(t, m, "Session · Thu 10:00", "10:00–10:40")
	m = press(t, m, "j", "j")
	mustContain(t, m, "Session · Thu 10:00")
	m = press(t, m, "k", "up")
	mustContain(t, m, "Last session", "09:10–10:00")
	m = press(t, m, "k")
	mustContain(t, m, "Last session")
	m = press(t, m, "j")
	mustContain(t, m, "Session · Sat 07:00")
	seedManual(t, e, "12:00", "12:30", 0, counters)
	m = press(t, m, "r")
	mustContain(t, m, "Last session", "12:00–12:30")
}

func TestFocusCardStates(t *testing.T) {
	m, e := seeded(t)
	e.Heartbeat(t, e.Clock.UnixMilli())
	mustContain(t, press(t, m, "4"), "Last session", "No finished session yet.",
		"A finished session gets a quality score and a card here.")

	seedManual(t, e, "09:10", "10:00", 0, counters)
	m = press(t, step(t, m, windowSize(100, 50)), "4")
	row := m.focus.rows()[0]
	q := focus.Quality(row, m.focus.data.Weights)
	mustContain(t, m, "quality "+strconv.Itoa(q.Score), "84% deep", q.Line(), "unrated",
		"categories", "apps", "1 interruption · longest run 32m", "R rate", "n note", "j/k other session")
	mustNotContain(t, m, "auto-detected", "f refile")

	// A session over a quiet day has nothing to list.
	seedManual(t, e, "10:00", "10:40", -2, store.Session{DeepMS: 30 * min1, LongestMS: 20 * min1, Switches: 3})
	m = press(t, m, "r", "j", "j")
	mustContain(t, m, "none")
	if l := line(t, m, "categories"); !strings.Contains(l, "none") {
		t.Fatalf("categories row = %q, want none", l)
	}
	if l := line(t, m, "apps"); !strings.Contains(l, "none") {
		t.Fatalf("apps row = %q, want none", l)
	}
}

func TestFocusCardAutoAndLegacy(t *testing.T) {
	m, e := seeded(t)
	e.Heartbeat(t, e.Clock.UnixMilli())
	seedAuto(t, e, "13:00", "13:40", 0, store.Session{DeepMS: 38 * min1, LongestMS: 30 * min1, Switches: -1})
	m = press(t, m, "4")
	mustContain(t, m, "auto-detected · longest run 30m", "calm n/a (auto)", "steady n/a", "unrated")
	mustNotContain(t, m, "interruption")
}

func TestFocusCardRefileKeycap(t *testing.T) {
	m, e := seeded(t)
	e.Heartbeat(t, e.Clock.UnixMilli())
	seedManual(t, e, "10:20", "11:00", 0, store.Session{DeepMS: 30 * min1, LongestMS: 20 * min1, Switches: 3})
	m = press(t, step(t, m, windowSize(100, 50)), "4")
	mustContain(t, m, "f refile → tock")
}

func TestFocusRowColumns(t *testing.T) {
	m, e := seeded(t)
	e.Heartbeat(t, e.Clock.UnixMilli())
	a := seedManual(t, e, "09:10", "10:00", 0, counters)
	seedManual(t, e, "07:00", "07:50", 0, counters)
	seedRating(t, e, a, 4, "")
	m = press(t, m, "4")
	rows := m.focus.rows()
	w := m.focus.data.Weights
	rated := fmt.Sprintf("Q%d  ★4", focus.Quality(rows[0], w).Score)
	if l := line(t, m, "09:10  manual"); !strings.HasSuffix(strings.TrimRight(l, " "), rated) {
		t.Fatalf("rated row = %q, want it to end with %q", l, rated)
	}
	unrated := fmt.Sprintf("Q%d   ·", focus.Quality(rows[1], w).Score)
	if l := line(t, m, "07:00  manual"); !strings.HasSuffix(strings.TrimRight(l, " "), unrated) {
		t.Fatalf("unrated row = %q, want it to end with %q", l, unrated)
	}
}

func TestFocusCardShortBody(t *testing.T) {
	m, e := seeded(t)
	e.Heartbeat(t, e.Clock.UnixMilli())
	seedManual(t, e, "09:10", "10:00", 0, counters)
	m = press(t, m, "4")
	card := func(h int) []string {
		var out []string
		in := false
		for _, l := range m.focus.view(100, h, m.now()) {
			l = ansi.Strip(l)
			if w := ansi.StringWidth(l); w > 100 {
				t.Fatalf("h=%d: line wider than the screen (%d): %q", h, w, l)
			}
			in = in || strings.Contains(l, "Last session")
			if in {
				out = append(out, l)
			}
			if in && strings.Contains(l, "╰") {
				break
			}
		}
		return out
	}
	blank := func(ls []string) (n int) {
		for _, l := range ls[1 : len(ls)-1] {
			if strings.TrimSpace(strings.Trim(strings.TrimSpace(l), "│")) == "" {
				n++
			}
		}
		return n
	}
	has := func(ls []string, s string) bool { return strings.Contains(strings.Join(ls, "\n"), s) }
	full, h33, h31, h30 := card(34), card(33), card(31), card(30)
	if blank(full) != 2 || !has(full, "apps") || !has(full, "categories") {
		t.Fatalf("h=34 must be the full card:\n%s", strings.Join(full, "\n"))
	}
	if blank(h33) != 0 || !has(h33, "apps") || !has(h33, "categories") {
		t.Fatalf("h=33 drops only the blank lines:\n%s", strings.Join(h33, "\n"))
	}
	if blank(h31) != 0 || has(h31, "apps") || !has(h31, "categories") {
		t.Fatalf("h=31 also drops apps:\n%s", strings.Join(h31, "\n"))
	}
	if blank(h30) != 0 || has(h30, "apps") || has(h30, "categories") {
		t.Fatalf("h=30 also drops categories:\n%s", strings.Join(h30, "\n"))
	}
}
