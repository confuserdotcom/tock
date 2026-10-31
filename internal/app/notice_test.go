package app

import (
	"encoding/json"
	"reflect"
	"testing"
	"time"

	"tock/internal/apptest"
	"tock/internal/classify"
	"tock/internal/focus"
)

func TestEnqueueAndTake(t *testing.T) {
	e := apptest.New(t, apptest.Rules)
	e.Seed(t, apptest.Span{ID: 1, Start: "09:00", End: "09:10", Bundle: "term", App: "WezTerm", Title: "x"})
	a := open(t, e)
	if err := a.Answer(classify.Interval{From: apptest.At(t, "10:00", 0), To: apptest.At(t, "10:30", 0)},
		classify.Want{Category: "Communication"}); err != nil {
		t.Fatal(err)
	}
	away := classify.Interval{From: apptest.At(t, "12:00", 0), To: apptest.At(t, "12:40", 0)}
	for _, ev := range []focus.Event{
		{Kind: focus.Notify, Topic: focus.TopicDone, Title: focus.TitleDone, Body: "done", Minutes: 50},
		{Kind: focus.Notify, Topic: focus.TopicAway, Title: focus.TitleAway, Body: "away", Away: away},
	} {
		if err := a.Enqueue(ev); err != nil {
			t.Fatal(err)
		}
	}
	ns, err := a.TakeNotices()
	if err != nil || len(ns) != 2 || ns[0].Topic != focus.TopicDone || ns[1].Body != "away" {
		t.Fatalf("TakeNotices = %+v, %v", ns, err)
	}
	if _, d, err := a.Notice(ns[0].ID); err != nil || d.Minutes != 50 {
		t.Fatalf("done data = %+v, %v", d, err)
	}
	_, d, err := a.Notice(ns[1].ID)
	want := NoticeData{From: away.From, To: away.To, Labels: []classify.Want{{Category: "Communication"}}}
	if err != nil || !reflect.DeepEqual(d, want) {
		t.Fatalf("away data = %+v, %v; want %+v", d, err, want)
	}
	if ns, _ := a.TakeNotices(); len(ns) != 0 {
		t.Fatalf("taken twice: %+v", ns)
	}

	// What the absence follows leads the buttons, once.
	for _, b := range []classify.Want{{Category: "Code", Project: "tock"}, {Category: "Communication"}} {
		if err := a.Enqueue(focus.Event{Kind: focus.Notify, Topic: focus.TopicAway, Title: focus.TitleAway, Away: away, Before: b}); err != nil {
			t.Fatal(err)
		}
		ns, _ := a.TakeNotices()
		_, d, _ := a.Notice(ns[0].ID)
		want := []classify.Want{b}
		if b.Category == "Code" {
			want = append(want, classify.Want{Category: "Communication"})
		}
		if !reflect.DeepEqual(d.Labels, want) {
			t.Fatalf("labels after %v = %+v", b, d.Labels)
		}
	}
}

func TestBarReady(t *testing.T) {
	e := apptest.New(t, apptest.Rules)
	e.Seed(t, apptest.Span{ID: 1, Start: "09:00", End: "09:10", Bundle: "term", App: "WezTerm", Title: "x"})
	a := open(t, e)
	if a.BarReady() {
		t.Fatal("ready with no heartbeat")
	}
	if err := a.BarBeat(NotifyDenied); err != nil {
		t.Fatal(err)
	}
	if running, access, _ := a.BarState(); !running || access != NotifyDenied || a.BarReady() {
		t.Fatalf("denied: running %v access %q ready %v", running, access, a.BarReady())
	}
	if err := a.BarBeat(NotifyGranted); err != nil {
		t.Fatal(err)
	}
	if !a.BarReady() {
		t.Fatal("fresh and granted must be ready")
	}
	e.Clock = e.Clock.Add(BarFreshMS*time.Millisecond + time.Second)
	if running, _, _ := a.BarState(); running || a.BarReady() {
		t.Fatal("a stale heartbeat must not be ready")
	}
}

func TestNoticeDoneData(t *testing.T) {
	e := apptest.New(t, apptest.Rules)
	e.Seed(t, apptest.Span{ID: 1, Start: "09:00", End: "09:10", Bundle: "term", App: "WezTerm", Title: "x"})
	a := open(t, e)
	ev := focus.Event{Kind: focus.Notify, Topic: focus.TopicDone, Title: focus.TitleDone, Body: "done", Minutes: 50,
		Sess: focus.Session{ID: 42}, Project: "Analyse 1", Refile: true}
	if err := a.Enqueue(ev); err != nil {
		t.Fatal(err)
	}
	ns, err := a.TakeNotices()
	if err != nil || len(ns) != 1 {
		t.Fatalf("TakeNotices = %+v, %v", ns, err)
	}
	_, d, err := a.Notice(ns[0].ID)
	want := NoticeData{Minutes: 50, Session: 42, Project: "Analyse 1", Refile: true}
	if err != nil || !reflect.DeepEqual(d, want) {
		t.Fatalf("data = %+v, %v; want %+v", d, err, want)
	}

	// A notice queued by an older trackd has none of the new keys.
	var old NoticeData
	if err := json.Unmarshal([]byte(`{"minutes":25}`), &old); err != nil || !reflect.DeepEqual(old, NoticeData{Minutes: 25}) {
		t.Fatalf("old data = %+v, %v", old, err)
	}
}

func TestNoticeFocusData(t *testing.T) {
	e := distracted(t)
	a := open(t, e)
	r, _ := a.Rules().RuleByRef("rules.toml#3")
	take := func(ev focus.Event) NoticeData {
		t.Helper()
		if err := a.Enqueue(ev); err != nil {
			t.Fatal(err)
		}
		ns, err := a.TakeNotices()
		if err != nil || len(ns) != 1 {
			t.Fatalf("TakeNotices = %+v, %v", ns, err)
		}
		_, d, err := a.Notice(ns[0].ID)
		if err != nil {
			t.Fatal(err)
		}
		return d
	}
	ev := focus.Event{Kind: focus.Notify, Topic: focus.TopicFocus, Title: focus.TitleFocus, Body: "x",
		Sess: focus.Session{ID: 5}, Category: "Entertainment", RuleRef: "rules.toml#3"}
	want := NoticeData{Session: 5, SnoozeMin: 10, Category: "Entertainment", Rule: "rules.toml#3", RuleFP: r.Fingerprint()}
	if d := take(ev); !reflect.DeepEqual(d, want) {
		t.Fatalf("data = %+v, want %+v", d, want)
	}
	// A ref that does not resolve, or whose rule sets another category now.
	ev.RuleRef = "rules.toml#9"
	want = NoticeData{Session: 5, SnoozeMin: 10, Category: "Entertainment"}
	if d := take(ev); !reflect.DeepEqual(d, want) {
		t.Fatalf("unresolved ref: data = %+v, want %+v", d, want)
	}
	ev.RuleRef = "rules.toml#2" // Code
	if d := take(ev); !reflect.DeepEqual(d, want) {
		t.Fatalf("other category: data = %+v, want %+v", d, want)
	}
	// Notices queued before this release have none of the keys.
	var old NoticeData
	if err := json.Unmarshal([]byte(`{"session":5,"snooze_min":10}`), &old); err != nil ||
		old.Category != "" || old.Rule != "" || old.RuleFP != "" {
		t.Fatalf("old data = %+v, %v", old, err)
	}
}
