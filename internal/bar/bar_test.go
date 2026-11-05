package bar

import (
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"tock/internal/app"
	"tock/internal/apptest"
	"tock/internal/classify"
	"tock/internal/config"
	"tock/internal/focus"
	"tock/internal/report"
	"tock/internal/store"
)

func openApp(t *testing.T, e *apptest.Env) *app.App {
	t.Helper()
	a, err := app.Open(app.Paths{DB: e.DB, Rules: e.Rules, Learned: e.Learned, Config: e.Config}, e.NowFunc())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { a.Close() })
	return a
}

func seeded(t *testing.T) (*apptest.Env, *app.App) {
	t.Helper()
	e := apptest.New(t, apptest.Rules)
	e.Seed(t, apptest.DaySpans()...)
	e.Heartbeat(t, e.Clock.UnixMilli())
	return e, openApp(t, e)
}

// titles renders a menu for tests: "--" separators, "[strip N]" strip rows,
// "Title ▸" submenu parents with their rows indented, "✓ " checked rows and
// " [id]" after clickable ones.
func titles(items []Item) string {
	var b []string
	var walk func([]Item, string)
	walk = func(items []Item, ind string) {
		for _, it := range items {
			switch {
			case it.Separator:
				b = append(b, ind+"--")
			case len(it.Strip) > 0:
				b = append(b, fmt.Sprintf("%s[strip %d]", ind, len(it.Strip)))
			case len(it.Sub) > 0:
				b = append(b, ind+it.Title+" ▸")
				walk(it.Sub, ind+"  ")
			case it.ID == "":
				b = append(b, ind+it.Title)
			default:
				c := ""
				if it.Checked {
					c = "✓ "
				}
				b = append(b, ind+c+it.Title+" ["+it.ID+"]")
			}
		}
	}
	walk(items, "")
	return strings.Join(b, "\n")
}

// settingsTitles is the Settings submenu as titles renders it for s.
func settingsTitles(s Snapshot) string {
	out := "Settings ▸"
	group := ""
	for _, f := range s.Features {
		if f.Group != group {
			if group != "" {
				out += "\n  --"
			}
			group = f.Group
			out += "\n  " + f.Group
		}
		c := ""
		if f.On {
			c = "✓ "
		}
		out += "\n  " + c + f.Name + " [feature:" + f.ID + "]"
	}
	if len(s.Features) > 0 {
		out += "\n  --"
	}
	for _, l := range strings.Split(titles([]Item{s.privacy()}), "\n") {
		out += "\n  " + l
	}
	return out + "\n  Notifications… [notify-settings]\n  Accessibility… [ax]\n  --\n  Quit tock menu bar [quit]"
}

func TestSnapshotDay(t *testing.T) {
	e, a := seeded(t)
	s, err := Load(a, e.Clock, app.NotifyGranted)
	if err != nil {
		t.Fatal(err)
	}
	now := e.Clock.UnixMilli()
	if got := s.Title(now); got != "◷ 2h54" {
		t.Fatalf("title = %q", got)
	}
	want := `2h54 today · 2h14 focus · score 74
[strip 24]
--
Start 25 min focus [start:25]
More lengths ▸
  Start 50 min focus [start:50]
  Start 90 min focus [start:90]
--
Inbox · 28m [dash:inbox]
Open dashboard [dash]
--
` + settingsTitles(s)
	if got := titles(s.Items(now)); got != want {
		t.Fatalf("menu:\n%s\nwant:\n%s", got, want)
	}

	s.TrackdUp, s.Access = false, app.NotifyDenied
	if got := s.Title(now); got != "◷ 2h54 !" {
		t.Fatalf("down title = %q", got)
	}
	got := titles(s.Items(now))
	if !strings.Contains(got, "trackd is not running\nNotifications are off… [notify-settings]\n--") {
		t.Fatalf("down and denied menu:\n%s", got)
	}
}

func TestSnapshotSession(t *testing.T) {
	e, a := seeded(t)
	if _, err := Menu(a, "start:25"); err != nil {
		t.Fatal(err)
	}
	e.Clock = e.Clock.Add(6*time.Minute + 18*time.Second)
	s, err := Load(a, e.Clock, app.NotifyGranted)
	if err != nil {
		t.Fatal(err)
	}
	now := e.Clock.UnixMilli()
	if got := s.Title(now); got != "◉ 18:42" {
		t.Fatalf("title = %q", got)
	}
	got := titles(s.Items(now))
	if !strings.Contains(got, "Session · 18:42 left · focus 50% · 0 interruptions") || !strings.Contains(got, "Stop session [stop]") ||
		strings.Contains(got, "start:") {
		t.Fatalf("session menu:\n%s", got)
	}
	if msg, err := Menu(a, IDStop); err != nil || msg != "stopped session after 6m" {
		t.Fatalf("stop = %q, %v", msg, err)
	}
	if _, err := Menu(a, IDStop); !errors.Is(err, store.ErrNoSession) {
		t.Fatalf("second stop = %v", err)
	}
	if _, err := Menu(a, "bogus"); !errors.Is(err, ErrUnknown) {
		t.Fatalf("unknown id = %v", err)
	}
}

func TestSnapshotNotSetUp(t *testing.T) {
	for _, c := range []struct {
		why  error
		want string
	}{
		{fmt.Errorf("app: %w", classify.ErrNoRules), "Set up tock [setup]"},
		{app.ErrNoDB, "Waiting for trackd to start"},
		{errors.New("disk on fire"), "tock could not open · see tockbar.log"},
		{nil, "tock could not open · see tockbar.log"},
	} {
		s := NotReady(c.why)
		if s.OK || s.Title(0) != "◷ tock" {
			t.Fatalf("%v: ok %v title %q", c.why, s.OK, s.Title(0))
		}
		if got, want := titles(s.Items(0)), c.want+"\n--\nQuit tock menu bar [quit]"; got != want {
			t.Fatalf("%v: menu:\n%s\nwant:\n%s", c.why, got, want)
		}
	}
}

func TestSnapshotAccessibility(t *testing.T) {
	s := Snapshot{OK: true, TrackdUp: true, AXDenied: true}
	got := titles(s.Items(0))
	if !strings.Contains(got, "Allow Accessibility for trackd… [ax]\n--") {
		t.Fatalf("menu:\n%s", got)
	}
	s.AXDenied = false
	if got := titles(s.Items(0)); strings.Contains(got, "Allow Accessibility") || !strings.Contains(got, "  Accessibility… [ax]") {
		t.Fatalf("granted menu warns about ax, or Settings lacks it:\n%s", got)
	}
}

func TestLoadAccessibility(t *testing.T) {
	e, a := seeded(t)
	for _, c := range []struct {
		v    string
		want bool
	}{{"0", true}, {"1", false}} {
		e.State(t, store.KeyAXTrusted, c.v)
		s, err := Load(a, e.Clock, app.NotifyGranted)
		if err != nil {
			t.Fatal(err)
		}
		if s.AXDenied != c.want {
			t.Fatalf("ax_trusted %s: AXDenied = %v", c.v, s.AXDenied)
		}
	}
}

func TestPresetsCapped(t *testing.T) {
	s := Snapshot{OK: true, TrackdUp: true, Presets: []int{10, 20, 30, 40}}
	if got := titles(s.Items(0)); strings.Contains(got, "40") || !strings.Contains(got, "start:30") {
		t.Fatalf("menu:\n%s", got)
	}
}

func TestButtons(t *testing.T) {
	away := store.Notice{ID: 7, Topic: focus.TopicAway}
	d := app.NoticeData{Labels: []classify.Want{{Category: "Meetings"}, {Category: "Code", Project: "tock"}}}
	want := []Button{{"away:0", "Meetings"}, {"away:1", "Code / tock"}, {"idle", "Leave idle"}}
	if got := Buttons(away, d); !reflect.DeepEqual(got, want) {
		t.Fatalf("away buttons = %+v", got)
	}
	if Category(away, d) != "away-7" || Category(store.Notice{ID: 8, Topic: focus.TopicBreak}, d) != "break-8" ||
		Category(store.Notice{ID: 9, Topic: focus.TopicRecap}, d) != "" ||
		Category(store.Notice{ID: 10, Topic: focus.TopicStart}, d) != "" ||
		Category(store.Notice{ID: 11, Topic: focus.TopicMidday}, d) != "" ||
		Category(store.Notice{ID: 12, Topic: focus.TopicOver}, d) != "" ||
		Category(store.Notice{ID: 13, Topic: focus.TopicStarting}, d) != "" ||
		Category(store.Notice{ID: 14, Topic: focus.TopicUpcoming}, d) != "" {
		t.Fatal("categories")
	}
	for topic, want := range map[string]string{
		focus.TopicFocus: "Stop session", focus.TopicDone: "Start again (50 min)", focus.TopicBreak: "Snooze 30 min",
	} {
		b := Buttons(store.Notice{Topic: topic}, app.NoticeData{Minutes: 50})
		if len(b) != 1 || b[0].Title != want {
			t.Fatalf("%s buttons = %+v", topic, b)
		}
	}
	if b := Buttons(store.Notice{Topic: focus.TopicRecap}, app.NoticeData{}); b != nil {
		t.Fatalf("recap buttons = %+v", b)
	}
	if b := Buttons(store.Notice{Topic: focus.TopicStart}, app.NoticeData{}); b != nil {
		t.Fatalf("start buttons = %+v", b)
	}
	if b := Buttons(store.Notice{Topic: focus.TopicMidday}, app.NoticeData{}); b != nil {
		t.Fatalf("midday buttons = %+v", b)
	}
	if b := Buttons(store.Notice{Topic: focus.TopicOver}, app.NoticeData{}); b != nil {
		t.Fatalf("overwork buttons = %+v", b)
	}
	for _, topic := range []string{focus.TopicStarting, focus.TopicUpcoming} {
		if b := Buttons(store.Notice{Topic: topic}, app.NoticeData{}); b != nil {
			t.Fatalf("%s buttons = %+v", topic, b)
		}
	}
}

func enqueue(t *testing.T, a *app.App, ev focus.Event) int64 {
	t.Helper()
	if err := a.Enqueue(ev); err != nil {
		t.Fatal(err)
	}
	ns, err := a.TakeNotices()
	if err != nil || len(ns) != 1 {
		t.Fatalf("take = %+v, %v", ns, err)
	}
	return ns[0].ID
}

func TestRespond(t *testing.T) {
	e, a := seeded(t)
	done := enqueue(t, a, focus.Event{Topic: focus.TopicDone, Title: focus.TitleDone, Body: "b", Minutes: 50})
	if msg, err := Respond(a, done, ActAgain); err != nil || msg != "started 50 min session" {
		t.Fatalf("again = %q, %v", msg, err)
	}
	if _, err := Respond(a, done, ActAgain); !errors.Is(err, store.ErrSessionRunning) {
		t.Fatalf("again while running = %v", err)
	}
	stop := enqueue(t, a, focus.Event{Topic: focus.TopicFocus, Title: focus.TitleFocus, Body: "b"})
	if msg, err := Respond(a, stop, ActStop); err != nil || !strings.HasPrefix(msg, "stopped session") {
		t.Fatalf("stop = %q, %v", msg, err)
	}
	brk := enqueue(t, a, focus.Event{Topic: focus.TopicBreak, Title: focus.TitleBreak, Body: "b"})
	if msg, err := Respond(a, brk, ActSnooze); err != nil || msg != "break reminders snoozed for 30 min" {
		t.Fatalf("snooze = %q, %v", msg, err)
	}
	// A body click, or an action the notice does not offer, does nothing.
	for _, act := range []string{"com.apple.UNNotificationDefaultActionIdentifier", ActStop, "away:9"} {
		if msg, err := Respond(a, brk, act); err != nil || msg != "" {
			t.Fatalf("%s = %q, %v", act, msg, err)
		}
	}
	if _, err := Respond(a, 9999, ActStop); !errors.Is(err, store.ErrNoNotice) {
		t.Fatalf("missing notice = %v", err)
	}

	// Away: label with the first quick label, then leave another idle.
	if err := a.Answer(classify.Interval{From: apptest.At(t, "08:00", 0), To: apptest.At(t, "08:30", 0)},
		classify.Want{Category: "Communication"}); err != nil {
		t.Fatal(err)
	}
	day, err := a.Day(e.Clock)
	if err != nil {
		t.Fatal(err)
	}
	iv := day.Away
	if len(iv) == 0 {
		t.Fatal("seeded day has no pending absence")
	}
	away := enqueue(t, a, focus.Event{Topic: focus.TopicAway, Title: focus.TitleAway, Body: "b", Away: iv[0]})
	if msg, err := Respond(a, away, "away:0"); err != nil || msg != "labelled away time" {
		t.Fatalf("away label = %q, %v", msg, err)
	}
	d, err := a.Day(e.Clock)
	if err != nil || len(d.Away) != len(iv)-1 {
		t.Fatalf("absence still pending: %+v, %v", d.Away, err)
	}
}

// finished runs a 50 minute manual session from 10:00 to 10:50 over the
// seeded day (40 min of tock, 10 of ATLAS) and leaves the clock at 10:50.
func finished(t *testing.T, e *apptest.Env, a *app.App) focus.Session {
	t.Helper()
	e.Clock = time.UnixMilli(apptest.At(t, "10:00", 0)).In(apptest.Zone)
	if _, err := a.StartSession(50); err != nil {
		t.Fatal(err)
	}
	e.Clock = e.Clock.Add(50 * time.Minute)
	s, err := a.StopSession()
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func doneEvent(s focus.Session, project string, refile bool) focus.Event {
	return focus.Event{Topic: focus.TopicDone, Title: focus.TitleDone, Body: "b", Minutes: 50, Sess: s, Project: project, Refile: refile}
}

func TestDoneButtons(t *testing.T) {
	done := store.Notice{ID: 3, Topic: focus.TopicDone}
	d := app.NoticeData{Session: 42, Minutes: 50, Project: "Analyse 1", Refile: true}
	want := []Button{
		{"rate:5", "Rate ★5"}, {"rate:4", "Rate ★4"}, {"rate:3", "Rate ★3"}, {"rate:2", "Rate ★2"}, {"rate:1", "Rate ★1"},
		{"refile:", "Refile → Analyse 1"}, {"again", "Start again (50 min)"},
	}
	if got := Buttons(done, d); !reflect.DeepEqual(got, want) {
		t.Fatalf("done buttons = %+v", got)
	}
	if Category(done, d) != "done-3" {
		t.Fatalf("category = %q", Category(done, d))
	}
	for name, d := range map[string]app.NoticeData{
		"refile off": {Session: 42, Minutes: 50, Project: "Analyse 1"},
		"no project": {Session: 42, Minutes: 50, Refile: true},
	} {
		got := Buttons(done, d)
		if len(got) != 6 || got[5].ID != ActAgain {
			t.Fatalf("%s: %+v", name, got)
		}
	}
	old := Buttons(done, app.NoticeData{Minutes: 50})
	if !reflect.DeepEqual(old, []Button{{ActAgain, "Start again (50 min)"}}) {
		t.Fatalf("old notice buttons = %+v", old)
	}
	long := Buttons(done, app.NoticeData{Session: 1, Project: strings.Repeat("p", 30), Refile: true})
	if r := []rune(long[5].Title); string(r[len(r)-1]) != "…" || len(r) != len([]rune("Refile → "))+24 {
		t.Fatalf("long project button = %q", long[5].Title)
	}
	if got := Buttons(done, app.NoticeData{}); got != nil || Category(done, app.NoticeData{}) != "" {
		t.Fatalf("empty done = %+v", got)
	}
}

func TestRespondRate(t *testing.T) {
	e, a := seeded(t)
	s := finished(t, e, a)
	id := enqueue(t, a, doneEvent(s, "ATLAS", true))
	if msg, err := Respond(a, id, "rate:4"); err != nil || msg != "rated the 50 min session ★4" {
		t.Fatalf("rate = %q, %v", msg, err)
	}
	got, err := a.SessionByID(s.ID)
	if err != nil || got.Rating != 4 {
		t.Fatalf("stored = %+v, %v", got, err)
	}
	// An action the notice does not offer stores nothing.
	if msg, err := Respond(a, id, "rate:7"); err != nil || msg != "" {
		t.Fatalf("rate:7 = %q, %v", msg, err)
	}
	old := enqueue(t, a, focus.Event{Topic: focus.TopicDone, Title: focus.TitleDone, Body: "b", Minutes: 50})
	if msg, err := Respond(a, old, "rate:2"); err != nil || msg != "" {
		t.Fatalf("rate on old notice = %q, %v", msg, err)
	}
	if got, _ := a.SessionByID(s.ID); got.Rating != 4 {
		t.Fatalf("rating changed to %d", got.Rating)
	}
}

func TestRespondRefile(t *testing.T) {
	e, a := seeded(t)
	s := finished(t, e, a)
	id := enqueue(t, a, doneEvent(s, "ATLAS", true))
	if msg, err := Respond(a, id, "refile:"); err != nil || msg != "refiled 50 min to ATLAS" {
		t.Fatalf("refile = %q, %v", msg, err)
	}
	if ok, err := a.RefileUndoable(s.ID); err != nil || !ok {
		t.Fatalf("undoable = %v, %v", ok, err)
	}
	if _, err := a.Undo(); err != nil {
		t.Fatal(err)
	}
	if ok, _ := a.RefileUndoable(s.ID); ok {
		t.Fatal("still undoable after undo")
	}
	// Not offered (no refile flag): nothing happens.
	id2 := enqueue(t, a, doneEvent(s, "ATLAS", false))
	if msg, err := Respond(a, id2, "refile:"); err != nil || msg != "" {
		t.Fatalf("refile not offered = %q, %v", msg, err)
	}
}

func TestRespondGone(t *testing.T) {
	_, a := seeded(t)
	id := enqueue(t, a, doneEvent(focus.Session{ID: 9999}, "ATLAS", true))
	for _, act := range []string{"rate:3", "refile:"} {
		if msg, err := Respond(a, id, act); err != nil || msg != "that session is gone" {
			t.Fatalf("%s = %q, %v", act, msg, err)
		}
	}
}

func TestRespondDismiss(t *testing.T) {
	e, a := seeded(t)
	s := finished(t, e, a)
	id := enqueue(t, a, doneEvent(s, "ATLAS", true))
	for _, act := range []string{"com.apple.UNNotificationDefaultActionIdentifier", "com.apple.UNNotificationDismissActionIdentifier", ""} {
		if msg, err := Respond(a, id, act); err != nil || msg != "" {
			t.Fatalf("%q = %q, %v", act, msg, err)
		}
	}
	got, err := a.SessionByID(s.ID)
	if err != nil || got.Rating != 0 || got.RatedMS != 0 {
		t.Fatalf("session changed: %+v, %v", got, err)
	}
	if es, _ := a.UndoList(5); len(es) != 0 {
		t.Fatalf("undo journal = %+v", es)
	}
}

// lastRow returns the Last session parent row of the menu at the test clock.
func lastRow(t *testing.T, a *app.App, e *apptest.Env) (Item, []Item) {
	t.Helper()
	s, err := Load(a, e.Clock, app.NotifyGranted)
	if err != nil {
		t.Fatal(err)
	}
	items := s.Items(e.Clock.UnixMilli())
	for _, it := range items {
		if strings.HasPrefix(it.Title, "Last session") {
			return it, items
		}
	}
	return Item{}, items
}

func TestSnapshotLastSession(t *testing.T) {
	e, a := seeded(t)
	s := finished(t, e, a)
	note := "derivation went well"
	if _, _, err := a.RateSession(s.ID, 4, &note); err != nil {
		t.Fatal(err)
	}
	d, err := a.SessionDetail(s.ID)
	if err != nil {
		t.Fatal(err)
	}
	row, items := lastRow(t, a, e)
	wantTitle := fmt.Sprintf("Last session · 50m · Q%d · ★4", d.Q.Score)
	if row.Title != wantTitle || row.ID != "" {
		t.Fatalf("parent = %+v, want title %q", row, wantTitle)
	}
	// First in the context block, after the header, before the unlabelled line.
	var idx, unl = -1, -1
	for i, it := range items {
		if it.Title == row.Title {
			idx = i
		}
		if strings.Contains(it.Title, "unlabelled") {
			unl = i
		}
	}
	if idx < 1 || !items[idx-1].Separator || (unl >= 0 && unl < idx) {
		t.Fatalf("position: last %d, unlabelled %d\n%s", idx, unl, titles(items))
	}
	id := strconv.FormatInt(s.ID, 10)
	want := []Item{
		{Title: d.Summary(apptest.Zone)},
		{Title: d.Q.Line()},
		{Title: "Note: derivation went well"},
		sep,
		{Title: "Rate ★5", ID: "rate:5:" + id},
		{Title: "Rate ★4", ID: "rate:4:" + id, Checked: true},
		{Title: "Rate ★3", ID: "rate:3:" + id},
		{Title: "Rate ★2", ID: "rate:2:" + id},
		{Title: "Rate ★1", ID: "rate:1:" + id},
		sep,
		{Title: "Refile to tock", ID: "refile:" + id},
	}
	if !reflect.DeepEqual(row.Sub, want) {
		t.Fatalf("sub = %+v\nwant %+v", row.Sub, want)
	}
	if !strings.HasPrefix(row.Sub[0].Title, "10:00–10:50 · ") {
		t.Fatalf("summary = %q", row.Sub[0].Title)
	}

	// Unrated, no note: no check mark, no note row, no star suffix.
	e2, a2 := seeded(t)
	finished(t, e2, a2)
	row2, _ := lastRow(t, a2, e2)
	if strings.Contains(row2.Title, "★") || strings.Contains(titlesSub(row2.Sub), "Note:") {
		t.Fatalf("unrated row: %+v", row2)
	}
	for _, it := range row2.Sub {
		if it.Checked {
			t.Fatalf("checked item in unrated menu: %+v", it)
		}
	}

	// More than 24 hours later the row is gone.
	e.Clock = e.Clock.Add(24*time.Hour + time.Minute)
	if row, _ := lastRow(t, a, e); row.Title != "" {
		t.Fatalf("stale session still shown: %+v", row)
	}
	e.Clock = e.Clock.Add(-time.Minute)
	if row, _ := lastRow(t, a, e); row.Title == "" {
		t.Fatal("session 24 h old should still show")
	}
}

func titlesSub(items []Item) string { return titles(items) }

func TestSnapshotUndoRefile(t *testing.T) {
	e, a := seeded(t)
	s := finished(t, e, a)
	if _, _, err := a.RefileSession(s.ID, "ATLAS"); err != nil {
		t.Fatal(err)
	}
	row, _ := lastRow(t, a, e)
	id := strconv.FormatInt(s.ID, 10)
	if last := row.Sub[len(row.Sub)-1]; last.Title != "Undo refile" || last.ID != "unrefile:"+id {
		t.Fatalf("sub = %s", titles(row.Sub))
	}
	if _, err := a.AddEdit(classify.EditLabel, apptest.At(t, "09:00", 0), apptest.At(t, "09:05", 0), classify.Want{Category: "Code"}, ""); err != nil {
		t.Fatal(err)
	}
	row, _ = lastRow(t, a, e)
	if strings.Contains(titles(row.Sub), "Undo refile") {
		t.Fatalf("undo offered after a newer edit: %s", titles(row.Sub))
	}
}

func TestMenuRate(t *testing.T) {
	e, a := seeded(t)
	s := finished(t, e, a)
	id := strconv.FormatInt(s.ID, 10)
	if msg, err := Menu(a, "rate:4:"+id); err != nil || msg != "rated the 50 min session ★4" {
		t.Fatalf("rate = %q, %v", msg, err)
	}
	if got, _ := a.SessionByID(s.ID); got.Rating != 4 {
		t.Fatalf("stored = %+v", got)
	}
	for _, bad := range []string{"rate:", "rate:4", "rate:4:", "rate:x:" + id, "rate:0:" + id, "rate:6:" + id, "rate:4:0", "rate:4:-1", "rate:4:x"} {
		if _, err := Menu(a, bad); !errors.Is(err, ErrUnknown) {
			t.Fatalf("%q = %v, want ErrUnknown", bad, err)
		}
	}
	if msg, err := Menu(a, "rate:3:9999"); err != nil || msg != "that session is gone" {
		t.Fatalf("gone = %q, %v", msg, err)
	}
}

func TestMenuRefile(t *testing.T) {
	e, a := seeded(t)
	s := finished(t, e, a)
	id := strconv.FormatInt(s.ID, 10)
	if msg, err := Menu(a, "refile:"+id); err != nil || msg != "refiled 50 min to tock" {
		t.Fatalf("refile = %q, %v", msg, err)
	}
	if _, err := Menu(a, "refile:x"); !errors.Is(err, ErrUnknown) {
		t.Fatalf("bad id = %v", err)
	}
	if msg, err := Menu(a, "refile:9999"); err != nil || msg != "that session is gone" {
		t.Fatalf("gone = %q, %v", msg, err)
	}
}

func TestUnrefile(t *testing.T) {
	e, a := seeded(t)
	s := finished(t, e, a)
	id := strconv.FormatInt(s.ID, 10)
	if _, err := Menu(a, "refile:"+id); err != nil {
		t.Fatal(err)
	}
	if msg, err := Menu(a, "unrefile:"+id); err != nil || msg != "undid the refile" {
		t.Fatalf("unrefile = %q, %v", msg, err)
	}
	if es, _ := a.UndoList(5); len(es) != 0 {
		t.Fatalf("journal after undo = %+v", es)
	}

	// A newer change blocks it and nothing is undone.
	if _, err := Menu(a, "refile:"+id); err != nil {
		t.Fatal(err)
	}
	if _, err := a.AddEdit(classify.EditLabel, apptest.At(t, "09:00", 0), apptest.At(t, "09:05", 0), classify.Want{Category: "Code"}, ""); err != nil {
		t.Fatal(err)
	}
	before, _ := a.UndoList(5)
	if _, err := Menu(a, "unrefile:"+id); err == nil {
		t.Fatal("unrefile after a newer edit succeeded")
	}
	if after, _ := a.UndoList(5); len(after) != len(before) {
		t.Fatalf("journal changed: %d -> %d", len(before), len(after))
	}
}

// The menu offers a refile only when it would change something: not when the
// session is already filed to its top project, and not when that project is
// archived and so cannot be chosen.
func TestSnapshotRefileOnlyWhenItChangesSomething(t *testing.T) {
	e, a := seeded(t)
	s := finished(t, e, a)
	row, _ := lastRow(t, a, e)
	if !strings.Contains(titles(row.Sub), "Refile to tock") {
		t.Fatalf("not offered when needed: %s", titles(row.Sub))
	}
	id := strconv.FormatInt(s.ID, 10)
	if _, err := Menu(a, "refile:"+id); err != nil {
		t.Fatal(err)
	}
	row, _ = lastRow(t, a, e)
	if strings.Contains(titles(row.Sub), "Refile to") {
		t.Fatalf("offered on a session already filed: %s", titles(row.Sub))
	}
	// A stale click does nothing: no edits, no new undo entry.
	before, _ := a.UndoList(5)
	if msg, err := Menu(a, "refile:"+id); err != nil || msg != "nothing to refile: already filed to tock" {
		t.Fatalf("stale refile = %q, %v", msg, err)
	}
	if after, _ := a.UndoList(5); len(after) != len(before) {
		t.Fatalf("journal grew: %d -> %d", len(before), len(after))
	}
}

func TestSnapshotNoRefileToArchivedProject(t *testing.T) {
	e, a := seeded(t)
	finished(t, e, a)
	e.WriteRules(t, strings.Replace(apptest.Rules, "[[project]]\nname = \"tock\"\n", "[[project]]\nname = \"tock\"\narchived = true\n", 1))
	if _, err := a.Reload(); err != nil {
		t.Fatal(err)
	}
	row, _ := lastRow(t, a, e)
	if row.Title == "" || strings.Contains(titles(row.Sub), "Refile to") {
		t.Fatalf("archived project offered: %q\n%s", row.Title, titles(row.Sub))
	}
}

// A failure reading the last session costs the menu that one row, not the
// title, countdown and start items.
func TestLoadSurvivesLastSessionFailure(t *testing.T) {
	e, a := seeded(t)
	finished(t, e, a)
	if _, err := e.RawDB(t).Exec(`DROP TABLE undo`); err != nil {
		t.Fatal(err)
	}
	s, err := Load(a, e.Clock, app.NotifyGranted)
	if err != nil {
		t.Fatalf("Load = %v, want the snapshot without the last session", err)
	}
	if s.Last != nil || s.LastErr == nil {
		t.Fatalf("Last = %+v, LastErr = %v", s.Last, s.LastErr)
	}
	if got := titles(s.Items(e.Clock.UnixMilli())); !strings.Contains(got, "Start 50 min focus") || strings.Contains(got, "Last session") {
		t.Fatalf("menu:\n%s", got)
	}
	if s.TrackedMS == 0 || !s.OK {
		t.Fatalf("snapshot lost the day: %+v", s)
	}
}

// distractedSession starts a 25 min session at 14:50 in an env whose last 5
// minutes are YouTube, so a distraction notice is due at 15:00.
func distractedSession(t *testing.T) (*apptest.Env, *app.App, focus.Session) {
	t.Helper()
	e := apptest.New(t, apptest.Rules)
	e.Seed(t,
		apptest.Span{ID: 1, Start: "14:50", End: "14:55", Bundle: "term", App: "WezTerm", Title: "tock │ a"},
		apptest.Span{ID: 2, Start: "14:55", Bundle: "arc", App: "Arc", Title: "Video", URL: "www.youtube.com/watch"},
	)
	a := openApp(t, e)
	e.Clock = time.UnixMilli(apptest.At(t, "14:50", 0)).In(apptest.Zone)
	s, err := a.StartSession(25)
	if err != nil {
		t.Fatal(err)
	}
	return e, a, s
}

// at moves the env clock to hh:mm today.
func at(t *testing.T, e *apptest.Env, hhmm string) {
	t.Helper()
	e.Clock = time.UnixMilli(apptest.At(t, hhmm, 0)).In(apptest.Zone)
}

// focusNotes are the distraction notices among the tick's events.
func focusNotes(t *testing.T, a *app.App) []focus.Event {
	t.Helper()
	notes, err := a.FocusTick(false)
	if err != nil {
		t.Fatal(err)
	}
	var out []focus.Event
	for _, n := range notes {
		if n.Topic == focus.TopicFocus {
			out = append(out, n)
		}
	}
	return out
}

func TestDistractSnoozeEndToEnd(t *testing.T) {
	e, a, s := distractedSession(t)
	at(t, e, "15:00")
	notes := focusNotes(t, a)
	if len(notes) != 1 || notes[0].Sess.ID != s.ID {
		t.Fatalf("first tick notes = %+v, want one for session %d", notes, s.ID)
	}
	id := enqueue(t, a, notes[0])
	n, d, err := a.Notice(id)
	if err != nil {
		t.Fatal(err)
	}
	sid := strconv.FormatInt(s.ID, 10)
	want := []Button{{ActSnoozeDistract, "Snooze 10 min"}, {ActQuietCat + sid, "Don't interrupt for Entertainment"},
		{ActQuietRule + sid, "Don't interrupt for this rule"}, {ActStop, "Stop session"}}
	if got := Buttons(n, d); !reflect.DeepEqual(got, want) {
		t.Fatalf("buttons = %+v, want %+v", got, want)
	}
	if msg, err := Respond(a, id, ActSnoozeDistract); err != nil || msg != "distraction notices snoozed until 15:10" {
		t.Fatalf("respond = %q, %v", msg, err)
	}

	at(t, e, "15:03")
	if notes := focusNotes(t, a); len(notes) != 0 {
		t.Fatalf("tick while snoozed notified: %+v", notes)
	}
	f, err := a.Focus(e.Clock)
	if err != nil || f.Open == nil || f.Open.Interruptions != 2 {
		t.Fatalf("while snoozed: %+v, %v; want 2 interruptions", f.Open, err)
	}

	at(t, e, "15:11")
	if notes := focusNotes(t, a); len(notes) != 1 {
		t.Fatalf("tick after the snooze notified %d times, want once", len(notes))
	}
	f, err = a.Focus(e.Clock)
	if err != nil || f.Open == nil || f.Open.Interruptions != 3 {
		t.Fatalf("after snooze: %+v, %v; want 3 interruptions", f.Open, err)
	}
}

func TestSnoozeIgnoresStaleNotice(t *testing.T) {
	e, a, s := distractedSession(t)
	at(t, e, "15:00")
	id := enqueue(t, a, focusNotes(t, a)[0])
	if _, err := a.StopSession(); err != nil {
		t.Fatal(err)
	}
	s2, err := a.StartSession(25)
	if err != nil || s2.ID == s.ID {
		t.Fatalf("second session = %+v, %v", s2, err)
	}
	if msg, err := Respond(a, id, ActSnoozeDistract); err != nil || msg != "that session is no longer running, nothing snoozed" {
		t.Fatalf("late click = %q, %v", msg, err)
	}
	if was, err := a.ResumeDistract(); err != nil || was {
		t.Fatalf("late click snoozed the new session: was = %v, %v", was, err)
	}
}

func TestSnapshotSnooze(t *testing.T) {
	e, a, _ := distractedSession(t)
	at(t, e, "15:00")
	load := func() string {
		t.Helper()
		s, err := Load(a, e.Clock, app.NotifyGranted)
		if err != nil {
			t.Fatal(err)
		}
		return titles(s.Items(e.Clock.UnixMilli()))
	}
	got := load()
	for _, row := range []string{"Snooze distraction notices", "Notices snoozed until", "Resume notices"} {
		if strings.Contains(got, row) {
			t.Fatalf("menu before any distraction notice has %q:\n%s", row, got)
		}
	}

	for _, n := range focusNotes(t, a) {
		enqueue(t, a, n)
	}
	got = load()
	if !strings.Contains(got, "--\nSnooze distraction notices (10 min) [snooze:distract]\n") ||
		strings.Contains(got, "Resume notices") {
		t.Fatalf("menu after a notice:\n%s", got)
	}

	if _, err := Menu(a, IDSnoozeDistract); err != nil {
		t.Fatal(err)
	}
	got = load()
	i := strings.Index(got, "Session · ")
	rest := got[i:]
	rest = rest[strings.Index(rest, "\n")+1:]
	if !strings.HasPrefix(rest, "Notices snoozed until 15:10\n") || !strings.Contains(got, "Resume notices [snooze:off]") ||
		strings.Contains(got, "Snooze distraction notices") {
		t.Fatalf("snoozed menu:\n%s", got)
	}

	at(t, e, "15:11")
	if got = load(); !strings.Contains(got, "Snooze distraction notices (10 min) [snooze:distract]") || strings.Contains(got, "Resume notices") {
		t.Fatalf("expired snooze menu:\n%s", got)
	}
	at(t, e, "15:16")
	if got = load(); strings.Contains(got, "Snooze distraction notices") {
		t.Fatalf("16 min after the notice:\n%s", got)
	}
}

func TestMenuSnooze(t *testing.T) {
	_, a := seeded(t)
	if msg, err := Menu(a, IDSnoozeDistract); err != nil || msg != "no session running, nothing snoozed" {
		t.Fatalf("snooze with no session = %q, %v", msg, err)
	}
	if _, err := Menu(a, "start:25"); err != nil {
		t.Fatal(err)
	}
	if msg, err := Menu(a, IDSnoozeDistract); err != nil || msg != "distraction notices snoozed until 15:10" {
		t.Fatalf("snooze = %q, %v", msg, err)
	}
	if msg, err := Menu(a, IDSnoozeOff); err != nil || msg != "distraction notices back on" {
		t.Fatalf("resume = %q, %v", msg, err)
	}
}

func TestLoadFollowsConfig(t *testing.T) {
	e, a, _ := distractedSession(t)
	at(t, e, "15:00")
	for _, n := range focusNotes(t, a) {
		enqueue(t, a, n)
	}
	row := func() string {
		t.Helper()
		s, err := Load(a, e.Clock, app.NotifyGranted)
		if err != nil {
			t.Fatal(err)
		}
		return titles(s.Items(e.Clock.UnixMilli()))
	}
	if got := row(); !strings.Contains(got, "Snooze distraction notices (10 min)") {
		t.Fatalf("before the edit:\n%s", got)
	}
	e.WriteConfig(t, "[focus]\nsnooze = 20\n")
	if got := row(); !strings.Contains(got, "Snooze distraction notices (20 min)") {
		t.Fatalf("after the edit:\n%s", got)
	}
}

func TestQuietButtons(t *testing.T) {
	n := store.Notice{ID: 3, Topic: focus.TopicFocus}
	d := app.NoticeData{Session: 7, SnoozeMin: 10, Category: "Entertainment", Rule: "rules.toml#3", RuleFP: "x"}
	want := []Button{{"snooze:distract", "Snooze 10 min"}, {"quiet:cat:7", "Don't interrupt for Entertainment"},
		{"quiet:rule:7", "Don't interrupt for this rule"}, {"stop", "Stop session"}}
	if got := Buttons(n, d); !reflect.DeepEqual(got, want) {
		t.Fatalf("buttons = %+v, want %+v", got, want)
	}
	d.Rule = ""
	if got := Buttons(n, d); !reflect.DeepEqual(got, []Button{want[0], want[1], want[3]}) {
		t.Fatalf("no rule: buttons = %+v", got)
	}
	d.Category = ""
	if got := Buttons(n, d); !reflect.DeepEqual(got, []Button{want[0], want[3]}) {
		t.Fatalf("no category: buttons = %+v", got)
	}
	d.Category = strings.Repeat("W", 30)
	got := Buttons(n, d)
	name := strings.TrimPrefix(got[1].Title, "Don't interrupt for ")
	if !strings.HasSuffix(name, "…") || len([]rune(name)) != ProjectCells {
		t.Fatalf("long name button = %q, want the name cut to %d cells with …", got[1].Title, ProjectCells)
	}
}

func TestQuietButtonsNoTitleOrURL(t *testing.T) {
	e, a, _ := distractedSession(t)
	at(t, e, "15:00")
	id := enqueue(t, a, focusNotes(t, a)[0])
	n, d, err := a.Notice(id)
	if err != nil {
		t.Fatal(err)
	}
	for _, b := range Buttons(n, d) {
		if strings.Contains(strings.ToLower(b.Title), "youtube") || strings.Contains(b.Title, "Video") || strings.Contains(b.ID, "youtube") {
			t.Fatalf("button leaks a title or URL: %+v", b)
		}
	}
	if strings.Contains(n.Body, "Video") || strings.Contains(n.Body, "youtube") {
		t.Fatalf("body leaks a title or URL: %q", n.Body)
	}
}

func TestQuietPickedUp(t *testing.T) {
	e, a, s := distractedSession(t)
	at(t, e, "15:00")
	notes := focusNotes(t, a)
	if len(notes) != 1 {
		t.Fatalf("first tick notes = %+v", notes)
	}
	id := enqueue(t, a, notes[0])
	b := openApp(t, e) // tockbar: another process on the same files
	if msg, err := Respond(b, id, ActQuietCat+strconv.FormatInt(s.ID, 10)); err != nil || msg != "Entertainment will no longer interrupt a session" {
		t.Fatalf("respond = %q, %v", msg, err)
	}
	rules, err := os.ReadFile(e.Rules)
	if err != nil || !strings.Contains(string(rules), "name = \"Entertainment\"\ninterrupt = false\n") {
		t.Fatalf("rules.toml (%v):\n%s", err, rules)
	}
	if _, err := os.Stat(e.Rules + ".bak"); err != nil {
		t.Fatalf("no rules.toml.bak: %v", err)
	}
	for _, hhmm := range []string{"15:05", "15:09"} {
		at(t, e, hhmm)
		if notes := focusNotes(t, a); len(notes) != 0 {
			t.Fatalf("%s: tick after the mark notified: %+v", hhmm, notes)
		}
	}
	f, err := a.Focus(e.Clock)
	if err != nil || f.Open == nil || f.Open.Interruptions != 1 || f.Open.DistractMS != 14*60_000 {
		t.Fatalf("after the mark: %+v, %v; want 1 interruption and 14 min distracted", f.Open, err)
	}
}

func TestRespondQuietRule(t *testing.T) {
	e, a, s := distractedSession(t)
	at(t, e, "15:00")
	id := enqueue(t, a, focusNotes(t, a)[0])
	sid := strconv.FormatInt(s.ID, 10)
	// A foreign session id is not one of the notice's buttons.
	if msg, err := Respond(a, id, ActQuietRule+"999"); err != nil || msg != "" {
		t.Fatalf("forged id = %q, %v; want nothing", msg, err)
	}
	if got, _ := os.ReadFile(e.Rules); string(got) != apptest.Rules {
		t.Fatal("a forged id changed rules.toml")
	}
	if msg, err := Respond(a, id, ActQuietRule+sid); err != nil || msg != "that rule will no longer interrupt a session" {
		t.Fatalf("respond = %q, %v", msg, err)
	}
	got, _ := os.ReadFile(e.Rules)
	if strings.Count(string(got), "interrupt = false") != 1 || !strings.Contains(string(got), "[[rule]]\ninterrupt = false\nurl = ") {
		t.Fatalf("rules.toml:\n%s", got)
	}

}

func TestRespondQuietRuleStale(t *testing.T) {
	e, a, s := distractedSession(t)
	at(t, e, "15:00")
	id := enqueue(t, a, focusNotes(t, a)[0])
	// A rule inserted before the YouTube rule after the notice.
	moved := strings.Replace(apptest.Rules, "[[rule]]\nbundle = [\"term\"]\ntitle",
		"[[rule]]\napp = \"Slack\"\ncategory = \"Communication\"\n\n[[rule]]\nbundle = [\"term\"]\ntitle", 1)
	e.WriteRules(t, moved)
	if msg, err := Respond(a, id, ActQuietRule+strconv.FormatInt(s.ID, 10)); err != nil || msg != "rule changed, not marked (open tock to pick it again)" {
		t.Fatalf("stale respond = %q, %v", msg, err)
	}
	if after, _ := os.ReadFile(e.Rules); string(after) != moved {
		t.Fatal("a stale click changed rules.toml")
	}
}

func TestRespondQuietCategoryGone(t *testing.T) {
	e, a, s := distractedSession(t)
	at(t, e, "15:00")
	id := enqueue(t, a, focusNotes(t, a)[0])
	if _, err := a.RenameName(classify.NameCategory, "Entertainment", "Videos"); err != nil {
		t.Fatal(err)
	}
	before, _ := os.ReadFile(e.Rules)
	if msg, err := Respond(a, id, ActQuietCat+strconv.FormatInt(s.ID, 10)); err != nil || msg != "that category is gone" {
		t.Fatalf("respond = %q, %v", msg, err)
	}
	if after, _ := os.ReadFile(e.Rules); string(after) != string(before) {
		t.Fatal("a click on a gone category changed rules.toml")
	}
}

func TestUrgeButtons(t *testing.T) {
	n := store.Notice{ID: 3, Topic: focus.TopicFocus}
	d := app.NoticeData{Session: 7, Urge: true, SnoozeMin: 10, Category: "Entertainment", Rule: "rules.toml#3", RuleFP: "x"}
	want := []Button{{"urge:resisted:7", "Back to work"}, {"urge:given:7", "Open anyway (counted)"},
		{"snooze:distract", "Snooze 10 min"}, {"quiet:cat:7", "Don't interrupt for Entertainment"},
		{"quiet:rule:7", "Don't interrupt for this rule"}, {"stop", "Stop session"}}
	if got := Buttons(n, d); !reflect.DeepEqual(got, want) {
		t.Fatalf("buttons = %+v, want %+v", got, want)
	}
	d.Urge = false
	if got := Buttons(n, d); !reflect.DeepEqual(got, want[2:]) {
		t.Fatalf("urge off: buttons = %+v, want %+v", got, want[2:])
	}
	d.Urge, d.Session = true, 0
	for _, b := range Buttons(n, d) {
		if strings.HasPrefix(b.ID, "urge:") {
			t.Fatalf("session 0 offers %+v, want no urge buttons", b)
		}
	}
}

// urgeEnv is a distracted session with urge surfing on, and the id of the
// notice enqueued at 15:00.
func urgeEnv(t *testing.T) (*apptest.Env, *app.App, focus.Session, int64) {
	t.Helper()
	e, a, s := distractedSession(t)
	e.WriteConfig(t, "[focus]\nurge_surf = true\n")
	a = openApp(t, e)
	at(t, e, "15:00")
	notes := focusNotes(t, a)
	if len(notes) != 1 {
		t.Fatalf("tick notes = %+v, want one", notes)
	}
	if !strings.HasPrefix(notes[0].Body, "Pause. Breathe. Name the urge.\n") {
		t.Fatalf("body = %q, want the urge prompt first", notes[0].Body)
	}
	return e, a, s, enqueue(t, a, notes[0])
}

func TestUrgeEndToEnd(t *testing.T) {
	e, a, s, id := urgeEnv(t)
	sid := strconv.FormatInt(s.ID, 10)
	if msg, err := Respond(a, id, ActUrgeResisted+sid); err != nil || msg != "counted: resisted" {
		t.Fatalf("resisted = %q, %v", msg, err)
	}
	if msg, err := Respond(a, id, ActStop); err != nil || !strings.HasPrefix(msg, "stopped session") {
		t.Fatalf("stop = %q, %v", msg, err)
	}
	// Late answer: the session has ended but still counts.
	before, err := a.SessionDetail(s.ID)
	if err != nil {
		t.Fatal(err)
	}
	if msg, err := Respond(a, id, ActUrgeGiven+sid); err != nil || msg != "counted: gave in" {
		t.Fatalf("late given = %q, %v", msg, err)
	}
	after, err := a.SessionDetail(s.ID)
	if err != nil {
		t.Fatal(err)
	}
	if after.Sess.UrgesResisted != 1 || after.Sess.UrgesGiven != 1 {
		t.Fatalf("session urges = %d resisted, %d given, want 1, 1", after.Sess.UrgesResisted, after.Sess.UrgesGiven)
	}
	if before.Q != after.Q {
		t.Fatalf("quality changed by an urge answer: %+v -> %+v", before.Q, after.Q)
	}
	row, _ := lastRow(t, a, e)
	var got []string
	for _, it := range row.Sub {
		got = append(got, it.Title)
	}
	if len(got) < 3 || got[1] != after.Q.Line() || got[2] != "urges 1 resisted · 1 given in" {
		t.Fatalf("last session submenu = %q, want the urges line right after the breakdown", got)
	}
}

func TestUrgeUnanswered(t *testing.T) {
	e, a, s, id := urgeEnv(t)
	for _, action := range []string{"", "dismiss", "click", "urge:resisted:" + strconv.FormatInt(s.ID+1, 10), "urge:given"} {
		if msg, err := Respond(a, id, action); err != nil || msg != "" {
			t.Fatalf("Respond(%q) = %q, %v; want no effect", action, msg, err)
		}
	}
	if _, err := a.StopSession(); err != nil {
		t.Fatal(err)
	}
	d, err := a.SessionDetail(s.ID)
	if err != nil || d.Sess.UrgesResisted != 0 || d.Sess.UrgesGiven != 0 {
		t.Fatalf("unanswered: %+v, %v; want 0 and 0", d.Sess, err)
	}
	row, _ := lastRow(t, a, e)
	for _, it := range row.Sub {
		if strings.HasPrefix(it.Title, "urges") {
			t.Fatalf("submenu has %q with no answers", it.Title)
		}
	}
}

func TestUrgeGone(t *testing.T) {
	e, a, s, id := urgeEnv(t)
	db, err := sql.Open("sqlite", "file:"+e.DB)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.Exec(`DELETE FROM session WHERE id=?`, s.ID); err != nil {
		t.Fatal(err)
	}
	sid := strconv.FormatInt(s.ID, 10)
	if msg, err := Respond(a, id, ActUrgeResisted+sid); err != nil || msg != "that session is gone" {
		t.Fatalf("respond = %q, %v; want %q", msg, err, "that session is gone")
	}
}

const meetLink = "https://meet.google.com/abc-defg-hij"

func slotNotify(topic, link string) focus.Event {
	return focus.Event{Kind: focus.Notify, Topic: topic, Title: "t", Body: "b",
		Slot: focus.Slot{Key: "event:1:x", Kind: focus.SlotEvent, JoinURL: link}}
}

func TestJoinButtons(t *testing.T) {
	_, a := seeded(t)
	for _, topic := range []string{focus.TopicUpcoming, focus.TopicStarting} {
		id := enqueue(t, a, slotNotify(topic, meetLink))
		n, d, err := a.Notice(id)
		if err != nil || d.Join != meetLink {
			t.Fatalf("%s data = %+v, %v", topic, d, err)
		}
		if b := Buttons(n, d); len(b) != 1 || b[0] != (Button{"join", "Join"}) {
			t.Fatalf("%s buttons = %+v", topic, b)
		}
		if got := Category(n, d); got != topic+"-"+strconv.FormatInt(id, 10) {
			t.Fatalf("%s category = %q", topic, got)
		}
	}
	id := enqueue(t, a, slotNotify(focus.TopicStarting, "https://evil.example/x"))
	n, d, _ := a.Notice(id)
	if d.Join != "" || Buttons(n, d) != nil || Category(n, d) != "" {
		t.Fatalf("bad link kept: %+v %+v", d, Buttons(n, d))
	}
	id = enqueue(t, a, focus.Event{Kind: focus.Notify, Topic: focus.TopicStarting, Title: "t", Body: "b",
		Slot: focus.Slot{Key: "plan:x", Kind: focus.SlotPlan}})
	n, d, _ = a.Notice(id)
	if Buttons(n, d) != nil {
		t.Fatalf("plan slot buttons = %+v", Buttons(n, d))
	}
}

func TestRespondJoin(t *testing.T) {
	e, a := seeded(t)
	id := enqueue(t, a, slotNotify(focus.TopicStarting, meetLink))
	rows := func() (n int) {
		for _, tbl := range []string{"span", "session", "override", "event", "slot_notice"} {
			var c int
			if err := e.RawDB(t).QueryRow("SELECT COUNT(*) FROM " + tbl).Scan(&c); err != nil {
				t.Fatal(err)
			}
			n = n*1000 + c
		}
		return n
	}
	before := rows()
	msg, err := Respond(a, id, ActJoin)
	if err != nil || msg == "" {
		t.Fatalf("join = %q, %v", msg, err)
	}
	if strings.Contains(msg, "meet.google") {
		t.Fatalf("message holds the link: %q", msg)
	}
	if rows() != before {
		t.Fatal("join changed the rows")
	}
	plain := enqueue(t, a, slotNotify(focus.TopicStarting, ""))
	if msg, err := Respond(a, plain, ActJoin); err != nil || msg != "" {
		t.Fatalf("join without a link = %q, %v", msg, err)
	}
}

func TestJoinURL(t *testing.T) {
	for _, c := range []struct {
		topic, join, want string
	}{
		{focus.TopicUpcoming, meetLink, meetLink},
		{focus.TopicStarting, meetLink, meetLink},
		{focus.TopicStarting, "javascript:x", ""},
		{focus.TopicStarting, "", ""},
		{focus.TopicDone, meetLink, ""},
		{focus.TopicFocus, meetLink, ""},
	} {
		if got := JoinURL(store.Notice{Topic: c.topic}, app.NoticeData{Join: c.join}); got != c.want {
			t.Errorf("JoinURL(%s, %q) = %q, want %q", c.topic, c.join, got, c.want)
		}
	}
}

func startingEvent(session int64, minutes int, link string) focus.Event {
	return focus.Event{Kind: focus.Notify, Topic: focus.TopicStarting, Title: "Starting now", Body: "b",
		Minutes: minutes, AutoStart: session > 0, Sess: focus.Session{ID: session},
		Slot: focus.Slot{Key: "plan:x", Kind: focus.SlotPlan, JoinURL: link}}
}

func TestSlotStartButtons(t *testing.T) {
	_, a := seeded(t)
	for _, c := range []struct {
		name string
		ev   focus.Event
		want []Button
	}{
		{"auto-started", startingEvent(7, 0, ""), []Button{{"stop", "Stop session"}}},
		{"auto-started with a link", startingEvent(7, 0, meetLink), []Button{{"join", "Join"}, {"stop", "Stop session"}}},
		{"start focus", startingEvent(0, 50, ""), []Button{{"again", "Start focus (50 min)"}}},
		{"nothing to offer", startingEvent(0, 0, ""), nil},
		{"up next", func() focus.Event {
			e := startingEvent(0, 50, "")
			e.Topic = focus.TopicUpcoming
			return e
		}(), nil},
	} {
		id := enqueue(t, a, c.ev)
		n, d, err := a.Notice(id)
		if err != nil {
			t.Fatal(err)
		}
		if got := Buttons(n, d); !reflect.DeepEqual(got, c.want) {
			t.Errorf("%s: buttons = %+v, want %+v", c.name, got, c.want)
		}
	}
}

func TestRespondStartFocus(t *testing.T) {
	_, a := seeded(t)
	id := enqueue(t, a, startingEvent(0, 50, ""))
	if msg, err := Respond(a, id, ActAgain); err != nil || msg != "started 50 min session" {
		t.Fatalf("start focus = %q, %v", msg, err)
	}
	s, err := a.RunningSessionID()
	if err != nil || s == 0 {
		t.Fatalf("RunningSessionID = %d, %v, want the new session", s, err)
	}
}

func TestRespondStopStale(t *testing.T) {
	_, a := seeded(t)
	first, err := a.StartSession(50)
	if err != nil {
		t.Fatal(err)
	}
	id := enqueue(t, a, startingEvent(first.ID, 0, ""))
	if _, err := a.StopSession(); err != nil {
		t.Fatal(err)
	}
	second, err := a.StartSession(25)
	if err != nil {
		t.Fatal(err)
	}
	if msg, err := Respond(a, id, ActStop); err != nil || msg != "that session is no longer running" {
		t.Fatalf("stale stop = %q, %v", msg, err)
	}
	if got, _ := a.RunningSessionID(); got != second.ID {
		t.Fatalf("running session = %d, want %d still running", got, second.ID)
	}
	// Nothing runs at all: also stale.
	if _, err := a.StopSession(); err != nil {
		t.Fatal(err)
	}
	if msg, err := Respond(a, id, ActStop); err != nil || msg != "that session is no longer running" {
		t.Fatalf("stop with none running = %q, %v", msg, err)
	}
	// The session the notice started is running: stop it.
	third, err := a.StartSession(50)
	if err != nil {
		t.Fatal(err)
	}
	id = enqueue(t, a, startingEvent(third.ID, 0, ""))
	if msg, err := Respond(a, id, ActStop); err != nil || !strings.HasPrefix(msg, "stopped session") {
		t.Fatalf("stop = %q, %v", msg, err)
	}
	if got, _ := a.RunningSessionID(); got != 0 {
		t.Fatalf("running session = %d, want none", got)
	}
}

// slotSnap is a menu snapshot with the given slots, seen in the apptest zone.
func slotSnap(slots ...focus.Slot) Snapshot {
	return Snapshot{OK: true, TrackdUp: true, Presets: []int{25}, Slots: slots, Loc: apptest.Zone}
}

func planSlot(t *testing.T, start, end, title string) focus.Slot {
	t.Helper()
	return focus.Slot{Key: "plan:" + start, Kind: focus.SlotPlan, StartMS: apptest.At(t, start, 0), EndMS: apptest.At(t, end, 0), Title: title}
}

func eventSlot(t *testing.T, start, end, title, link string) focus.Slot {
	t.Helper()
	return focus.Slot{Key: "event:" + start, Kind: focus.SlotEvent, StartMS: apptest.At(t, start, 0), EndMS: apptest.At(t, end, 0), Title: title, JoinURL: link}
}

func TestSnapshotNext(t *testing.T) {
	now := apptest.At(t, "13:48", 0)
	plain := titles(slotSnap().Items(now))
	if strings.Contains(plain, "Next:") || strings.Contains(plain, "Join") {
		t.Fatalf("no slots, menu:\n%s", plain)
	}
	// The Next row opens the context block, between the header and the start items.
	tail := func(s Snapshot, at int64) string {
		for _, line := range strings.Split(titles(s.Items(at)), "\n") {
			if strings.HasPrefix(line, "Next: ") {
				return line
			}
		}
		return ""
	}
	review := planSlot(t, "14:00", "14:50", "Review")
	if got := tail(slotSnap(review), now); got != "Next: 14:00 Review (in 12 min)" {
		t.Errorf("12 min ahead: %q", got)
	}
	if got := titles(slotSnap(review).Items(now)); !strings.Contains(got, "--\nNext: 14:00 Review (in 12 min)\n--\nStart 25 min focus [start:25]") {
		t.Errorf("menu:\n%s", got)
	}
	if got := tail(slotSnap(review), apptest.At(t, "13:48", 0)+30_000); !strings.HasSuffix(got, "(in 12 min)") {
		t.Errorf("11.5 min ahead rounds up: %q", got)
	}
	if got := tail(slotSnap(review), apptest.At(t, "13:59", 0)+30_000); !strings.HasSuffix(got, "(in 1 min)") {
		t.Errorf("30 s ahead: %q, want at least 1 min", got)
	}
	if got := tail(slotSnap(review), apptest.At(t, "14:10", 0)); !strings.HasSuffix(got, "Next: 14:00 Review (now)") {
		t.Errorf("running: %q", got)
	}
	if got := titles(slotSnap(review).Items(apptest.At(t, "14:50", 0))); strings.Contains(got, "Next:") {
		t.Errorf("ended slot still offered:\n%s", got)
	}
	far := planSlot(t, "14:49", "15:30", "Far")
	if got := titles(slotSnap(far).Items(now)); strings.Contains(got, "Next:") {
		t.Errorf("61 min ahead:\n%s", got)
	}
	if got := titles(slotSnap(planSlot(t, "14:47", "15:30", "Edge")).Items(now)); !strings.Contains(got, "Next: 14:47 Edge (in 59 min)") {
		t.Errorf("59 min ahead:\n%s", got)
	}
	long := planSlot(t, "14:00", "14:50", strings.Repeat("x", 40))
	if got := tail(slotSnap(long), now); !strings.Contains(got, "Next: 14:00 "+strings.Repeat("x", 23)+"… (in 12 min)") {
		t.Errorf("long title: %q", got)
	}
	// The earliest slot still to end wins, the link words of a title are dropped.
	words := planSlot(t, "14:00", "14:50", "Review https://x.example/y")
	later := planSlot(t, "14:30", "15:00", "Later")
	if got := tail(slotSnap(words, later), now); got != "Next: 14:00 Review (in 12 min)" {
		t.Errorf("two slots: %q", got)
	}
}

func TestSnapshotJoin(t *testing.T) {
	standup := eventSlot(t, "14:30", "15:00", "Standup", meetLink)
	id := IDJoin + strconv.FormatInt(standup.StartMS, 10)
	joinItem := func(s Snapshot, at int64) (Item, bool) {
		for _, it := range s.Items(at) {
			if it.ID != "" && strings.HasPrefix(it.ID, IDJoin) {
				return it, true
			}
		}
		return Item{}, false
	}
	it, ok := joinItem(slotSnap(standup), apptest.At(t, "14:21", 0))
	if !ok || it.Title != "Join Standup" || it.ID != id {
		t.Fatalf("14:21: %+v, %v", it, ok)
	}
	if _, ok := joinItem(slotSnap(standup), apptest.At(t, "14:19", 0)); ok {
		t.Error("11 min ahead: a Join item")
	}
	if _, ok := joinItem(slotSnap(standup), apptest.At(t, "14:45", 0)); !ok {
		t.Error("running: no Join item")
	}
	if _, ok := joinItem(slotSnap(standup), apptest.At(t, "15:00", 0)); ok {
		t.Error("ended: a Join item")
	}
	// Next row first, then Join.
	if got := titles(slotSnap(standup).Items(apptest.At(t, "14:21", 0))); !strings.Contains(got, "Next: 14:30 Standup (in 9 min)\nJoin Standup ["+id+"]") {
		t.Errorf("menu:\n%s", got)
	}
	bad := standup
	bad.JoinURL = "javascript:x"
	if _, ok := joinItem(slotSnap(bad), apptest.At(t, "14:21", 0)); ok {
		t.Error("tampered link: a Join item")
	}
	plan := standup
	plan.Kind = focus.SlotPlan
	if _, ok := joinItem(slotSnap(plan), apptest.At(t, "14:21", 0)); ok {
		t.Error("plan slot: a Join item")
	}
	long := standup
	long.Title = strings.Repeat("y", 40)
	if it, _ := joinItem(slotSnap(long), apptest.At(t, "14:21", 0)); it.Title != "Join "+strings.Repeat("y", 23)+"…" {
		t.Errorf("long title: %q", it.Title)
	}

	s, nowMS := slotSnap(standup), apptest.At(t, "14:21", 0)
	if got, ok := s.JoinURL(id, nowMS); !ok || got != meetLink {
		t.Errorf("JoinURL = %q, %v", got, ok)
	}
	for _, other := range []string{"join:1", IDJoin, "stop", "", IDJoin + strconv.FormatInt(standup.StartMS+1, 10)} {
		if got, ok := s.JoinURL(other, nowMS); ok || got != "" {
			t.Errorf("JoinURL(%q) = %q, %v, want nothing", other, got, ok)
		}
	}
	if got, ok := s.JoinURL(id, apptest.At(t, "15:05", 0)); ok || got != "" {
		t.Errorf("after the event ended: %q, %v", got, ok)
	}
	if got, ok := slotSnap(bad).JoinURL(id, nowMS); ok || got != "" {
		t.Errorf("tampered link: %q, %v", got, ok)
	}
}

func TestLoadSlots(t *testing.T) {
	e, a := seeded(t)
	todo := filepath.Join(t.TempDir(), "todo.txt")
	if err := os.WriteFile(todo, []byte("Review @14h00 due:2026-10-10\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	e.WriteConfig(t, apptest.NoRoutines+"[plan]\nfile = '"+todo+"'\n")
	e.Clock = time.Date(2026, 10, 10, 13, 48, 0, 0, apptest.Zone)
	s, err := Load(a, e.Clock, app.NotifyGranted)
	if err != nil || s.SlotsErr != nil {
		t.Fatalf("Load: %v, slots err %v", err, s.SlotsErr)
	}
	if len(s.Slots) != 1 || s.Slots[0].Title != "Review" || s.Loc == nil || s.Loc.String() != apptest.Zone.String() {
		t.Fatalf("slots = %+v, loc %v", s.Slots, s.Loc)
	}
	if got := titles(s.Items(e.Clock.UnixMilli())); !strings.Contains(got, "Next: 14:00 Review (in 12 min)") {
		t.Errorf("menu:\n%s", got)
	}

	// A slot read failure goes to SlotsErr and the rest is good.
	if _, err := e.RawDB(t).Exec(`ALTER TABLE event DROP COLUMN join_url`); err != nil {
		t.Fatal(err)
	}
	e.Event(t, "16:00", "16:30", "New") // a new row changes the stamp, so the slots are read again (a minute on)
	e.Clock = e.Clock.Add(time.Minute)
	s, err = Load(a, e.Clock, app.NotifyGranted)
	if err != nil || s.SlotsErr == nil || s.Slots != nil {
		t.Fatalf("with an event table the slots cannot read: err %v, slots err %v, slots %+v", err, s.SlotsErr, s.Slots)
	}
	if !s.OK || s.TrackedMS == 0 {
		t.Errorf("snapshot not good: %+v", s)
	}
}

// TestRespondBreakScreen: Back to work changes nothing; 5 more minutes makes
// the break end 5 min from now, keeping its start. The notice carries the
// break so the screen can count it.
func TestRespondBreakScreen(t *testing.T) {
	e, a := seeded(t)
	now := e.Clock.UnixMilli()
	if _, err := a.TakeBreak(10 * 60_000); err != nil {
		t.Fatal(err)
	}
	ev := focus.Event{Kind: focus.Notify, Topic: focus.TopicBreakScreen, Title: focus.TitleBreakScreen, Body: "b",
		Break: classify.Interval{From: now - 15*60_000, To: now - 5*60_000}}
	id := enqueue(t, a, ev)
	n, d, err := a.Notice(id)
	if err != nil || d.From != ev.Break.From || d.To != ev.Break.To {
		t.Fatalf("notice data = %+v, %v", d, err)
	}
	if got := Buttons(n, d); len(got) != 2 || got[0].ID != ActBack || got[1].Title != "5 more minutes" {
		t.Fatalf("buttons = %+v", got)
	}
	if msg, err := Respond(a, id, ActBack); err != nil || msg != "back to work" {
		t.Fatalf("back = %q, %v", msg, err)
	}
	if msg, err := Respond(a, id, ActMore); err != nil || msg != "break 5 min longer" {
		t.Fatalf("more = %q, %v", msg, err)
	}
	if until, err := a.BreakUntil(e.Clock); err != nil || until != now+5*60_000 {
		t.Fatalf("break until = %d, %v, want %d", until, err, now+5*60_000)
	}
}

// TestRespondWake: the wake screen offers Back to work, This is work (or
// I'm on paper when idle), Snooze and End session; This is work learns a
// rule, I'm on paper marks the session.
func TestRespondWake(t *testing.T) {
	e, a := seeded(t)
	s, err := a.StartSession(50)
	if err != nil {
		t.Fatal(err)
	}
	ev := focus.Event{Kind: focus.Notify, Topic: focus.TopicWake, Title: focus.TitleWake, Body: "b",
		OffFrom: e.Clock.UnixMilli() - 10*60_000, Key: "Arc · youtube.com", Target: classify.Want{Category: "Study"}, Sess: s}
	id := enqueue(t, a, ev)
	n, d, err := a.Notice(id)
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, b := range Buttons(n, d) {
		got = append(got, b.ID+"="+b.Title)
	}
	want := []string{"back=Back to work", "wake:work=This is work → Study", "snooze:distract=Snooze 10 min", "stop=End session"}
	if !slices.Equal(got, want) {
		t.Fatalf("buttons = %q", got)
	}
	if msg, err := Respond(a, id, ActWakeWork); err != nil || !strings.Contains(msg, "Study") {
		t.Fatalf("work = %q, %v", msg, err)
	}
	if us, _ := a.UndoList(1); len(us) != 1 || !strings.HasPrefix(us[0].What, "learn") {
		t.Fatalf("undo list = %+v", us)
	}

	ev.Idle, ev.Key, ev.Target = true, "", classify.Want{}
	id = enqueue(t, a, ev)
	n, d, _ = a.Notice(id)
	if b := Buttons(n, d); len(b) != 4 || b[1].ID != ActPaper || b[1].Title != "I'm on paper" {
		t.Fatalf("idle buttons = %+v", b)
	}
	if msg, err := Respond(a, id, ActPaper); err != nil || !strings.Contains(msg, "paper") {
		t.Fatalf("paper = %q, %v", msg, err)
	}

	warn := ev
	warn.Topic, warn.Title, warn.Idle, warn.Key, warn.Target = focus.TopicWakeWarn, focus.TitleWakeWarn, false, "Arc · youtube.com", classify.Want{Category: "Study"}
	id = enqueue(t, a, warn)
	n, d, _ = a.Notice(id)
	if b := Buttons(n, d); len(b) != 3 || b[0].ID != ActWakeWork || b[2].ID != ActStop {
		t.Fatalf("warn buttons = %+v", b)
	}
}

func TestLoadMenuData(t *testing.T) {
	e, a, s := distractedSession(t)
	at(t, e, "15:00")
	snap, err := Load(a, e.Clock, app.NotifyGranted)
	if err != nil {
		t.Fatal(err)
	}
	if len(snap.Strip) != StripCells || snap.DistractAtMS != 0 {
		t.Fatalf("before notice: strip %d cells, distract %d", len(snap.Strip), snap.DistractAtMS)
	}
	notes := focusNotes(t, a)
	if len(notes) != 1 || notes[0].Sess.ID != s.ID {
		t.Fatalf("notes = %+v", notes)
	}
	enqueue(t, a, notes[0])
	if snap, err = Load(a, e.Clock, app.NotifyGranted); err != nil || snap.DistractAtMS < s.StartMS {
		t.Fatalf("after notice: distract %d, %v, want ≥ %d", snap.DistractAtMS, err, s.StartMS)
	}
	if len(snap.Features) != len(config.Features) || snap.Features[0].ID != config.Features[0].ID || snap.Features[0].Group == "" {
		t.Fatalf("features = %+v", snap.Features)
	}
}

func TestMenuSnoozeOnlyAfterNotice(t *testing.T) {
	now := int64(10 * 60 * 60_000)
	sess := &focus.Session{ID: 1, StartMS: now - 20*60_000, PlannedMS: now + 5*60_000}
	base := Snapshot{OK: true, TrackdUp: true, Session: sess, SnoozeMin: 10, Live: focus.Live{Level: -1}}
	if got := titles(base.Items(now)); strings.Contains(got, "Snooze") {
		t.Fatalf("no notice yet:\n%s", got)
	}
	recent := base
	recent.DistractAtMS = now - 14*60_000
	if got := titles(recent.Items(now)); !strings.Contains(got, "Snooze distraction notices (10 min) [snooze:distract]") {
		t.Fatalf("recent notice:\n%s", got)
	}
	old := base
	old.DistractAtMS = now - 16*60_000
	if got := titles(old.Items(now)); strings.Contains(got, "Snooze") {
		t.Fatalf("old notice:\n%s", got)
	}
	snoozed := recent
	snoozed.SnoozeUntil = time.UnixMilli(now + 5*60_000)
	got := titles(snoozed.Items(now))
	if !strings.Contains(got, "Resume notices [snooze:off]") || strings.Contains(got, "snooze:distract") {
		t.Fatalf("snoozed:\n%s", got)
	}
}

func TestMenuOnePreset(t *testing.T) {
	s := Snapshot{OK: true, TrackdUp: true, Presets: []int{25}}
	got := titles(s.Items(0))
	if !strings.Contains(got, "Start 25 min focus [start:25]") || strings.Contains(got, "More lengths") {
		t.Fatalf("one preset:\n%s", got)
	}
}

func TestMenuCalmTopLevel(t *testing.T) {
	s := Snapshot{OK: true, TrackdUp: true, Active: true, TrackedMS: 3600_000, Presets: []int{25, 50},
		Strip: make([]string, StripCells), UnlabelledMS: 60_000}
	rows := 0
	for _, it := range s.Items(0) {
		if !it.Separator {
			rows++
		}
	}
	if rows > 7 { // header, strip, start, more lengths, inbox, dashboard, settings
		t.Fatalf("%d top-level rows:\n%s", rows, titles(s.Items(0)))
	}
}

func TestLastSessionVisibility(t *testing.T) {
	now := int64(48 * 60 * 60_000)
	mk := func(endAgoMS int64, rating int, refile, undo bool) Snapshot {
		var d report.SessionDetail
		d.Sess.ID, d.Sess.StartMS, d.Sess.EndMS, d.Sess.Rating = 7, now-endAgoMS-25*60_000, now-endAgoMS, rating
		return Snapshot{OK: true, TrackdUp: true, Last: &LastSession{Detail: d, Refile: refile, Undo: undo, Loc: time.UTC}}
	}
	for _, c := range []struct {
		name string
		s    Snapshot
		want bool
	}{
		{"unrated", mk(5*3600_000, 0, false, false), true},
		{"rated fresh", mk(30*60_000, 4, false, false), true},
		{"rated settled", mk(2*3600_000, 4, false, false), false},
		{"rated with refile", mk(2*3600_000, 4, true, false), true},
		{"rated with undo", mk(2*3600_000, 4, false, true), true},
	} {
		if got := strings.Contains(titles(c.s.Items(now)), "Last session"); got != c.want {
			t.Errorf("%s: shown %v, want %v", c.name, got, c.want)
		}
	}
}

func TestMenuFeature(t *testing.T) {
	e, a := seeded(t)
	if a.Config().Focus.BreakScreen {
		t.Fatal("break_screen is on before the click")
	}
	if msg, err := Menu(a, IDFeature+"break_screen"); err != nil || msg != "Break screen on" {
		t.Fatalf("toggle = %q, %v", msg, err)
	}
	if !a.Config().Focus.BreakScreen {
		t.Fatal("break_screen not picked up")
	}
	s, err := Load(a, e.Clock, app.NotifyGranted)
	if err != nil || !strings.Contains(titles(s.Items(e.Clock.UnixMilli())), "✓ Break screen [feature:break_screen]") {
		t.Fatalf("menu after toggle:\n%s\n%v", titles(s.Items(e.Clock.UnixMilli())), err)
	}
	if msg, err := Menu(a, IDFeature+"break_screen"); err != nil || msg != "Break screen off" {
		t.Fatalf("second toggle = %q, %v", msg, err)
	}
	if _, err := os.Stat(e.Config + ".bak"); err != nil {
		t.Fatalf("no backup: %v", err)
	}
	if msg, err := Menu(a, IDFeature+"ocr"); err != nil || !strings.HasPrefix(msg, "Screen text on · ") {
		t.Fatalf("ocr = %q, %v", msg, err)
	}
	if _, err := Menu(a, IDFeature+"nope"); !errors.Is(err, ErrUnknown) {
		t.Fatalf("unknown = %v", err)
	}
}

func TestPrivacyItems(t *testing.T) {
	url := func(s string) *string { return &s }
	for _, tc := range []struct {
		name string
		p    PrivacyState
		want string
	}{
		{"nothing", PrivacyState{}, "Privacy ▸\n  Nothing to hide yet"},
		{"own app", PrivacyState{Front: &store.Active{Bundle: "com.tock.bar", App: "tock"}}, "Privacy ▸\n  Nothing to hide yet"},
		{"app and site", PrivacyState{Front: &store.Active{Bundle: "com.apple.Safari", App: "Safari", URL: url("www.bank.com/acct")}},
			"Privacy ▸\n  Hide Safari [hide:app:com.apple.Safari]\n  Hide bank.com [hide:site:bank.com]"},
		{"hidden", PrivacyState{
			Front: &store.Active{Bundle: "com.apple.Safari", App: "Safari", URL: url("bank.com/")},
			Apps:  []HiddenApp{{Bundle: "com.example.diary", Name: "Diary"}}, Sites: []string{"bank.com"}},
			"Privacy ▸\n  Hide Safari [hide:app:com.apple.Safari]\n  --\n  ✓ Diary [unhide:app:com.example.diary]\n  ✓ bank.com [unhide:site:bank.com]"},
		{"front app hidden", PrivacyState{
			Front: &store.Active{Bundle: "com.example.diary", App: "Diary"},
			Apps:  []HiddenApp{{Bundle: "com.example.diary", Name: "Diary"}}},
			"Privacy ▸\n  ✓ Diary [unhide:app:com.example.diary]"},
	} {
		if got := titles([]Item{Snapshot{Privacy: tc.p}.privacy()}); got != tc.want {
			t.Errorf("%s:\ngot\n%s\nwant\n%s", tc.name, got, tc.want)
		}
	}
}

func TestMenuHide(t *testing.T) {
	e, a := seeded(t)
	s, err := Load(a, e.Clock, app.NotifyGranted)
	if err != nil || s.PrivacyErr != nil {
		t.Fatal(err, s.PrivacyErr)
	}
	if s.Privacy.Front == nil || s.Privacy.Front.Bundle != "mail" {
		t.Fatalf("front = %+v", s.Privacy.Front)
	}
	menu := titles(s.Items(e.Clock.UnixMilli()))
	if !strings.Contains(menu, "    Hide Mail [hide:app:mail]") {
		t.Fatalf("menu offers no hide:\n%s", menu)
	}
	if msg, err := Menu(a, IDHide+"app:mail"); err != nil || msg != "Mail hidden" {
		t.Fatalf("hide = %q, %v", msg, err)
	}
	if got := a.Config().Privacy.HideApps; len(got) != 1 || got[0] != "mail" {
		t.Fatalf("hide_apps = %v", got)
	}
	s, _ = Load(a, e.Clock, app.NotifyGranted)
	menu = titles(s.Items(e.Clock.UnixMilli()))
	if !strings.Contains(menu, "✓ Mail [unhide:app:mail]") || strings.Contains(menu, "Hide Mail") {
		t.Fatalf("menu after hide:\n%s", menu)
	}
	if msg, err := Menu(a, IDHide+"site:chatgpt.com"); err != nil || msg != "chatgpt.com hidden" {
		t.Fatalf("hide site = %q, %v", msg, err)
	}
	if msg, err := Menu(a, IDUnhide+"app:mail"); err != nil || msg != "Mail shown again" {
		t.Fatalf("unhide = %q, %v", msg, err)
	}
	if c := a.Config().Privacy; len(c.HideApps) != 0 || len(c.HideSites) != 1 {
		t.Fatalf("privacy = %+v", c)
	}
	for _, id := range []string{IDHide + "window:x", IDHide + "app:", IDUnhide + "site"} {
		if _, err := Menu(a, id); !errors.Is(err, ErrUnknown) {
			t.Errorf("%s: err = %v, want ErrUnknown", id, err)
		}
	}
}

func TestSessionRowGoal(t *testing.T) {
	e, a := seeded(t)
	if _, err := a.StartSessionGoal(50, "TD 3"); err != nil {
		t.Fatal(err)
	}
	s, err := Load(a, e.Clock, app.NotifyGranted)
	if err != nil {
		t.Fatal(err)
	}
	if got := titles(s.Items(e.Clock.UnixMilli())); !strings.Contains(got, "\nTD 3 · 50:00 left · ") {
		t.Fatalf("menu:\n%s", got)
	}
}

func TestMenuFocusGoal(t *testing.T) {
	e, a := seeded(t)
	if msg, err := Menu(a, IDFocus+"50:TD 3: exercises"); err != nil || msg != "started 50 min session · TD 3: exercises" {
		t.Fatalf("focus = %q, %v", msg, err)
	}
	f, _ := a.Focus(e.Clock)
	if f.Open == nil || f.Open.Goal != "TD 3: exercises" || f.Open.PlannedMS-f.Open.StartMS != 50*60_000 {
		t.Fatalf("open = %+v", f.Open)
	}
	if _, err := a.StopSession(); err != nil {
		t.Fatal(err)
	}
	if msg, err := Menu(a, IDFocus+"0:"); err != nil || msg != "started 25 min session" {
		t.Fatalf("default length = %q, %v", msg, err)
	}
	for _, id := range []string{IDFocus + "x:goal", IDFocus + "50"} {
		if _, err := Menu(a, id); !errors.Is(err, ErrUnknown) {
			t.Errorf("%s: %v, want ErrUnknown", id, err)
		}
	}
}

func TestPrompt(t *testing.T) {
	s := Snapshot{OK: true, Presets: []int{25, 50, 90}}
	if p := s.Prompt(0); p.Stop || p.Title != "What are you working on?" || !slices.Equal(p.Presets, []int{25, 50, 90}) {
		t.Fatalf("idle prompt = %+v", p)
	}
	s.Session = &focus.Session{StartMS: 0, PlannedMS: 50 * 60_000, Goal: "TD 3"}
	if p := s.Prompt(16 * 60_000); !p.Stop || p.Title != "Stop TD 3?" || p.Detail != "34:00 left" {
		t.Fatalf("running prompt = %+v", p)
	}
	s.Session.Goal = ""
	if p := s.Prompt(16 * 60_000); p.Title != "Stop the session?" {
		t.Fatalf("running prompt, no goal = %+v", p)
	}
	if p := (Snapshot{OK: true}).Prompt(0); !slices.Equal(p.Presets, []int{25}) {
		t.Fatalf("no presets = %+v", p)
	}
}

func TestURLAction(t *testing.T) {
	for raw, want := range map[string]string{
		"tock://start?min=50&goal=TD%203": "focus:50:TD 3",
		"tock://start":                    "focus:0:",
		"tock://start?goal=read":          "focus:0:read",
		"tock://stop":                     "stop",
		"tock://prompt":                   "hotkey",
		"TOCK://Stop":                     "stop",
	} {
		if got, ok := URLAction(raw); !ok || got != want {
			t.Errorf("URLAction(%q) = %q, %v; want %q", raw, got, ok, want)
		}
	}
	for _, raw := range []string{"tock://start?min=abc", "http://stop", "tock://quit", "::", "tock://start?min=999"} {
		if got, ok := URLAction(raw); ok {
			t.Errorf("URLAction(%q) = %q, want refused", raw, got)
		}
	}
}
