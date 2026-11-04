package store

import (
	"reflect"
	"testing"
)

func TestDerivedRows(t *testing.T) {
	s := openAt(t, dbPath(t))
	if got, err := s.DerivedRows([]string{"2026-11-01"}); err != nil || len(got) != 0 {
		t.Fatalf("empty cache = %v, %v", got, err)
	}
	if err := s.PutDerived(map[string]Derived{"2026-11-01": {"k1", []byte("a")}, "2026-11-02": {"k2", []byte("b")}}, 5); err != nil {
		t.Fatal(err)
	}
	if err := s.PutDerived(map[string]Derived{"2026-11-01": {"k3", []byte("c")}}, 6); err != nil {
		t.Fatal(err)
	}
	got, err := s.DerivedRows([]string{"2026-11-01", "2026-11-02", "2026-11-03"})
	want := map[string]Derived{"2026-11-01": {"k3", []byte("c")}, "2026-11-02": {"k2", []byte("b")}}
	if err != nil || !reflect.DeepEqual(got, want) {
		t.Fatalf("cache = %v, %v", got, err)
	}
}

// SpanMarks returns the overrides and kept labels of the spans that overlap
// the range, the open span included.
func TestSpanMarks(t *testing.T) {
	s := openAt(t, dbPath(t))
	for _, q := range []string{
		`INSERT INTO span(id,start,"end",bundle_id,app,title) VALUES(1,0,10,'b','B','t')`,
		`INSERT INTO span(id,start,"end",bundle_id,app,title) VALUES(2,20,30,'b','B','t')`,
		`INSERT INTO span(id,start,"end",bundle_id,app,title) VALUES(3,40,NULL,'b','B','t')`,
		`INSERT INTO override(span_id,category,project,at) VALUES(1,'Study','',1)`,
		`INSERT INTO override(span_id,category,project,at) VALUES(3,'Code','tock',1)`,
		`INSERT INTO kept(span_id,category,project) VALUES(2,'Social','')`,
	} {
		if _, err := s.db.Exec(q); err != nil {
			t.Fatal(err)
		}
	}
	got, err := s.SpanMarks(5, 45, 100)
	want := []SpanMark{
		{SpanID: 1, StartMS: 0, EndMS: 10, Category: "Study"},
		{SpanID: 2, StartMS: 20, EndMS: 30, Kept: true, Category: "Social"},
		{SpanID: 3, StartMS: 40, EndMS: 100, Category: "Code", Project: "tock"},
	}
	if err != nil || !reflect.DeepEqual(got, want) {
		t.Fatalf("marks = %+v, %v", got, err)
	}
	if got, _ := s.SpanMarks(11, 19, 100); len(got) != 0 {
		t.Fatalf("no span in [11, 19): %+v", got)
	}
}
