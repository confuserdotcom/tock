package app

import (
	"errors"
	"os"
	"strings"
	"testing"

	"tock/internal/apptest"
	"tock/internal/classify"
)

func rowAt(t *testing.T, a *App, hhmm string) (string, classify.Label, []int64) {
	t.Helper()
	d, err := a.Day(apptest.Now)
	if err != nil {
		t.Fatal(err)
	}
	ms := apptest.At(t, hhmm, 0)
	for _, r := range d.Timeline {
		if r.Start <= ms && ms < r.End {
			return r.App, r.Label, r.IDs
		}
	}
	return "", classify.Label{}, nil
}

func TestEditsChangeTheDay(t *testing.T) {
	e := apptest.New(t, apptest.Rules)
	e.Seed(t, apptest.DaySpans()...)
	a := open(t, e)
	before, _ := a.Day(apptest.Now)

	// Add a meeting over the idle hour: it labels the time and answers the absence.
	if _, err := a.AddEdit(classify.EditAdd, apptest.At(t, "11:41", 0), apptest.At(t, "12:22", 0),
		classify.Want{Category: "Communication"}, "standup"); err != nil {
		t.Fatal(err)
	}
	app, l, _ := rowAt(t, a, "12:00")
	if app != "Entry" || l.Category != "Communication" || l.CategorySource != classify.SourceManual {
		t.Fatalf("entry row = %s %+v", app, l)
	}
	d, _ := a.Day(apptest.Now)
	if d.TrackedMS != before.TrackedMS+41*60_000 || len(d.Away) != len(before.Away)-1 {
		t.Fatalf("tracked %d (was %d), away %v", d.TrackedMS, before.TrackedMS, d.Away)
	}

	// Label the first half of the YouTube span as Study: a split.
	if _, err := a.AddEdit(classify.EditLabel, apptest.At(t, "11:14", 0), apptest.At(t, "11:20", 0),
		classify.Want{Category: "Study"}, ""); err != nil {
		t.Fatal(err)
	}
	if app, l, _ := rowAt(t, a, "11:15"); app != "Arc" || l.Category != "Study" {
		t.Fatalf("labelled half = %s %+v", app, l)
	}
	if _, l, _ := rowAt(t, a, "11:22"); l.Category != "Entertainment" {
		t.Fatalf("other half = %+v", l)
	}

	// Delete 09:00–09:30.
	if _, err := a.AddEdit(classify.EditCut, apptest.At(t, "09:00", 0), apptest.At(t, "09:30", 0), classify.Want{}, ""); err != nil {
		t.Fatal(err)
	}
	if app, _, _ := rowAt(t, a, "09:10"); app != "" {
		t.Fatalf("cut time still shows %s", app)
	}

	// t on an entry relabels the edit; undo puts it back.
	_, _, ids := rowAt(t, a, "12:00")
	if err := a.Tag(ids, classify.Want{Category: "Code", Project: "ATLAS"}); err != nil {
		t.Fatal(err)
	}
	if _, l, _ := rowAt(t, a, "12:00"); l.Category != "Code" || l.Project != "ATLAS" {
		t.Fatalf("retagged entry = %+v", l)
	}
	for _, want := range []string{"tag 1 span → Code / ATLAS", "delete Sat 09:00–09:30", "label Sat 11:14–11:20 Study"} {
		what, err := a.Undo()
		if err != nil || what != want {
			t.Fatalf("Undo = %q, %v; want %q", what, err, want)
		}
	}
	if _, l, _ := rowAt(t, a, "12:00"); l.Category != "Communication" {
		t.Fatalf("after undoing the tag = %+v", l)
	}
	if app, _, _ := rowAt(t, a, "09:10"); app != "WezTerm" {
		t.Fatalf("cut not undone: %s", app)
	}
	if _, l, _ := rowAt(t, a, "11:15"); l.Category != "Entertainment" {
		t.Fatalf("label not undone: %+v", l)
	}
	es, _ := a.Edits(apptest.Now)
	if len(es) != 1 || es[0].Note != "standup" {
		t.Fatalf("edits = %+v", es)
	}
	if err := a.RemoveEdit(es[0].ID); err != nil {
		t.Fatal(err)
	}
	if app, _, _ := rowAt(t, a, "12:00"); app != "Idle" {
		t.Fatalf("removed entry still shows %s", app)
	}
	if what, err := a.Undo(); err != nil || !strings.HasPrefix(what, "remove edit: add") {
		t.Fatalf("undo remove = %q, %v", what, err)
	}
	if app, _, _ := rowAt(t, a, "12:00"); app != "Entry" {
		t.Fatalf("restored entry = %s", app)
	}
}

func TestAddEditRefuses(t *testing.T) {
	e := apptest.New(t, apptest.Rules)
	e.Seed(t, apptest.DaySpans()...)
	a := open(t, e)
	w := classify.Want{Category: "Code"}
	for _, c := range []struct {
		kind     string
		from, to string
		want     classify.Want
	}{
		{"x", "09:00", "10:00", w},
		{classify.EditAdd, "10:00", "09:00", w},
		{classify.EditAdd, "14:00", "16:00", w},
		{classify.EditAdd, "09:00", "10:00", classify.Want{}},
		{classify.EditCut, "09:00", "10:00", w},
	} {
		if _, err := a.AddEdit(c.kind, apptest.At(t, c.from, 0), apptest.At(t, c.to, 0), c.want, ""); err == nil {
			t.Errorf("AddEdit(%+v) accepted", c)
		}
	}
	if _, err := a.Undo(); !errors.Is(err, ErrNothingToUndo) {
		t.Fatalf("Undo on empty journal = %v", err)
	}
}

func TestUndoTagLearnIgnoreAnswer(t *testing.T) {
	e := apptest.New(t, apptest.Rules)
	e.Seed(t, apptest.DaySpans()...)
	a := open(t, e)
	if err := a.Tag([]int64{4}, classify.Want{Category: "Study"}); err != nil {
		t.Fatal(err)
	}
	if err := a.Tag([]int64{4}, classify.Want{Project: "ATLAS"}); err != nil {
		t.Fatal(err)
	}
	if _, err := a.Undo(); err != nil {
		t.Fatal(err)
	}
	ovs, _ := a.st.Overrides()
	if o := ovs[4]; o.Category != "Study" || o.Project != "" {
		t.Fatalf("after one undo = %+v", o)
	}
	a.Undo()
	if ovs, _ := a.st.Overrides(); len(ovs) != 0 {
		t.Fatalf("after two undos = %+v", ovs)
	}

	key := "Arc · chatgpt.com/c"
	cs, err := a.Propose(key, classify.Want{Category: "Study"})
	if err != nil || len(cs) == 0 {
		t.Fatalf("Propose = %v, %v", cs, err)
	}
	if err := a.Learn(key, cs[0], classify.Want{Category: "Study"}); err != nil {
		t.Fatal(err)
	}
	if len(a.Rules().Learned) != 1 {
		t.Fatal("not learned")
	}
	if what, err := a.Undo(); err != nil || !strings.HasPrefix(what, "learn ") {
		t.Fatalf("undo learn = %q, %v", what, err)
	}
	if _, err := os.Stat(e.Learned); !errors.Is(err, os.ErrNotExist) || len(a.Rules().Learned) != 0 {
		t.Fatalf("learned.toml after undo: %v, %d rules", err, len(a.Rules().Learned))
	}

	if err := a.Ignore("Mail · inbox", true); err != nil {
		t.Fatal(err)
	}
	a.Undo()
	if ig, _ := a.st.Ignored(); ig["Mail · inbox"] {
		t.Fatal("ignore not undone")
	}

	iv := classify.Interval{From: apptest.At(t, "11:41", 0), To: apptest.At(t, "12:22", 0)}
	if err := a.Answer(iv, classify.Want{Category: "Communication"}); err != nil {
		t.Fatal(err)
	}
	a.Undo()
	if d, _ := a.Day(apptest.Now); len(d.Away) != 2 || d.Away[0] != iv {
		t.Fatalf("answer not undone: away %v", d.Away)
	}
}

func TestUndoRefusesChangedFile(t *testing.T) {
	e := apptest.New(t, apptest.Rules)
	e.Seed(t, apptest.DaySpans()...)
	a := open(t, e)
	key := "Arc · chatgpt.com/c"
	cs, _ := a.Propose(key, classify.Want{Category: "Study"})
	if err := a.Learn(key, cs[0], classify.Want{Category: "Study"}); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(e.Learned, []byte("# edited by hand\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := a.Undo(); err == nil || !strings.Contains(err.Error(), "changed since") {
		t.Fatalf("Undo = %v", err)
	}
}
