package app

import (
	"fmt"
	"hash/fnv"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"tock/internal/apptest"
	"tock/internal/focus"
)

// slotEnv is an env whose plan file holds a Review slot at 14:00 ended by a
// Break at 14:50, plus items that never notify (done, untimed, other day),
// with the clock at 14:00:30 and a span open since 13:50. extra is appended
// to the [plan] table.
func slotEnv(t *testing.T, extra string) (*apptest.Env, string) {
	t.Helper()
	e := apptest.New(t, moduleRules)
	todo := filepath.Join(t.TempDir(), "todo.txt")
	writePlan(t, todo, "Review @14h00 due:2026-10-10\n"+
		"Break @14h50 due:2026-10-10\n"+
		"x 2026-10-10 Done one @14h00 due:2026-10-10\n"+
		"Untimed due:2026-10-10\n"+
		"Later @14h00 due:2026-10-11\n")
	e.WriteConfig(t, apptest.NoRoutines+"[plan]\nfile = '"+todo+"'\n"+extra)
	e.Seed(t, apptest.Span{ID: 1, Start: "13:50", Bundle: "term", App: "WezTerm", Title: "x"})
	slotClock(e, 14, 0, 30)
	return e, todo
}

func writePlan(t *testing.T, path, body string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
}

func slotClock(e *apptest.Env, h, m, s int) {
	e.Clock = time.Date(2026, 10, 10, h, m, s, 0, apptest.Zone)
}

func slotRows(t *testing.T, e *apptest.Env) []string {
	t.Helper()
	db := e.RawDB(t)
	defer db.Close()
	rows, err := db.Query(`SELECT slot_key, kind FROM slot_notice ORDER BY slot_key, kind`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var k, kind string
		if err := rows.Scan(&k, &kind); err != nil {
			t.Fatal(err)
		}
		out = append(out, k+"|"+kind)
	}
	return out
}

func notes(evs []focus.Event) []string {
	var out []string
	for _, ev := range evs {
		out = append(out, fmt.Sprintf("%d %s|%s|%s", ev.Kind, ev.Topic, ev.Title, ev.Body))
	}
	return out
}

func wantNote(topic, title, body string) string {
	return fmt.Sprintf("%d %s|%s|%s", focus.Notify, topic, title, body)
}

func tickAt(t *testing.T, a *App, e *apptest.Env, h, m, s int) []string {
	t.Helper()
	slotClock(e, h, m, s)
	evs, err := a.FocusTick(false)
	if err != nil {
		t.Fatal(err)
	}
	return notes(evs)
}

func wantList(t *testing.T, label string, got []string, want ...string) {
	t.Helper()
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Fatalf("%s: got %q, want %q", label, got, want)
	}
}

func reviewKey() string {
	h := fnv.New64a()
	h.Write([]byte("Review"))
	return fmt.Sprintf("plan:2026-10-10T14:00:%016x", h.Sum64())
}

func TestSlotStartingNotice(t *testing.T) {
	e, _ := slotEnv(t, "")
	a := open(t, e)
	start := wantNote(focus.TopicStarting, "Starting now", "14:00–14:50 Review")
	wantList(t, "14:00:30", tickAt(t, a, e, 14, 0, 30), start)
	wantList(t, "second tick", tickAt(t, a, e, 14, 0, 40))
	wantList(t, "after a restart", tickAt(t, open(t, e), e, 14, 1, 0))
	wantList(t, "rows", slotRows(t, e), reviewKey()+"|start")
}

func TestSlotStartStale(t *testing.T) {
	e, _ := slotEnv(t, "")
	a := open(t, e)
	wantList(t, "14:02", tickAt(t, a, e, 14, 2, 0))
	wantList(t, "rows", slotRows(t, e))
}

func TestSlotsCached(t *testing.T) {
	e, todo := slotEnv(t, "")
	a := open(t, e)
	now := e.Clock
	got, err := a.Slots(now)
	if err != nil {
		t.Fatal(err)
	}
	var desc []string
	for _, s := range got {
		desc = append(desc, fmt.Sprintf("%s %s-%s %s", s.Kind, time.UnixMilli(s.StartMS).In(apptest.Zone).Format("15:04"),
			time.UnixMilli(s.EndMS).In(apptest.Zone).Format("15:04"), s.Title))
		if strings.Contains(s.Key, "line") {
			t.Errorf("key %q mentions a line", s.Key)
		}
	}
	// Done one is done, Untimed has no clock, Later is another day.
	wantList(t, "slots", desc, "plan 14:00-14:50 Review", "plan 14:50-17:50 Break")
	if got[0].Key != reviewKey() {
		t.Errorf("key = %q, want %q", got[0].Key, reviewKey())
	}
	writePlan(t, todo, "Retro @14h00 due:2026-10-10\n")
	got, err = a.Slots(now)
	if err != nil || len(got) != 1 || got[0].Title != "Retro" {
		t.Fatalf("after rewrite: %+v, %v, want Retro", got, err)
	}
}

func TestSlotModule(t *testing.T) {
	e, todo := slotEnv(t, "")
	writePlan(t, todo, "Cours +unknown +ana1 @14h00 due:2026-10-10\n")
	got, err := open(t, e).Slots(e.Clock)
	if err != nil || len(got) != 1 || got[0].Project != "Analyse 1" {
		t.Fatalf("slots = %+v, %v, want project Analyse 1", got, err)
	}
}

func TestSlotUpcomingNotice(t *testing.T) {
	e, _ := slotEnv(t, "")
	a := open(t, e)
	wantList(t, "13:54", tickAt(t, a, e, 13, 54, 0))
	wantList(t, "13:55", tickAt(t, a, e, 13, 55, 0), wantNote(focus.TopicUpcoming, "Up next", "14:00 Review · in 5 min"))
	wantList(t, "13:56", tickAt(t, a, e, 13, 56, 0))
	wantList(t, "restart at 13:57", tickAt(t, open(t, e), e, 13, 57, 0))
	wantList(t, "14:00:10", tickAt(t, a, e, 14, 0, 10), wantNote(focus.TopicStarting, "Starting now", "14:00–14:50 Review"))
	wantList(t, "14:00:20", tickAt(t, a, e, 14, 0, 20))
}

func TestSlotFirstSeenLate(t *testing.T) {
	e, _ := slotEnv(t, "")
	a := open(t, e)
	wantList(t, "13:59:30", tickAt(t, a, e, 13, 59, 30))
	wantList(t, "14:00:00", tickAt(t, a, e, 14, 0, 0), wantNote(focus.TopicStarting, "Starting now", "14:00–14:50 Review"))
}

func TestSlotNotifyOff(t *testing.T) {
	e, _ := slotEnv(t, "notify = false\n")
	a := open(t, e)
	wantList(t, "13:55", tickAt(t, a, e, 13, 55, 0))
	wantList(t, "14:00", tickAt(t, a, e, 14, 0, 0))
	wantList(t, "rows", slotRows(t, e))

	e2 := apptest.New(t, moduleRules)
	todo := filepath.Join(t.TempDir(), "todo.txt")
	writePlan(t, todo, "Review @14h00 due:2026-10-10\n")
	e2.WriteConfig(t, apptest.NoRoutines+apptest.NoPlanNotices)
	e2.Seed(t, apptest.Span{ID: 1, Start: "13:50", Bundle: "term", App: "WezTerm", Title: "x"})
	a2 := open(t, e2)
	if a2.ConfigErr() != nil || a2.cfg.Plan.Notify {
		t.Fatalf("NoPlanNotices: err %v, notify %v, want a loaded config with notices off", a2.ConfigErr(), a2.cfg.Plan.Notify)
	}
}
