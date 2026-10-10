package tui

import (
	"strings"
	"testing"
	"time"

	"tock/internal/app"
	"tock/internal/apptest"
)

func TestTodayShowsTheDay(t *testing.T) {
	m, _ := seeded(t)
	mustContain(t, m,
		"tock   today   week   inbox ³", "Sat 10 Oct  15:00  ● trackd down",
		"trackd is not running",
		"74  score  good day", "2h14 focus of 4h00",
		"1 block · longest 2h14 · 2 switches · tracked 2h54 · idle 41m",
		"● Code           2h14  ━━━━━━━━  tock 1h40",
		"○ unlabelled      28m  ━━──────  3 keys",
		"09:00–10:40   1h40  ● Code / tock         WezTerm     tock │ ✳ x",
		"11:41–12:22    41m    idle",
	)
	if l := line(t, m, "14:50–15:00"); !strings.HasPrefix(l, "▸") || !strings.Contains(l, "○ unlabelled") {
		t.Fatalf("cursor must start on the open last row: %q", l)
	}
	if l := line(t, m, "│ deep       "); !strings.HasPrefix(l, " │ deep       ████████░░░░ ") {
		// Code: 09h 60m, 10h 40m+20m, 11h 14m; four cells per hour from 09
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
	mustContain(t, m, "● trackd\n", "15:00–15:02     2m  ● Code / tock")
	if l := line(t, m, "15:00–15:02"); !strings.HasPrefix(l, "▸") {
		t.Fatalf("cursor on the last row must follow new rows: %q", l)
	}
}

func TestCursorStaysOnItsRowAcrossRefresh(t *testing.T) {
	m, e := seeded(t)
	m = press(t, m, "k", "k")
	if l := line(t, m, "11:41–12:22"); !strings.HasPrefix(l, "▸") {
		t.Fatalf("k k: %q", l)
	}
	e.Seed(t, apptest.Span{ID: 9, Start: "15:00", End: "", Bundle: "term", App: "WezTerm", Title: "x"})
	e.Clock = e.Clock.Add(time.Minute)
	m = step(t, m, tickMsg{})
	if l := line(t, m, "11:41–12:22"); !strings.HasPrefix(l, "▸") {
		t.Fatalf("cursor moved on refresh: %q", l)
	}
}

func TestTagRow(t *testing.T) {
	m, _ := seeded(t)
	m = press(t, m, "t")
	mustContain(t, m, "Category", "Communication")
	m = typeText(t, m, "comm")
	if opts := optNames(m.picker); strings.Contains(opts, "Entertainment") {
		t.Fatalf("filter kept Entertainment: %s", opts)
	}
	m = press(t, m, "enter")
	mustContain(t, m, "Project for Communication", "(no project)", "ATLAS")
	if opts := optNames(m.picker); strings.Contains(opts, "Old") { // archived
		t.Fatalf("archived project offered: %s", opts)
	}
	m = press(t, m, "enter")
	mustContain(t, m, "tagged 1 span (10m) → Communication", "inbox ²")
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
	m = press(t, m, "k", "k")
	for _, k := range []string{"p", "T"} {
		m = press(t, m, k)
		mustContain(t, m, "idle time has no label")
		if m.picker != nil {
			t.Fatalf("%s opened a picker on an idle row", k)
		}
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
	mustContain(t, m, "Fri 9 Oct  ‹", "tracked 1h00", "09:00–10:00")
	m = press(t, m, "l")
	mustContain(t, m, "Today  ‹ h  l ›")
	m = press(t, m, "l")
	mustContain(t, m, "Today  ‹ h  l ›")
	m = press(t, m, "h", "h", ".")
	mustContain(t, m, "Today  ‹ h  l ›")
}

func TestMidnightMovesToTheNewDay(t *testing.T) {
	m, e := seeded(t)
	e.Clock = time.Date(2026, 10, 11, 0, 5, 0, 0, apptest.Zone)
	m = step(t, m, tickMsg{})
	// The open Mail span (since 14:50) now counts from midnight.
	mustContain(t, m, "Sun 11 Oct  00:05", "Today  ‹", "tracked 5m", "00:00–00:05")
}

func TestTooSmall(t *testing.T) {
	m, _ := seeded(t)
	m = step(t, m, struct{}{}) // no-op
	next, _ := m.Update(windowSize(59, 30))
	if got := screen(next.(Model)); got != "terminal too small (60×16 needed)" {
		t.Fatalf("got %q", got)
	}
}

func TestScorePanel(t *testing.T) {
	m, _ := seeded(t)
	m = press(t, m, "i")
	mustContain(t, m,
		"Score 74 · Sat 10 Oct",
		"  depth    0.56 × 0.5 = 27.9   focus 2h14 of target 4h00",
		"  blocks   1.00 × 0.3 = 30.0   2h14 in blocks of 25m or more",
		"  calm     0.97 × 0.2 = 19.3   2 switches in 2h54 active = 0.7 an hour (20 = 0)",
		"  penalty             −    3   12m distraction, 1 point per 4m, at most 15",
		"                 77 − 3 = 74   (sum rounded, then penalty, 0–100)",
		"  target   4h00 default: 0 of 7 workdays so far",
		"  blocks   09:00–11:14  2h14",
		"esc close · ? keys",
	)
	m = press(t, m, "?")
	mustContain(t, m, "─ Keys ─", "why this score")
	m = press(t, m, "?")
	mustContain(t, m, "Today  ‹ h  l ›")
	m = press(t, m, "?", "esc")
	mustContain(t, m, "Today  ‹ h  l ›")
	mustNotContain(t, m, "Score 74 ·")
}

func TestScorePanelEmptyDayAndManyBlocks(t *testing.T) {
	e := apptest.New(t, apptest.Rules)
	var spans []apptest.Span
	// 20 blocks of 6m deep separated by 2m of distraction.
	for i := range 20 {
		start := time.Date(2026, 10, 10, 9, 0, 0, 0, apptest.Zone).Add(time.Duration(i) * 8 * time.Minute)
		spans = append(spans,
			apptest.Span{ID: int64(2*i + 1), Start: start.Format("15:04"), End: start.Add(6 * time.Minute).Format("15:04"),
				Bundle: "term", App: "WezTerm", Title: "tock │ x"},
			apptest.Span{ID: int64(2*i + 2), Start: start.Add(6 * time.Minute).Format("15:04"), End: start.Add(8 * time.Minute).Format("15:04"),
				Bundle: "arc", App: "Arc", URL: "youtube.com/x"})
	}
	e.Seed(t, spans...)
	m := newModel(t, e)
	m = press(t, m, "i")
	// 30 rows: header, banner and footer leave 27; the panel uses 12 before
	// the blocks, so 14 blocks show and one line counts the other 6.
	mustContain(t, m, "  blocks   09:00–09:06  6m", "           10:44–10:50  6m", "           +6 more")
	mustNotContain(t, m, "10:52–10:58")
	m = press(t, m, "esc", "h", "i")
	mustContain(t, m, "Score · Fri 9 Oct", "no activity on this day")
}

func TestConfigBanner(t *testing.T) {
	m, e := seeded(t)
	e.Heartbeat(t, e.Clock.UnixMilli())
	e.WriteConfig(t, "[score]\ndepth = 0.9\n")
	m = step(t, m, tickMsg{})
	mustContain(t, m, "config error, using the last good settings: config.toml: [score] weights sum to 1.40, want 1")
}

func optNames(p *picker) string {
	var b []string
	for _, o := range p.options() {
		b = append(b, o.name)
	}
	return strings.Join(b, "|")
}

func TestRemoteRowsRefuseLabels(t *testing.T) {
	dir := t.TempDir()
	er := apptest.New(t, apptest.Rules)
	er.Seed(t, apptest.Span{ID: 1, Start: "11:45", End: "12:15", Bundle: "term", App: "WezTerm", Title: "tock │ ryuk"})
	er.WriteConfig(t, "[sync]\ndir = \""+dir+"\"\nmachine = \"ryuk\"\n")
	ra, err := app.Open(app.Paths{DB: er.DB, Rules: er.Rules, Learned: er.Learned, Config: er.Config}, er.NowFunc())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ra.Sync(app.SyncDays); err != nil {
		t.Fatal(err)
	}
	ra.Close()

	m, e := seeded(t)
	e.WriteConfig(t, "[sync]\ndir = \""+dir+"\"\nmachine = \"mac\"\n")
	ma, err := app.Open(app.Paths{DB: e.DB, Rules: e.Rules, Learned: e.Learned, Config: e.Config}, e.NowFunc())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ma.Sync(app.SyncDays); err != nil {
		t.Fatal(err)
	}
	ma.Close()
	m = press(t, m, "r")
	mustContain(t, m, "ryuk · We")
	m = clickOn(t, m, "tock │ ryuk")
	for _, k := range []string{"t", "p", "T", "enter"} {
		m = press(t, m, k)
		if m.picker != nil {
			t.Fatalf("%s opened a picker on a remote row", k)
		}
	}
	mustContain(t, m, "recorded on another machine")
}
