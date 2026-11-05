package main

import (
	"encoding/json"
	"strconv"
	"strings"
	"testing"

	"tock/internal/bar"
	"tock/internal/focus"
)

// island parses the newest island the fake UI drew.
func island(t *testing.T, u *fakeUI) bar.Island {
	t.Helper()
	var i bar.Island
	if err := json.Unmarshal([]byte(last(u.islands)), &i); err != nil {
		t.Fatalf("island %q: %v", last(u.islands), err)
	}
	return i
}

func TestNotchOnlyAfterThePanelIsReady(t *testing.T) {
	e := seededEnv(t)
	r, u, _ := testRunner(t, e)
	r.handle(event{kind: evReady})
	if len(u.islands) != 0 {
		t.Fatalf("drew a notch with no panel: %q", u.islands)
	}
	r.handle(event{kind: evNotch, id: "1"})
	if i := island(t, u); i.State != bar.IslandQuiet || i.Hover == nil {
		t.Fatalf("quiet = %+v", i)
	}
	if len(u.options) != 1 || u.options[0] != "sharing=false all=true hover=false" {
		t.Fatalf("options = %v, want hidden from recordings, every display, click to open", u.options)
	}
	n := len(u.islands)
	r.tick()
	if len(u.islands) != n {
		t.Fatal("an unchanged island must not be redrawn")
	}
}

func TestNotchTakesDoneInsteadOfABanner(t *testing.T) {
	e := seededEnv(t)
	r, u, _ := testRunner(t, e)
	r.handle(event{kind: evReady})
	r.handle(event{kind: evNotch, id: "1"})
	if err := r.a.Enqueue(focus.Event{Topic: focus.TopicDone, Title: focus.TitleDone, Body: "25 min: done.", Minutes: 25}); err != nil {
		t.Fatal(err)
	}
	r.tick()
	if len(u.posts) != 0 {
		t.Fatalf("posted a banner too: %q", u.posts)
	}
	i := island(t, u)
	if i.State != bar.IslandDone || !i.Open || i.Haptic != bar.HapticLevel {
		t.Fatalf("done = %+v", i)
	}
	if again := i.Panel.Buttons[0]; again.ID != bar.IDNotchAct+strconv.FormatInt(i.Notice, 10)+":again" {
		t.Fatalf("first button = %+v", again)
	}
	r.tick()
	if i := island(t, u); i.Haptic != "" || i.State != bar.IslandDone {
		t.Fatalf("haptic played twice: %+v", i)
	}
	// Another 25 answers the notice and closes the moment
	r.handle(event{kind: evMenu, id: i.Panel.Buttons[0].ID})
	if i := island(t, u); i.State != bar.IslandFocus || i.Open || r.moment != nil {
		t.Fatalf("after Another = %+v", i)
	}
	if !strings.HasPrefix(last(u.titles), "◉ 25:00") {
		t.Fatalf("title = %q", last(u.titles))
	}
}

func TestNotchOffKeepsBanners(t *testing.T) {
	e := seededEnv(t)
	e.WriteConfig(t, "[notch]\nenabled = false\n")
	r, u, _ := testRunner(t, e)
	r.handle(event{kind: evReady})
	r.handle(event{kind: evNotch, id: "1"})
	if err := r.a.Enqueue(focus.Event{Topic: focus.TopicDone, Title: focus.TitleDone, Body: "done", Minutes: 25}); err != nil {
		t.Fatal(err)
	}
	r.tick()
	if len(u.posts) != 1 || last(u.islands) != "" {
		t.Fatalf("posts %q islands %q", u.posts, u.islands)
	}
}

func TestNotchHigherMomentWins(t *testing.T) {
	e := seededEnv(t)
	r, u, _ := testRunner(t, e)
	r.handle(event{kind: evReady})
	r.handle(event{kind: evNotch, id: "1"})
	if err := r.a.Enqueue(focus.Event{Topic: focus.TopicDone, Title: focus.TitleDone, Body: "done", Minutes: 25}); err != nil {
		t.Fatal(err)
	}
	r.tick()
	// a recap ranks below the open session end: it goes to a banner
	if err := r.a.Enqueue(focus.Event{Topic: focus.TopicRecap, Title: "Recap", Body: "the day"}); err != nil {
		t.Fatal(err)
	}
	r.tick()
	if len(u.posts) != 1 || !strings.HasPrefix(u.posts[0], "Recap|") || island(t, u).State != bar.IslandDone {
		t.Fatalf("posts %q island %s", u.posts, island(t, u).State)
	}
	r.handle(event{kind: evMenu, id: bar.IDNotchClose})
	if i := island(t, u); i.Open {
		t.Fatalf("close left %+v", i)
	}
}

func TestNotchLabelling(t *testing.T) {
	e := seededEnv(t)
	r, u, _ := testRunner(t, e)
	r.handle(event{kind: evReady})
	r.handle(event{kind: evNotch, id: "1"})
	r.handle(event{kind: evMenu, id: bar.IDNotchLabel})
	i := island(t, u)
	if i.State != bar.IslandLabel || len(i.Panel.Entries) == 0 || i.Panel.At != 0 {
		t.Fatalf("label = %+v", i)
	}
	// a stale click (entry 1 before entry 0) does nothing
	r.handle(event{kind: evMenu, id: bar.IDNotchPick + "1:0"})
	if island(t, u).Panel.At != 0 {
		t.Fatal("stale pick moved on")
	}
	r.handle(event{kind: evMenu, id: bar.IDNotchPick + "0:0"})
	i = island(t, u)
	if i.Panel.At != 1 || i.Haptic != bar.HapticAlignment || i.Panel.Entries[0].Saved == "" || i.Panel.Entries[0].Picked != 0 {
		t.Fatalf("after pick = %+v", i.Panel)
	}
	for k := 1; k < len(i.Panel.Entries); k++ {
		r.handle(event{kind: evMenu, id: bar.IDNotchSkip + strconv.Itoa(k)})
	}
	if i := island(t, u); i.Face.Right != "clear" {
		t.Fatalf("end = %+v", i.Face)
	}
	r.handle(event{kind: evMenu, id: bar.IDNotchClose})
	if island(t, u).State != bar.IslandQuiet {
		t.Fatal("close did not go back to quiet")
	}
}

func TestNotchDriftExpires(t *testing.T) {
	e := seededEnv(t)
	r, u, _ := testRunner(t, e)
	r.handle(event{kind: evReady})
	r.handle(event{kind: evNotch, id: "1"})
	if err := r.a.Enqueue(focus.Event{Topic: focus.TopicFocus, Title: "Back to work?", Body: "YouTube"}); err != nil {
		t.Fatal(err)
	}
	r.tick()
	i := island(t, u)
	if i.State != bar.IslandDrift || i.AutoClose == 0 {
		t.Fatalf("drift = %+v", i)
	}
	r.handle(event{kind: evMenu, id: bar.IDNotchExpire + "999999"})
	if r.moment == nil {
		t.Fatal("expire of another notice closed this one")
	}
	r.handle(event{kind: evMenu, id: bar.IDNotchExpire + strconv.FormatInt(i.Notice, 10)})
	if r.moment != nil {
		t.Fatal("expire left the moment open")
	}
}

func TestNotchMissedMomentWaits(t *testing.T) {
	e := seededEnv(t)
	r, u, _ := testRunner(t, e)
	r.handle(event{kind: evReady})
	r.handle(event{kind: evNotch, id: "1"})
	if err := r.a.Enqueue(focus.Event{Topic: focus.TopicDone, Title: focus.TitleDone, Body: "25 min: done.", Minutes: 25}); err != nil {
		t.Fatal(err)
	}
	r.tick()
	done := island(t, u)
	r.handle(event{kind: evMenu, id: bar.IDNotchExpire + strconv.FormatInt(done.Notice, 10)})
	q := island(t, u)
	if q.State != bar.IslandQuiet || !q.Pending || q.Hover == nil || q.Hover.Lead != done.Panel.Lead {
		t.Fatalf("an unseen session end must wait in the notch: %+v", q)
	}
	r.handle(event{kind: evMenu, id: bar.IDNotchSeen})
	if island(t, u).Pending {
		t.Fatal("seen left the dot")
	}
	// a moment put away by hand was seen: nothing waits
	if err := r.a.Enqueue(focus.Event{Topic: focus.TopicDone, Title: focus.TitleDone, Body: "25 min: done.", Minutes: 25}); err != nil {
		t.Fatal(err)
	}
	r.tick()
	r.handle(event{kind: evMenu, id: bar.IDNotchClose})
	if island(t, u).Pending {
		t.Fatal("a closed moment must not wait")
	}
}
