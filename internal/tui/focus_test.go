package tui

import (
	"fmt"
	"strconv"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"tock/internal/app"
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
	mustContain(t, m, "auto-detected · longest run 30m", "switches n/a", "unrated")
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

func TestFocusRateKeys(t *testing.T) {
	m, e := seeded(t)
	e.Heartbeat(t, e.Clock.UnixMilli())
	id := seedManual(t, e, "09:10", "10:00", 0, counters)
	m = press(t, m, "4", "R")
	mustContain(t, m, "Rate this session", "★★★★★", "★☆☆☆☆", "deep, nothing pulled", "lost it")
	if l := line(t, m, "★★★★★"); !strings.Contains(l, "▸") {
		t.Fatalf("an unrated session must start on ★5: %q", l)
	}
	m = press(t, m, "4")
	mustContain(t, m, "rated 09:10 session ★4", "Last session", "★★★★☆")
	mustNotContain(t, m, "Rate this session")
	if m.tab != tabFocus {
		t.Fatalf("the digit switched tabs: tab = %d", m.tab)
	}
	var got focus.Session
	m.src.do(func(a *app.App) { got, _ = a.SessionByID(id) })
	if got.Rating != 4 {
		t.Fatalf("stored rating = %d, want 4", got.Rating)
	}
	m = press(t, m, "R")
	if l := line(t, m, "★★★★☆  4"); !strings.Contains(l, "▸") {
		t.Fatalf("a rated session must start on its rating: %q", l)
	}
	m = press(t, m, "5")
	mustContain(t, m, "rating changed to ★5", "★★★★★")
	m = press(t, m, "R", "esc")
	mustNotContain(t, m, "Rate this session")
	m.src.do(func(a *app.App) { got, _ = a.SessionByID(id) })
	if got.Rating != 5 {
		t.Fatalf("esc changed the rating to %d", got.Rating)
	}
	// enter picks the highlighted option too
	m = press(t, m, "R", "j", "j", "enter")
	mustContain(t, m, "rating changed to ★3")
}

func TestFocusNoteCap(t *testing.T) {
	m, e := seeded(t)
	e.Heartbeat(t, e.Clock.UnixMilli())
	seedManual(t, e, "09:10", "10:00", 0, counters)
	m = press(t, m, "4", "n")
	mustContain(t, m, "Note for 09:10 session", "One line, up to 200 characters.")
	m = typeText(t, m, strings.Repeat("a", 205))
	if n := len([]rune(m.ask.text)); n != 200 {
		t.Fatalf("note field has %d runes, want 200", n)
	}
	m = press(t, m, "ctrl+u")
	m = step(t, m, tea.KeyPressMsg{Code: 'a', Text: "a\nb"})
	if m.ask.text != "a b" {
		t.Fatalf("pasted text = %q, want \"a b\"", m.ask.text)
	}
	m = press(t, m, "enter")
	mustContain(t, m, "note saved", "a b")
	m = press(t, m, "n")
	if m.ask.text != "a b" {
		t.Fatalf("the dialog must be prefilled with the note, got %q", m.ask.text)
	}
	m = press(t, m, "ctrl+u", "enter")
	mustContain(t, m, "note cleared")
	mustNotContain(t, m, "note        a b")
	// other dialogs keep the 80-rune cap
	m = press(t, m, "esc", "4")
	m.ask = &askDlg{title: "x", done: func(m Model, text string) (tea.Model, tea.Cmd) { return m, nil }}
	m = typeText(t, m, strings.Repeat("b", 90))
	if n := len([]rune(m.ask.text)); n != 80 {
		t.Fatalf("default cap = %d, want 80", n)
	}
}

func TestFocusRefilePick(t *testing.T) {
	m, e := seeded(t)
	e.Heartbeat(t, e.Clock.UnixMilli())
	id := seedManual(t, e, "10:20", "11:00", 0, store.Session{DeepMS: 30 * min1, LongestMS: 20 * min1, Switches: 3})
	m = press(t, step(t, m, windowSize(100, 50)), "4", "f")
	mustContain(t, m, "Refile session 10:20–11:00 (40m) to", "tock", "top project")
	m = press(t, m, "enter")
	if m.flash != "refiled 40m to tock (u undo)" {
		t.Fatalf("flash = %q", m.flash)
	}
	mustContain(t, m, "refiled 40m to tock")
	undoable := func() bool {
		var ok bool
		m.src.do(func(a *app.App) { ok, _ = a.RefileUndoable(id) })
		return ok
	}
	if !undoable() {
		t.Fatal("the refile left no undo entry")
	}
	m = press(t, m, "u")
	mustContain(t, m, "undid: refile session")
	if undoable() {
		t.Fatal("u did not revert the refile")
	}
	// any project can be chosen, not only the top one
	m = press(t, m, "f")
	m = typeText(t, m, "atl")
	m = press(t, m, "enter")
	if m.flash != "refiled 40m to ATLAS (u undo)" {
		t.Fatalf("flash = %q", m.flash)
	}
}

func TestFocusNoSession(t *testing.T) {
	m, e := seeded(t)
	e.Heartbeat(t, e.Clock.UnixMilli())
	m = press(t, m, "4")
	for _, k := range []string{"R", "n", "f"} {
		m = press(t, m, k)
		mustContain(t, m, "no finished session yet · s starts one")
		mustNotContain(t, m, "Rate this session", "Note for", "Refile session")
		if m.choose != nil || m.ask != nil || m.picker != nil {
			t.Fatalf("%s opened a dialog with nothing to act on", k)
		}
	}
	// a running session is never the target
	m = press(t, m, "s", "enter", "R", "n", "f")
	mustContain(t, m, "no finished session yet · s starts one")
	if m.choose != nil || m.ask != nil || m.picker != nil {
		t.Fatal("a key acted on the running session")
	}
}

func TestFocusRatePersists(t *testing.T) {
	m, e := seeded(t)
	e.Heartbeat(t, e.Clock.UnixMilli())
	seedManual(t, e, "09:10", "10:00", 0, counters)
	m = press(t, m, "4", "R", "4")
	mustContain(t, m, "rated 09:10 session ★4")
	again := press(t, newModel(t, e), "4")
	mustContain(t, again, "★★★★☆")
	if l := line(t, again, "09:10  manual"); !strings.Contains(l, "★4") {
		t.Fatalf("row after restart = %q", l)
	}
}

func TestFocusPaletteRates(t *testing.T) {
	m, e := seeded(t)
	e.Heartbeat(t, e.Clock.UnixMilli())
	m = press(t, m, ":")
	mustNotContain(t, m, "rate last session")
	m = press(t, m, "esc")
	id := seedManual(t, e, "09:10", "10:00", 0, counters)
	seedManual(t, e, "07:00", "07:50", 0, counters)
	m = press(t, m, "r", ":")
	m = typeText(t, m, "last session")
	mustContain(t, m, "rate last session 5 stars", "rate last session 1 star", "note on last session", "refile last session")
	m = typeText(t, m, " 3")
	m = press(t, m, "enter")
	var got focus.Session
	m.src.do(func(a *app.App) { got, _ = a.SessionByID(id) })
	if got.Rating != 3 {
		t.Fatalf("palette rated session %d, want the newest rated 3", got.Rating)
	}
}

// snoozeHM is the clock the snooze tests expect: the test clock plus the
// default 10 minute snooze.
const snoozeHM = "15:10"

func TestFocusSnoozeKey(t *testing.T) {
	m, e := seeded(t)
	e.Heartbeat(t, e.Clock.UnixMilli())
	m = press(t, m, "4", "z")
	mustContain(t, m, "no session running · s starts one")
	m = press(t, m, "s", "enter", "z")
	mustContain(t, m, "distraction notices snoozed until "+snoozeHM)
	if got := m.focus.data.SnoozeUntilMS; got != e.Clock.Add(10*time.Minute).UnixMilli() {
		t.Fatalf("SnoozeUntilMS = %d, want the clock plus 10 min", got)
	}
	m = press(t, m, "z")
	mustContain(t, m, "distraction notices back on")
	if got := m.focus.data.SnoozeUntilMS; got != 0 {
		t.Fatalf("SnoozeUntilMS after resume = %d, want 0", got)
	}
}

func TestFocusSnoozedCard(t *testing.T) {
	m, e := seeded(t)
	e.Heartbeat(t, e.Clock.UnixMilli())
	m = press(t, m, "4", "s", "enter")
	mustContain(t, m, "snooze notices", "stop session")
	mustNotContain(t, m, "notices snoozed until", "resume notices")
	m = press(t, m, "z")
	mustContain(t, m, "notices snoozed until "+snoozeHM, "resume notices")
	mustNotContain(t, m, "z snooze notices")
	m = press(t, m, "z")
	mustContain(t, m, "snooze notices")
	mustNotContain(t, m, "notices snoozed until", "resume notices")
}

func TestFocusUrgeLine(t *testing.T) {
	m, e := seeded(t)
	e.Heartbeat(t, e.Clock.UnixMilli())
	id := seedManual(t, e, "09:10", "10:00", 0, counters)
	m = step(t, m, windowSize(100, 50))
	m = press(t, m, "4")
	mustNotContain(t, m, "urges")

	st, err := store.Open(e.DB)
	if err != nil {
		t.Fatal(err)
	}
	for _, given := range []bool{false, false, true} {
		if err := st.AddUrge(id, given); err != nil {
			t.Fatal(err)
		}
	}
	st.Close()
	m = press(t, m, "r")
	want := "urges       2 resisted · 1 given in"
	if l := line(t, m, "urges"); !strings.Contains(l, want) {
		t.Fatalf("urges line = %q, want it to contain %q", l, want)
	}
	rows := strings.Split(screen(m), "\n")
	at := func(s string) int {
		for i, l := range rows {
			if strings.Contains(l, s) {
				return i
			}
		}
		t.Fatalf("no line with %q", s)
		return -1
	}
	if !(at("1 interruption") < at("urges") && at("urges") < at("rating")) {
		t.Fatalf("urges must sit between the interruption line and rating:\n%s", screen(m))
	}

	cardAt := func(h int) string {
		var out []string
		for _, l := range m.focus.view(100, h, m.now()) {
			out = append(out, ansi.Strip(l))
		}
		return strings.Join(out, "\n")
	}
	if !strings.Contains(cardAt(33), "urges") {
		t.Fatal("the urges line must stay at body height 33")
	}
	if s := cardAt(32); strings.Contains(s, "urges") || !strings.Contains(s, "apps") {
		t.Fatalf("at 32 the urges line goes while apps stays:\n%s", s)
	}
}

func TestFocusNoUrgeLineWithoutUrges(t *testing.T) {
	m, e := seeded(t)
	e.Heartbeat(t, e.Clock.UnixMilli())
	seedManual(t, e, "09:10", "10:00", 0, counters)
	m = press(t, m, "4")
	for _, l := range strings.Split(screen(m), "\n") {
		if strings.Contains(l, "urges") {
			t.Fatalf("a card without urges must have no urges line: %q", l)
		}
	}
}

func TestFocusSnoozeHelp(t *testing.T) {
	m, e := seeded(t)
	e.Heartbeat(t, e.Clock.UnixMilli())
	m = press(t, m, "4", "s", "enter", "?")
	mustContain(t, m, "z      snooze or resume di")
	// A narrow screen has one column, wide enough for the whole name.
	narrow := step(t, m, windowSize(60, 80))
	mustContain(t, narrow, "snooze or resume distraction notices")
	m = press(t, m, "esc", "1", ":")
	m = typeText(t, m, "distraction notices")
	mustContain(t, m, "snooze distraction notices")
	mustNotContain(t, m, "resume distraction notices")
	m = press(t, m, "enter")
	m = press(t, m, ":")
	m = typeText(t, m, "distraction notices")
	mustContain(t, m, "resume distraction notices")
	mustNotContain(t, m, "snooze distraction notices")
}

// addRec writes a focus recommendation of 25 min sent ago before the test
// clock and returns its id.
func addRec(t *testing.T, e *apptest.Env, ago time.Duration) int64 {
	t.Helper()
	st, err := store.Open(e.DB)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	id, err := st.AddRecommend(store.Recommend{Kind: "focus", AtMS: e.Clock.Add(-ago).UnixMilli(), Weekday: int(e.Clock.Weekday()), Hour: e.Clock.Hour(), LengthMS: 25 * 60_000})
	if err != nil {
		t.Fatal(err)
	}
	return id
}

func TestFocusSuggested(t *testing.T) {
	m, e := seeded(t)
	e.Heartbeat(t, e.Clock.UnixMilli())
	id := addRec(t, e, 5*time.Minute)
	m = press(t, m, "r", "4")
	mustContain(t, m, "Suggested: focus now (25 min)", "enter start", "d not now")

	// enter takes it.
	m = press(t, m, "enter")
	mustContain(t, m, "started 25 min session", "left of 25m")
	mustNotContain(t, m, "Suggested:")
	if r := recRow(t, e, id); r.Reply != "accept" {
		t.Fatalf("reply = %q, want accept", r.Reply)
	}

	// d says not now, on a fresh row.
	m2, e2 := seeded(t)
	e2.Heartbeat(t, e2.Clock.UnixMilli())
	id2 := addRec(t, e2, 5*time.Minute)
	m2 = press(t, m2, "r", "4", "d")
	mustContain(t, m2, "suggestion dismissed", "No session running.")
	mustNotContain(t, m2, "Suggested:")
	if r := recRow(t, e2, id2); r.Reply != "dismiss" {
		t.Fatalf("reply = %q, want dismiss", r.Reply)
	}

	// With nothing pending both keys only say so.
	m3, e3 := seeded(t)
	e3.Heartbeat(t, e3.Clock.UnixMilli())
	m3 = press(t, m3, "r", "4", "enter")
	mustContain(t, m3, "no suggestion pending")
	m3 = press(t, m3, "d")
	mustContain(t, m3, "no suggestion pending")
	mustNotContain(t, m3, "Suggested:", "started")

	// 30 min old: not shown.
	m4, e4 := seeded(t)
	e4.Heartbeat(t, e4.Clock.UnixMilli())
	addRec(t, e4, 30*time.Minute)
	m4 = press(t, m4, "r", "4")
	mustNotContain(t, m4, "Suggested:")
}

// With a session running there is no line, and no key answers the row.
func TestFocusSuggestedSessionRunning(t *testing.T) {
	m, e := seeded(t)
	e.Heartbeat(t, e.Clock.UnixMilli())
	id := addRec(t, e, 5*time.Minute)
	m = press(t, m, "4", "s", "enter")
	mustContain(t, m, "session started")
	mustNotContain(t, m, "Suggested:")
	m = press(t, m, "d")
	mustContain(t, m, "no suggestion pending")
	if r := recRow(t, e, id); r.Reply != "" {
		t.Fatalf("reply = %q, want open", r.Reply)
	}
}

// A screen drawn before the row was answered elsewhere answers nothing.
func TestFocusSuggestedStale(t *testing.T) {
	m, e := seeded(t)
	e.Heartbeat(t, e.Clock.UnixMilli())
	id := addRec(t, e, 5*time.Minute)
	m = press(t, m, "r", "4")
	mustContain(t, m, "Suggested: focus now (25 min)")
	st, err := store.Open(e.DB)
	if err != nil {
		t.Fatal(err)
	}
	if ok, err := st.ReplyRecommend(id, "dismiss", e.Clock.UnixMilli(), 0); err != nil || !ok {
		t.Fatalf("answer elsewhere: %v, %v", ok, err)
	}
	st.Close()
	next, cmd := m.Update(key("enter"))
	m = runCmd(t, next.(Model), cmd)
	mustContain(t, m, "that suggestion is no longer open")
	mustNotContain(t, m, "left of 25m")
}

func recRow(t *testing.T, e *apptest.Env, id int64) store.Recommend {
	t.Helper()
	st, err := store.Open(e.DB)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	r, err := st.Recommend(id)
	if err != nil {
		t.Fatal(err)
	}
	return r
}

// A suggested break is taken with enter: it says when the break ends and starts
// no session.
func TestFocusSuggestedBreak(t *testing.T) {
	m, e := seeded(t)
	e.Heartbeat(t, e.Clock.UnixMilli())
	st, err := store.Open(e.DB)
	if err != nil {
		t.Fatal(err)
	}
	id, err := st.AddRecommend(store.Recommend{Kind: "break", AtMS: e.Clock.Add(-5 * time.Minute).UnixMilli(), Weekday: int(e.Clock.Weekday()), Hour: e.Clock.Hour(), LengthMS: 10 * 60_000})
	st.Close()
	if err != nil {
		t.Fatal(err)
	}
	m = press(t, m, "r", "4")
	mustContain(t, m, "Suggested: break (10 min)", "enter take it")
	m = press(t, m, "enter")
	mustContain(t, m, "break until "+e.Clock.Add(10*time.Minute).Format("15:04"), "No session running.")
	mustNotContain(t, m, "Suggested:", "started")
	if r := recRow(t, e, id); r.Reply != "accept" {
		t.Fatalf("reply = %q, want accept", r.Reply)
	}
}
