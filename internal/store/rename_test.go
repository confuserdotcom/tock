package store

import "testing"

func TestRenameLabel(t *testing.T) {
	s := openAt(t, dbPath(t))
	defer s.Close()
	apply(t, s, openOp(1, 1000), openOp(2, 2000))
	if err := s.SetOverrides([]Override{{SpanID: 1, Category: "meeting", At: 1}, {SpanID: 2, Category: "Code", Project: "atlas", At: 1}}); err != nil {
		t.Fatal(err)
	}
	id, err := s.AddAway(Away{StartMS: 0, EndMS: 10, Category: "Meeting", AtMS: 1})
	if err != nil {
		t.Fatal(err)
	}
	if err := s.RenameLabel("category", "Meeting", "Meetings", 99); err != nil {
		t.Fatal(err)
	}
	ovs, _ := s.Overrides()
	if ovs[1].Category != "Meetings" || ovs[2].Category != "Code" || ovs[2].Project != "atlas" {
		t.Fatalf("overrides = %+v", ovs)
	}
	aw, _ := s.Aways(0, 100)
	if len(aw) != 1 || aw[0].ID != id || aw[0].Category != "Meetings" || aw[0].AtMS != 99 {
		t.Fatalf("away = %+v", aw)
	}
	if err := s.RenameLabel("title", "a", "b", 1); err == nil {
		t.Fatal("renamed a non-label column")
	}
}
