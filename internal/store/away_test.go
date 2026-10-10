package store

import (
	"errors"
	"reflect"
	"testing"
)

func TestAway(t *testing.T) {
	s := openAt(t, dbPath(t))
	id1, err := s.AddAway(Away{StartMS: 100, EndMS: 200, Category: "Meeting", AtMS: 1})
	if err != nil {
		t.Fatal(err)
	}
	id2, err := s.AddAway(Away{StartMS: 300, EndMS: 400, AtMS: 2}) // skipped
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.AddAway(Away{StartMS: 500, EndMS: 500, AtMS: 3}); err == nil {
		t.Fatal("an empty interval must fail")
	}
	got, err := s.Aways(150, 350)
	if err != nil {
		t.Fatal(err)
	}
	want := []Away{{id1, 100, 200, "Meeting", "", 1}, {id2, 300, 400, "", "", 2}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Aways = %+v", got)
	}
	if got, _ := s.Aways(200, 300); len(got) != 0 {
		t.Fatalf("touching ranges must not overlap: %+v", got)
	}
	if err := s.SetAwayLabel(id1, "", "tock", 9); err != nil {
		t.Fatal(err)
	}
	if got, _ := s.Aways(0, 1000); got[0].Category != "Meeting" || got[0].Project != "tock" || got[0].AtMS != 9 {
		t.Fatalf("relabel kept/changed wrong fields: %+v", got[0])
	}
	if err := s.SetAwayLabel(99, "x", "", 1); !errors.Is(err, ErrNoAway) {
		t.Fatalf("missing row: %v", err)
	}
}
