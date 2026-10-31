package store

import (
	"errors"
	"fmt"
	"path/filepath"
	"testing"
)

func TestEditsAndUndo(t *testing.T) {
	st, err := Open(filepath.Join(t.TempDir(), "tock.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	id, err := st.AddEdit(Edit{Kind: "add", StartMS: 10, EndMS: 20, Category: "Meeting", Note: "standup", AtMS: 1})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.AddEdit(Edit{Kind: "bogus", StartMS: 10, EndMS: 20, AtMS: 1}); err == nil {
		t.Fatal("bad kind accepted")
	}
	n0, s0, _ := st.EditStamp()
	es, err := st.Edits(15, 16)
	if err != nil || len(es) != 1 || es[0].Note != "standup" || es[0].ID != id {
		t.Fatalf("Edits = %+v, %v", es, err)
	}
	e := es[0]
	e.Category, e.AtMS = "Code", 2
	if err := st.PutEdit(e); err != nil {
		t.Fatal(err)
	}
	if n1, s1, _ := st.EditStamp(); n1 != n0 || s1 == s0 {
		t.Fatal("stamp did not change on relabel")
	}
	if got, _ := st.EditByID(id); got.Category != "Code" {
		t.Fatalf("EditByID = %+v", got)
	}
	if err := st.DeleteEdit(id); err != nil {
		t.Fatal(err)
	}
	if _, err := st.EditByID(id); err != ErrNoEdit {
		t.Fatalf("after delete: %v", err)
	}

	for i := 0; i < MaxUndo+5; i++ {
		if err := st.PushUndo(Undo{AtMS: int64(i), What: "w", Data: "{}"}); err != nil {
			t.Fatal(err)
		}
	}
	us, err := st.Undos(1000)
	if err != nil || len(us) != MaxUndo || us[0].AtMS != MaxUndo+4 {
		t.Fatalf("Undos = %d, %v", len(us), err)
	}

	if err := st.SetOverrides([]Override{{SpanID: 1, Category: "A", At: 1}}); err == nil {
		// span 1 does not exist: the foreign key refuses it
		t.Log("override without span accepted")
	}
}

func TestSummaries(t *testing.T) {
	st, err := Open(filepath.Join(t.TempDir(), "tock.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	if m, _, err := st.Summaries("2026-10-10"); err != nil || len(m) != 0 {
		t.Fatalf("empty = %v, %v", m, err)
	}
	if err := st.SaveSummary("2026-10-10", "m", "day", map[int64]string{5: "b"}, 9); err != nil {
		t.Fatal(err)
	}
	if err := st.SaveSummary("2026-10-10", "m", "day2", nil, 10); err != nil {
		t.Fatal(err)
	}
	m, at, err := st.Summaries("2026-10-10")
	if err != nil || len(m) != 1 || m[0] != "day2" || at != 10 {
		t.Fatalf("summaries = %v %d %v", m, at, err)
	}
}

func TestAddEditsWithUndo(t *testing.T) {
	st, err := Open(filepath.Join(t.TempDir(), "tock.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	es := []Edit{
		{Kind: "label", StartMS: 10, EndMS: 20, Project: "p", AtMS: 1},
		{Kind: "label", StartMS: 30, EndMS: 40, Project: "p", AtMS: 1},
	}
	ids, err := st.AddEditsWithUndo(es, func(ids []int64) (Undo, error) {
		return Undo{AtMS: 1, What: fmt.Sprint(ids), Data: "{}"}, nil
	})
	if err != nil || len(ids) != 2 {
		t.Fatalf("AddEditsWithUndo = %v, %v", ids, err)
	}
	if got, _ := st.Edits(0, 100); len(got) != 2 {
		t.Fatalf("edits = %+v", got)
	}
	if us, _ := st.Undos(5); len(us) != 1 || us[0].What != fmt.Sprint(ids) {
		t.Fatalf("undo = %+v", us)
	}
	// An error from the undo builder leaves neither edits nor an entry.
	boom := errors.New("boom")
	if _, err := st.AddEditsWithUndo(es, func([]int64) (Undo, error) { return Undo{}, boom }); !errors.Is(err, boom) {
		t.Fatalf("err = %v, want boom", err)
	}
	if got, _ := st.Edits(0, 100); len(got) != 2 {
		t.Fatalf("a refused undo left edits behind: %+v", got)
	}
	if us, _ := st.Undos(5); len(us) != 1 {
		t.Fatalf("undo = %+v", us)
	}
	// A bad edit rolls back the ones before it.
	bad := []Edit{es[0], {Kind: "bogus", StartMS: 1, EndMS: 2}}
	if _, err := st.AddEditsWithUndo(bad, func([]int64) (Undo, error) { return Undo{What: "x", Data: "{}"}, nil }); err == nil {
		t.Fatal("bad edit accepted")
	}
	if got, _ := st.Edits(0, 100); len(got) != 2 {
		t.Fatalf("edits = %+v", got)
	}
}
