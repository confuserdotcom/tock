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
	"tock/internal/store"
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
	got, err = a.Slots(now.Add(slotRefresh))
	if err != nil || len(got) != 1 || got[0].Title != "Retro" {
		t.Fatalf("after rewrite: %+v, %v, want Retro", got, err)
	}
}

// The plan file and the event table are looked at once per slotRefresh, not
// on every 5 s tick (trackd's idle budget); the cached list serves between.
func TestSlotsRefreshThrottled(t *testing.T) {
	e, todo := slotEnv(t, "")
	a := open(t, e)
	now := e.Clock
	first, err := a.Slots(now)
	if err != nil || len(first) != 2 {
		t.Fatalf("first: %+v, %v", first, err)
	}
	writePlan(t, todo, "Retro @14h00 due:2026-10-10\n")
	e.Event(t, "16:00", "16:30", "New")
	got, err := a.Slots(now.Add(slotRefresh - time.Second))
	if err != nil || len(got) != 2 || got[0].Title != "Review" {
		t.Fatalf("inside the period: %+v, %v, want the cached Review and Break", got, err)
	}
	got, err = a.Slots(now.Add(slotRefresh))
	if err != nil || len(got) != 2 || got[0].Title != "Retro" || got[1].Title != "New" {
		t.Fatalf("after the period: %+v, %v, want Retro and New", got, err)
	}
	// A new day is never served from the cache.
	got, err = a.Slots(now.AddDate(0, 0, 1).Add(slotRefresh - time.Second))
	if err != nil || len(got) != 0 {
		t.Fatalf("next day: %+v, %v, want no slot", got, err)
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

func standupKey(t testing.TB) string {
	h := fnv.New64a()
	h.Write([]byte("Standup"))
	return fmt.Sprintf("event:%d:%016x", apptest.At(t, "14:30", 0), h.Sum64())
}

// eventEnv is an env with NoRoutines, a span open since 14:20 and the clock at 14:24.
func eventEnv(t *testing.T) (*apptest.Env, *App) {
	t.Helper()
	e := apptest.New(t, moduleRules)
	e.WriteConfig(t, apptest.NoRoutines)
	e.Seed(t, apptest.Span{ID: 1, Start: "14:20", Bundle: "term", App: "WezTerm", Title: "x"})
	e.Event(t, "14:30", "15:00", "Standup")
	slotClock(e, 14, 24, 0)
	return e, open(t, e)
}

func TestEventSlotNotices(t *testing.T) {
	e, a := eventEnv(t)
	wantList(t, "14:24", tickAt(t, a, e, 14, 24, 0))
	wantList(t, "14:25", tickAt(t, a, e, 14, 25, 0), wantNote(focus.TopicUpcoming, "Up next", "14:30 Standup (Work) · in 5 min"))
	wantList(t, "14:30:10", tickAt(t, a, e, 14, 30, 10), wantNote(focus.TopicStarting, "Starting now", "14:30–15:00 Standup (Work)"))
	wantList(t, "14:30:20", tickAt(t, a, e, 14, 30, 20))
}

func TestEventSlotResync(t *testing.T) {
	e, a := eventEnv(t)
	wantList(t, "14:25", tickAt(t, a, e, 14, 25, 0), wantNote(focus.TopicUpcoming, "Up next", "14:30 Standup (Work) · in 5 min"))
	st := a.st
	if _, err := st.SyncEvents(apptest.At(t, "00:00", 0), apptest.At(t, "00:00", 1), []store.Event{
		{StartMS: apptest.At(t, "14:30", 0), EndMS: apptest.At(t, "15:00", 0), Title: "Standup", Calendar: "Work"},
		{StartMS: apptest.At(t, "16:00", 0), EndMS: apptest.At(t, "16:30", 0), Title: "Retro", Calendar: "Work"},
	}); err != nil {
		t.Fatal(err)
	}
	wantList(t, "14:26", tickAt(t, a, e, 14, 26, 0))
	slotClock(e, 14, 26, 0)
	got, err := a.Slots(e.Clock)
	if err != nil {
		t.Fatal(err)
	}
	var titles []string
	for _, s := range got {
		titles = append(titles, s.Title)
	}
	wantList(t, "slots", titles, "Standup", "Retro")
}

func TestSlotsMerged(t *testing.T) {
	e, todo := slotEnv(t, "")
	writePlan(t, todo, "Review @14h30 due:2026-10-10\n")
	e.Event(t, "14:30", "15:00", "Standup")
	a := open(t, e)
	got, err := a.Slots(e.Clock)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0].Kind != focus.SlotEvent || got[1].Kind != focus.SlotPlan {
		t.Fatalf("slots = %+v, want the event first, then the plan item", got)
	}
	if got[0].Key != standupKey(t) {
		t.Errorf("event key = %q, want %q", got[0].Key, standupKey(t))
	}
	wantList(t, "14:30:10", tickAt(t, a, e, 14, 30, 10),
		wantNote(focus.TopicStarting, "Starting now", "14:30–15:00 Standup (Work)"),
		wantNote(focus.TopicStarting, "Starting now", "14:30–17:30 Review"))
}

func TestEventSlotJoinURL(t *testing.T) {
	e := apptest.New(t, moduleRules)
	e.WriteConfig(t, apptest.NoRoutines)
	e.EventLink(t, "14:30", "15:00", "Standup", "https://meet.google.com/abc-defg-hij")
	e.EventLinkUnanswered(t, "15:30", "16:00", "Invite", "https://zoom.us/j/1")
	slotClock(e, 14, 24, 0)
	got, err := open(t, e).Slots(e.Clock)
	if err != nil || len(got) != 2 || got[0].JoinURL != "https://meet.google.com/abc-defg-hij" || !got[0].Mine ||
		got[1].JoinURL != "https://zoom.us/j/1" || got[1].Mine {
		t.Fatalf("slots = %+v, %v, want the Standup slot with its link, Mine, and the Invite one not", got, err)
	}
}

// remoteView seeds one view of machine ryuk on day with the given end.
func remoteView(t *testing.T, a *App, from, to int64) {
	t.Helper()
	if err := a.st.ReplaceRemoteDay("ryuk", "2026-10-10", store.RemoteFile{MtimeMS: 1, Size: 1},
		[]store.RemoteView{{StartMS: from, EndMS: to, Bundle: "term", App: "WezTerm"}}); err != nil {
		t.Fatal(err)
	}
}

func TestSlotOtherMachineNewer(t *testing.T) {
	e := apptest.New(t, moduleRules)
	todo := filepath.Join(t.TempDir(), "todo.txt")
	writePlan(t, todo, "Review @14h00 due:2026-10-10\n")
	e.WriteConfig(t, apptest.NoRoutines+"[plan]\nfile = '"+todo+"'\n")
	e.Seed(t, apptest.Span{ID: 1, Start: "13:00", End: "13:45", Bundle: "term", App: "WezTerm", Title: "x"})
	a := open(t, e)
	remoteView(t, a, apptest.At(t, "13:50", 0), apptest.At(t, "14:00", 0)+20_000)
	wantList(t, "14:00:30", tickAt(t, a, e, 14, 0, 30))
	wantList(t, "rows", slotRows(t, e))

	e.Seed(t, apptest.Span{ID: 2, Start: "14:00", Bundle: "term", App: "WezTerm", Title: "y"})
	a = open(t, e)
	wantList(t, "14:01:00", tickAt(t, a, e, 14, 1, 0), wantNote(focus.TopicStarting, "Starting now", "14:00–17:00 Review"))
	wantList(t, "14:01:10", tickAt(t, a, e, 14, 1, 10))
}

func TestSlotOtherMachineStale(t *testing.T) {
	e := apptest.New(t, moduleRules)
	todo := filepath.Join(t.TempDir(), "todo.txt")
	writePlan(t, todo, "Review @14h00 due:2026-10-10\n")
	e.WriteConfig(t, apptest.NoRoutines+"[plan]\nfile = '"+todo+"'\n")
	e.Seed(t, apptest.Span{ID: 1, Start: "13:30", End: "13:35", Bundle: "term", App: "WezTerm", Title: "x"})
	a := open(t, e)
	remoteView(t, a, apptest.At(t, "13:30", 0), apptest.At(t, "13:40", 0))
	wantList(t, "14:00:30", tickAt(t, a, e, 14, 0, 30), wantNote(focus.TopicStarting, "Starting now", "14:00–17:00 Review"))
}

// autoEnv is slotEnv with auto_start on and a module-tagged TD 1 at 14:00
// ended by a Lunch at 14:50.
func autoEnv(t *testing.T, extra string) *apptest.Env {
	t.Helper()
	e, todo := slotEnv(t, "auto_start = true\n"+extra)
	writePlan(t, todo, "TD 1 +ana1 @14h00 due:2026-10-10\nLunch @14h50 due:2026-10-10\n")
	return e
}

func sessionRows(t *testing.T, a *App) []string {
	t.Helper()
	ss, err := a.st.Sessions(0, 1<<60)
	if err != nil {
		t.Fatal(err)
	}
	var out []string
	for _, s := range ss {
		out = append(out, fmt.Sprintf("%s %s-%s end %d", s.Kind, time.UnixMilli(s.StartMS).In(apptest.Zone).Format("15:04:05"),
			time.UnixMilli(s.PlannedMS).In(apptest.Zone).Format("15:04:05"), s.EndMS))
	}
	return out
}

func tickEvents(t *testing.T, a *App, e *apptest.Env, call bool, h, m, s int) []focus.Event {
	t.Helper()
	slotClock(e, h, m, s)
	evs, err := a.FocusTick(call)
	if err != nil {
		t.Fatal(err)
	}
	return evs
}

func TestSlotAutoStart(t *testing.T) {
	e := autoEnv(t, "")
	a := open(t, e)
	evs := tickEvents(t, a, e, false, 14, 0, 30)
	wantList(t, "14:00:30", notes(evs), wantNote(focus.TopicStarting, "Starting now",
		"14:00–14:50 TD 1 · focus session started · 50 min · Analyse 1"))
	wantList(t, "sessions", sessionRows(t, a), "manual 14:00:30-14:50:00 end 0")
	id, err := a.RunningSessionID()
	if err != nil || id == 0 || evs[0].Sess.ID != id {
		t.Fatalf("RunningSessionID = %d, %v; event session %d", id, err, evs[0].Sess.ID)
	}
	if err := a.Enqueue(evs[0]); err != nil {
		t.Fatal(err)
	}
	ns, err := a.TakeNotices()
	if err != nil || len(ns) != 1 {
		t.Fatalf("notices = %+v, %v", ns, err)
	}
	if _, d, err := a.Notice(ns[0].ID); err != nil || d.Session != id {
		t.Fatalf("notice data = %+v, %v, want session %d", d, err, id)
	}
	wantList(t, "second tick", tickAt(t, a, e, 14, 0, 40))
	b := open(t, e)
	wantList(t, "after a restart", tickAt(t, b, e, 14, 1, 0))
	wantList(t, "sessions after the restart", sessionRows(t, b), "manual 14:00:30-14:50:00 end 0")
}

func TestSlotAutoStartBusy(t *testing.T) {
	e := autoEnv(t, "")
	a := open(t, e)
	slotClock(e, 13, 58, 0)
	if _, err := a.StartSession(25); err != nil {
		t.Fatal(err)
	}
	wantList(t, "14:00:30", tickAt(t, a, e, 14, 0, 30), wantNote(focus.TopicStarting, "Starting now", "14:00–14:50 TD 1"))
	wantList(t, "sessions", sessionRows(t, a), "manual 13:58:00-14:23:00 end 0")

	e2 := autoEnv(t, "")
	a2 := open(t, e2)
	evs := tickEvents(t, a2, e2, true, 14, 0, 30)
	wantList(t, "call", notes(evs), wantNote(focus.TopicStarting, "Starting now", "14:00–14:50 TD 1"))
	wantList(t, "sessions during a call", sessionRows(t, a2))
}

func TestSlotAutoStartRace(t *testing.T) {
	e := autoEnv(t, "")
	a := open(t, e)
	slotClock(e, 14, 0, 30)
	now := e.Clock.UnixMilli()
	// tock started a session after trackd read the open ones: the engine saw
	// none, the store refuses a second one and the notice stays plain.
	if _, err := a.st.StartSession(now-1000, now+600_000); err != nil {
		t.Fatal(err)
	}
	ev := focus.Event{Kind: focus.Notify, Topic: focus.TopicStarting, AutoStart: true,
		Sess: focus.Session{Kind: focus.KindManual, StartMS: now, PlannedMS: now + 3_000_000},
		Body: "14:00–14:50 TD 1 · focus session started · 50 min · Analyse 1", AltBody: "14:00–14:50 TD 1"}
	if err := a.startSlotSession(&ev); err != nil {
		t.Fatalf("startSlotSession: %v, want the race to leave a plain notice", err)
	}
	if ev.AutoStart || ev.Sess.ID != 0 || ev.Body != "14:00–14:50 TD 1" {
		t.Errorf("event = %+v, want the plain body and no start", ev)
	}
	if rows := sessionRows(t, a); len(rows) != 1 {
		t.Errorf("sessions = %v, want only tock's", rows)
	}
}

func TestSlotAutoStartSilent(t *testing.T) {
	e := autoEnv(t, "notify = false\n")
	a := open(t, e)
	wantList(t, "13:55", tickAt(t, a, e, 13, 55, 0))
	wantList(t, "14:00:30", tickAt(t, a, e, 14, 0, 30))
	wantList(t, "sessions", sessionRows(t, a), "manual 14:00:30-14:50:00 end 0")
	// The session ends; a later tick in the window starts nothing.
	if _, err := a.StopSession(); err != nil {
		t.Fatal(err)
	}
	wantList(t, "14:01:00", tickAt(t, a, e, 14, 1, 0))
	if rows := sessionRows(t, a); len(rows) != 1 || !strings.HasPrefix(rows[0], "manual 14:00:30-14:50:00 end ") {
		t.Errorf("sessions after = %v, want the one ended session", rows)
	}
}

// On the day the clocks go forward (2026-03-29 02:00 in Paris) a plan item at
// 14h00 still starts at 14:00 on the wall, and so does the classify slot.
func TestSlotsOnDSTDay(t *testing.T) {
	paris, err := time.LoadLocation("Europe/Paris")
	if err != nil {
		t.Skip("no tzdata:", err)
	}
	e := apptest.New(t, moduleRules)
	todo := filepath.Join(t.TempDir(), "todo.txt")
	writePlan(t, todo, "Review +ana1 @14h00 due:2026-03-29\n")
	e.WriteConfig(t, apptest.NoRoutines+"[plan]\nfile = '"+todo+"'\n")
	e.Seed(t, apptest.Span{ID: 1, Start: "13:50", Bundle: "term", App: "WezTerm", Title: "x"})
	e.Clock = time.Date(2026, 3, 29, 13, 0, 0, 0, paris)
	a := open(t, e)
	got, err := a.Slots(e.Clock)
	if err != nil || len(got) != 1 {
		t.Fatalf("slots = %+v, %v, want one", got, err)
	}
	if h := time.UnixMilli(got[0].StartMS).In(paris).Format("15:04"); h != "14:00" {
		t.Errorf("slot starts at %s, want 14:00", h)
	}
	ps := a.planSlots(time.Date(2026, 3, 29, 0, 0, 0, 0, paris).UnixMilli(), time.Date(2026, 3, 30, 0, 0, 0, 0, paris).UnixMilli())
	if len(ps) != 1 {
		t.Fatalf("planSlots = %+v, want one", ps)
	}
	if h := time.UnixMilli(ps[0].From).In(paris).Format("15:04"); h != "14:00" {
		t.Errorf("planSlots starts at %s, want 14:00", h)
	}
}

func planKey(text string, hhmm string) string {
	h := fnv.New64a()
	h.Write([]byte(text))
	return fmt.Sprintf("plan:2026-10-10T%s:%016x", hhmm, h.Sum64())
}

// A slot whose mark cannot be written is skipped and reported; the notices of
// the other slots of the tick are neither lost nor left marked-but-unsent, and
// the skipped one comes on the next tick.
func TestSlotMarkFailureKeepsOtherNotices(t *testing.T) {
	e, todo := slotEnv(t, "")
	writePlan(t, todo, "Alpha @14h00 due:2026-10-10\nReview @14h00 due:2026-10-10\n")
	db := e.RawDB(t)
	defer db.Close()
	if _, err := db.Exec(`CREATE TRIGGER boom BEFORE INSERT ON slot_notice WHEN NEW.slot_key = '` + planKey("Alpha", "14:00") +
		`' BEGIN SELECT RAISE(ABORT, 'boom'); END`); err != nil {
		t.Fatal(err)
	}
	a := open(t, e)
	slotClock(e, 14, 0, 30)
	evs, err := a.FocusTick(false)
	if err != nil {
		t.Fatalf("FocusTick: %v, want the failing slot skipped", err)
	}
	wantList(t, "first tick", notes(evs), wantNote(focus.TopicStarting, "Starting now", "14:00–17:00 Review"))
	if a.RoutineErr() == nil {
		t.Error("RoutineErr = nil, want the failed mark reported")
	}
	wantList(t, "rows", slotRows(t, e), reviewKey()+"|start")
	if _, err := db.Exec(`DROP TRIGGER boom`); err != nil {
		t.Fatal(err)
	}
	wantList(t, "retry", tickAt(t, a, e, 14, 0, 40), wantNote(focus.TopicStarting, "Starting now", "14:00–17:00 Alpha"))
}

// A session that cannot be started leaves the starting notice unmarked, so
// the next tick of the window tries again.
func TestSlotAutoStartFailureRetries(t *testing.T) {
	e := autoEnv(t, "")
	db := e.RawDB(t)
	defer db.Close()
	if _, err := db.Exec(`CREATE TRIGGER boom BEFORE INSERT ON session BEGIN SELECT RAISE(ABORT, 'boom'); END`); err != nil {
		t.Fatal(err)
	}
	a := open(t, e)
	wantList(t, "14:00:30", tickAt(t, a, e, 14, 0, 30))
	if a.RoutineErr() == nil {
		t.Error("RoutineErr = nil, want the failed start reported")
	}
	wantList(t, "rows", slotRows(t, e))
	if _, err := db.Exec(`DROP TRIGGER boom`); err != nil {
		t.Fatal(err)
	}
	got := tickAt(t, a, e, 14, 0, 40)
	if len(got) != 1 || !strings.Contains(got[0], "focus session started") {
		t.Fatalf("retry = %q, want the started notice", got)
	}
	wantList(t, "sessions", sessionRows(t, a), "manual 14:00:40-14:50:00 end 0")
}

// A notice that was marked but could not be handed over is released and comes
// again.
func TestReleaseSlotNotice(t *testing.T) {
	e, _ := slotEnv(t, "")
	a := open(t, e)
	slotClock(e, 14, 0, 30)
	evs, err := a.FocusTick(false)
	if err != nil || len(evs) != 1 {
		t.Fatalf("tick: %v, %v", evs, err)
	}
	if err := a.ReleaseSlotNotice(evs[0]); err != nil {
		t.Fatal(err)
	}
	if err := a.ReleaseSlotNotice(focus.Event{Kind: focus.Notify}); err != nil {
		t.Fatalf("an event without a slot: %v, want nil", err)
	}
	wantList(t, "rows", slotRows(t, e))
	wantList(t, "next tick", tickAt(t, a, e, 14, 0, 40), wantNote(focus.TopicStarting, "Starting now", "14:00–14:50 Review"))
}

// With [focus] notify off no notice reaches anyone, so none is marked as sent;
// auto_start still starts its session.
func TestSlotFocusNotifyOffMarksNothing(t *testing.T) {
	e, _ := slotEnv(t, "\n[focus]\nnotify = false\n")
	a := open(t, e)
	wantList(t, "13:55", tickAt(t, a, e, 13, 55, 0))
	wantList(t, "14:00", tickAt(t, a, e, 14, 0, 30))
	wantList(t, "rows", slotRows(t, e))

	e2 := autoEnv(t, "\n[focus]\nnotify = false\n")
	a2 := open(t, e2)
	wantList(t, "auto 14:00", tickAt(t, a2, e2, 14, 0, 30))
	wantList(t, "auto sessions", sessionRows(t, a2), "manual 14:00:30-14:50:00 end 0")
	if rows := slotRows(t, e2); len(rows) != 1 || !strings.HasSuffix(rows[0], "|start") {
		t.Errorf("auto rows = %v, want the one start row", rows)
	}
}
