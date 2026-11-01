package main

import (
	"database/sql"
	"strings"
	"testing"
	"time"
)

func TestTrends(t *testing.T) {
	te := newTestEnv(t, testRules)
	te.seed(t, daySpans()[:7]...) // no open span
	te.now = testNow.AddDate(0, 1, 0)
	code, out := te.run(t, "", "trends", "--month", "2026-10")
	want := "October 2026 vs September\n" +
		"tracked     2h44  new\n" +
		"focus       2h14  new\n" +
		"score     74 avg · 1 workday   vs —   ·········▆·····················\n" +
		"switches  0.7/h   vs —\n" +
		"\n" +
		"Code               2h14  new\n" +
		"Entertainment       12m  new\n" +
		"Unclassified        18m  new\n"
	if code != 0 || out != want {
		t.Fatalf("code %d\n%s\nwant\n%s", code, out, want)
	}
	code, out = te.run(t, "", "trends")
	if code != 0 || !strings.HasPrefix(out, "November 2026 · 1–10 vs October 1–10\ntracked       0s   −2h44  gone\n") ||
		!strings.Contains(out, "score     —   vs 74   ··········\n") || !strings.Contains(out, "Code                 0s   −2h14  gone\n") {
		t.Fatalf("code %d\n%s", code, out)
	}
	te.now = time.Date(2026, 10, 31, 12, 0, 0, 0, testZone)
	if code, out := te.run(t, "", "trends"); code != 0 || !strings.HasPrefix(out, "October 2026 · 1–31 vs September 1–30\n") {
		t.Fatalf("31st: code %d\n%s", code, out)
	}
}

func TestTrendsErrors(t *testing.T) {
	te := newTestEnv(t, testRules)
	te.seed(t, daySpans()[:7]...)
	if code, _ := te.run(t, "", "trends", "--month", "2026-13"); code != 2 {
		t.Fatalf("bad month: code %d", code)
	}
	if code, _ := te.run(t, "", "trends", "--month", "2026-11"); code != 1 ||
		te.errb.String() != "tock: that month has not started\n" {
		t.Fatalf("future: code %d %q", code, te.errb.String())
	}
}

// seedFocusHistory inserts, for each of the days days before testNow's day,
// a 09:00-10:00 and a 14:00-14:20 span in the terminal on a "tock" title.
// te.seed runs first so the database exists.
func seedFocusHistory(t *testing.T, te *tenv, days int) {
	t.Helper()
	te.seed(t)
	db, err := sql.Open("sqlite", "file:"+te.dbPath())
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	id := int64(1000)
	for i := 1; i <= days; i++ {
		for _, h := range [][4]int{{9, 0, 10, 0}, {14, 0, 14, 20}} {
			d := testNow.AddDate(0, 0, -i)
			day := time.Date(d.Year(), d.Month(), d.Day(), 0, 0, 0, 0, testZone)
			from := day.Add(time.Duration(h[0])*time.Hour + time.Duration(h[1])*time.Minute)
			to := day.Add(time.Duration(h[2])*time.Hour + time.Duration(h[3])*time.Minute)
			id++
			if _, err := db.Exec(`INSERT INTO span(id, start, "end", bundle_id, app, title, url, private) VALUES(?,?,?,?,?,?,?,?)`,
				id, from.UnixMilli(), to.UnixMilli(), "term", "WezTerm", "tock │ h", nil, 0); err != nil {
				t.Fatal(err)
			}
		}
	}
}

func TestTrendsProfile(t *testing.T) {
	te := newTestEnv(t, testRules)
	seedFocusHistory(t, te, 21)
	code, out := te.run(t, "", "trends")
	want := "\nBest focus hours\n"
	for _, wd := range []string{"Mon", "Tue", "Wed", "Thu", "Fri", "Sat", "Sun"} {
		want += wd + "  09–10 · 14–15\n"
	}
	want += "Typical session: 25 min (default, not enough sessions yet)\n" +
		"Sustained focus: 60 min before a break\n" +
		"Suggestions: none answered yet\n" +
		"Breaks: none answered yet\n"
	if code != 0 || !strings.HasSuffix(out, want) {
		t.Fatalf("code %d\n%s\nwant a tail of\n%s", code, out, want)
	}
	if !strings.HasPrefix(out, "October 2026 · 1–10 vs September 1–10\n") {
		t.Errorf("month lines changed:\n%s", out)
	}
	code, out = te.run(t, "", "trends", "--month", "2026-10")
	if code != 0 || !strings.Contains(out, "\nBest focus hours\n") {
		t.Fatalf("current month by name: code %d\n%s", code, out)
	}
	code, out = te.run(t, "", "trends", "--month", "2026-09")
	if code != 0 || strings.Contains(out, "Best focus hours") || !strings.HasPrefix(out, "September 2026 vs August\n") {
		t.Fatalf("past month: code %d\n%s", code, out)
	}
}

func TestTrendsProfileThin(t *testing.T) {
	te := newTestEnv(t, testRules)
	seedFocusHistory(t, te, 4)
	code, out := te.run(t, "", "trends")
	if code != 0 || !strings.Contains(out, "\nBest focus hours (not enough history: 4 of 7 days)\nMon  –\n") ||
		!strings.HasSuffix(out, "Sustained focus: (not enough history)\nSuggestions: none answered yet\nBreaks: none answered yet\n") {
		t.Fatalf("code %d\n%s", code, out)
	}
}

// TestTrendsReplies: the accept rates and the muted hours come from the
// answered recommendations of the last 30 days.
func TestTrendsReplies(t *testing.T) {
	te := newTestEnv(t, testRules)
	seedFocusHistory(t, te, 21)
	db, err := sql.Open("sqlite", "file:"+te.dbPath())
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	add := func(kind string, hour, daysAgo int, reply string) {
		t.Helper()
		d := testNow.AddDate(0, 0, -daysAgo)
		at := time.Date(d.Year(), d.Month(), d.Day(), hour, 10, 0, 0, testZone).UnixMilli()
		if _, err := db.Exec(`INSERT INTO recommend(kind, at_ms, weekday, hour, length_ms, reply, reply_ms) VALUES(?,?,?,?,?,?,?)`,
			kind, at, int(d.Weekday()), hour, 25*60_000, reply, at+1000); err != nil {
			t.Fatal(err)
		}
	}
	for i := 1; i <= 3; i++ {
		add("focus", 16, i, "dismiss")
	}
	add("focus", 9, 4, "accept")
	add("focus", 9, 5, "")
	for i := 1; i <= 3; i++ {
		add("break", 15, i, "dismiss")
	}
	add("break", 11, 4, "accept")
	add("focus", 9, 40, "dismiss") // aged out
	code, out := te.run(t, "", "trends")
	want := "Suggestions: 1 of 4 accepted · muted at 16–17\nBreaks: 1 of 4 taken · muted at 15–16\n"
	if code != 0 || !strings.HasSuffix(out, want) {
		t.Fatalf("code %d\n%s\nwant a tail of\n%s", code, out, want)
	}
}

func TestTrendsHours(t *testing.T) {
	te := newTestEnv(t, testRules)
	seedFocusHistory(t, te, 21)
	code, out := te.run(t, "", "trends", "--hours")
	if code != 0 {
		t.Fatalf("code %d %q", code, te.errb.String())
	}
	lines := strings.Split(strings.TrimRight(out, "\n"), "\n")
	if len(lines) != 9 || lines[0] != "Mean focus minutes per hour, last 28 days" ||
		lines[1] != "     07  08  09  10  11  12  13  14  15  16  17  18  19  20  21  22" {
		t.Fatalf("grid:\n%s", out)
	}
	for i, wd := range []string{"Mon", "Tue", "Wed", "Thu", "Fri", "Sat", "Sun"} {
		if want := wd + "   ·   ·  60   ·   ·   ·   ·  20   ·   ·   ·   ·   ·   ·   ·   ·"; lines[2+i] != want {
			t.Errorf("row %d = %q, want %q", i, lines[2+i], want)
		}
	}
	if strings.Contains(out, "October 2026") {
		t.Errorf("--hours printed the month:\n%s", out)
	}
	if code, _ := te.run(t, "", "trends", "--hours", "extra"); code != 2 {
		t.Errorf("--hours extra: code %d, want 2", code)
	}
}

// TestHelpTrendsAndRecommend: the usage text names --hours and tock recommend.
func TestHelpTrendsAndRecommend(t *testing.T) {
	te := newTestEnv(t, testRules)
	_, out := te.run(t, "", "help")
	all := out + te.errb.String()
	for _, want := range []string{"tock trends [--month YYYY-MM] [--hours]", "tock recommend [accept | dismiss]"} {
		if !strings.Contains(all, want) {
			t.Errorf("help lacks %q:\n%s", want, all)
		}
	}
}
