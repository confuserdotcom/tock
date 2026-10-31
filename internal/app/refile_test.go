package app

import (
	"errors"
	"strings"
	"testing"

	"tock/internal/apptest"
	"tock/internal/classify"
	"tock/internal/store"
)

const refileRules = apptest.Rules + `
[[project]]
name = "Analyse 1"
category = "Study"
`

// refileEnv has active time 09:00-09:20 and 09:30-09:50 split by 10 idle
// minutes, and a finished 50 minute session over all of it.
func refileEnv(t *testing.T) (*apptest.Env, *App, store.Session) {
	t.Helper()
	e := apptest.New(t, refileRules)
	e.Seed(t,
		apptest.Span{ID: 1, Start: "09:00", End: "09:20", Bundle: "term", App: "WezTerm", Title: "tock │ a"},
		apptest.Span{ID: 2, Start: "09:20", End: "09:30", Bundle: "tock.idle", App: "Idle"},
		apptest.Span{ID: 3, Start: "09:30", End: "09:50", Bundle: "term", App: "WezTerm", Title: "tock │ b"},
	)
	a := open(t, e)
	return e, a, finishedAt(t, a, "09:00", 50, 40)
}

func TestRefileSession(t *testing.T) {
	_, a, x := refileEnv(t)
	before, err := a.Day(apptest.Now)
	if err != nil {
		t.Fatal(err)
	}
	s, name, err := a.RefileSession(x.ID, "analyse")
	if err != nil || name != "Analyse 1" || s.ID != x.ID {
		t.Fatalf("RefileSession = %+v, %q, %v", s, name, err)
	}
	es, err := a.Edits(apptest.Now)
	if err != nil || len(es) != 2 {
		t.Fatalf("edits = %+v, %v", es, err)
	}
	want := [][2]int64{{apptest.At(t, "09:00", 0), apptest.At(t, "09:20", 0)}, {apptest.At(t, "09:30", 0), apptest.At(t, "09:50", 0)}}
	for i, e := range es {
		if e.Kind != classify.EditLabel || e.StartMS != want[i][0] || e.EndMS != want[i][1] ||
			e.Category != "Study" || e.Project != "Analyse 1" || e.Note != "refile session #1" {
			t.Fatalf("edit %d = %+v", i, e)
		}
	}
	us, err := a.UndoList(5)
	if err != nil || len(us) != 1 || !strings.HasPrefix(us[0].What, "refile session #1 ") {
		t.Fatalf("journal = %+v, %v", us, err)
	}
	after, err := a.Day(apptest.Now)
	if err != nil {
		t.Fatal(err)
	}
	if after.TrackedMS != before.TrackedMS {
		t.Fatalf("tracked %d, was %d", after.TrackedMS, before.TrackedMS)
	}
	for _, hhmm := range []string{"09:10", "09:40"} {
		if _, l, _ := rowAt(t, a, hhmm); l.Project != "Analyse 1" || l.Category != "Study" {
			t.Fatalf("%s label = %+v", hhmm, l)
		}
	}
	if app, l, _ := rowAt(t, a, "09:25"); app != "Idle" || l.Project == "Analyse 1" {
		t.Fatalf("idle row = %s %+v", app, l)
	}
}

func TestRefileUndo(t *testing.T) {
	_, a, x := refileEnv(t)
	if _, _, err := a.RefileSession(x.ID, "Analyse 1"); err != nil {
		t.Fatal(err)
	}
	what, err := a.Undo()
	if err != nil || !strings.HasPrefix(what, "refile session #1 ") {
		t.Fatalf("Undo = %q, %v", what, err)
	}
	if es, _ := a.Edits(apptest.Now); len(es) != 0 {
		t.Fatalf("edits left: %+v", es)
	}
	if _, l, _ := rowAt(t, a, "09:10"); l.Project != "tock" || l.Category != "Code" {
		t.Fatalf("label after undo = %+v", l)
	}
}

func TestRefileNonModule(t *testing.T) {
	_, a, x := refileEnv(t)
	if _, name, err := a.RefileSession(x.ID, "ATLAS"); err != nil || name != "ATLAS" {
		t.Fatalf("RefileSession = %q, %v", name, err)
	}
	es, _ := a.Edits(apptest.Now)
	if len(es) != 2 || es[0].Category != "" || es[0].Project != "ATLAS" {
		t.Fatalf("edits = %+v", es)
	}
	// The category the rules gave is kept.
	if _, l, _ := rowAt(t, a, "09:10"); l.Project != "ATLAS" || l.Category != "Code" {
		t.Fatalf("label = %+v", l)
	}
}

func TestRefileErrors(t *testing.T) {
	_, a, x := refileEnv(t)
	if _, _, err := a.RefileSession(x.ID, "Nope"); err == nil || err.Error() != `no project "Nope" (the labels tab lists them)` {
		t.Fatalf("unknown project: %v", err)
	}
	if _, _, err := a.RefileSession(x.ID, "Old"); err == nil || !strings.Contains(err.Error(), "archived") {
		t.Fatalf("archived project: %v", err)
	}
	if _, _, err := a.RefileSession(999, "ATLAS"); !errors.Is(err, store.ErrSessionGone) {
		t.Fatalf("unknown id: %v", err)
	}
	// A session over idle time only has nothing to refile.
	y := finishedAt(t, a, "09:21", 8, 0)
	if _, _, err := a.RefileSession(y.ID, "ATLAS"); err == nil || err.Error() != "nothing to refile in that session" {
		t.Fatalf("idle only: %v", err)
	}
	run, err := a.st.StartSession(apptest.At(t, "13:00", 0), 0)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := a.RefileSession(run.ID, "ATLAS"); !errors.Is(err, store.ErrSessionOpen) {
		t.Fatalf("running: %v", err)
	}
	if es, _ := a.Edits(apptest.Now); len(es) != 0 {
		t.Fatalf("edits after errors: %+v", es)
	}
	if us, _ := a.UndoList(5); len(us) != 0 {
		t.Fatalf("journal after errors: %+v", us)
	}
}

func TestRefileTwice(t *testing.T) {
	_, a, x := refileEnv(t)
	if _, _, err := a.RefileSession(x.ID, "Analyse 1"); err != nil {
		t.Fatal(err)
	}
	if _, _, err := a.RefileSession(x.ID, "ATLAS"); err != nil {
		t.Fatal(err)
	}
	if us, _ := a.UndoList(5); len(us) != 2 {
		t.Fatalf("journal = %+v", us)
	}
	if es, _ := a.Edits(apptest.Now); len(es) != 4 {
		t.Fatalf("edits = %+v", es)
	}
	if _, err := a.Undo(); err != nil {
		t.Fatal(err)
	}
	es, _ := a.Edits(apptest.Now)
	if len(es) != 2 || es[0].Project != "Analyse 1" {
		t.Fatalf("after one undo: %+v", es)
	}
}

func TestRefileUndoable(t *testing.T) {
	_, a, x := refileEnv(t)
	if ok, err := a.RefileUndoable(x.ID); err != nil || ok {
		t.Fatalf("before any refile = %v, %v", ok, err)
	}
	if _, _, err := a.RefileSession(x.ID, "Analyse 1"); err != nil {
		t.Fatal(err)
	}
	if ok, err := a.RefileUndoable(x.ID); err != nil || !ok {
		t.Fatalf("after refile = %v, %v", ok, err)
	}
	if ok, _ := a.RefileUndoable(x.ID + 41); ok {
		t.Fatal("another session id is undoable")
	}
	if ok, _ := a.RefileUndoable(x.ID * 10); ok {
		t.Fatal("#10 matched #1")
	}
	if _, err := a.AddEdit(classify.EditLabel, apptest.At(t, "11:00", 0), apptest.At(t, "11:05", 0), classify.Want{Category: "Study"}, ""); err != nil {
		t.Fatal(err)
	}
	if ok, _ := a.RefileUndoable(x.ID); ok {
		t.Fatal("still undoable after a newer change")
	}
	if what, err := a.UndoRefile(x.ID); err == nil || err.Error() != "the latest change is not that refile" || what != "" {
		t.Fatalf("UndoRefile = %q, %v", what, err)
	}
	if es, _ := a.Edits(apptest.Now); len(es) != 3 {
		t.Fatalf("refused undo changed the edits: %+v", es)
	}
	if _, err := a.Undo(); err != nil { // the newer label
		t.Fatal(err)
	}
	if what, err := a.UndoRefile(x.ID); err != nil || !strings.HasPrefix(what, "refile session #1 ") {
		t.Fatalf("UndoRefile = %q, %v", what, err)
	}
	if es, _ := a.Edits(apptest.Now); len(es) != 0 {
		t.Fatalf("edits left: %+v", es)
	}
}

// A refile whose undo entry cannot be written changes nothing, instead of
// leaving edits that no undo can revert.
func TestRefileWithoutJournalChangesNothing(t *testing.T) {
	e, a, x := refileEnv(t)
	if _, err := e.RawDB(t).Exec(`DROP TABLE undo`); err != nil {
		t.Fatal(err)
	}
	if _, _, err := a.RefileSession(x.ID, "ATLAS"); err == nil {
		t.Fatal("RefileSession succeeded without a journal")
	}
	if es, err := a.Edits(apptest.Now); err != nil || len(es) != 0 {
		t.Fatalf("edits left behind = %+v, %v", es, err)
	}
}
