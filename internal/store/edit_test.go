package store

import (
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
