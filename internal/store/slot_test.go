package store

import (
	"slices"
	"testing"
)

func TestMarkSlotNotice(t *testing.T) {
	s := openAt(t, dbPath(t))
	defer s.Close()
	const t0 = int64(1_000_000_000)
	if ok, err := s.MarkSlotNotice("plan:k", "start", t0); err != nil || !ok {
		t.Fatalf("first mark = %v, %v, want true", ok, err)
	}
	if ok, err := s.MarkSlotNotice("plan:k", "start", t0+1); err != nil || ok {
		t.Fatalf("second mark = %v, %v, want false", ok, err)
	}
	if ok, err := s.MarkSlotNotice("plan:k", "upcoming", t0); err != nil || !ok {
		t.Fatalf("other kind = %v, %v, want true", ok, err)
	}
	if _, err := s.MarkSlotNotice("plan:other", "start", t0); err != nil {
		t.Fatal(err)
	}
	got, err := s.SlotNotices([]string{"plan:k", "plan:none"})
	if err != nil {
		t.Fatal(err)
	}
	var kinds []string
	for _, n := range got {
		if n.Key != "plan:k" || n.AtMS != t0 {
			t.Fatalf("row %+v, want plan:k at %d", n, t0)
		}
		kinds = append(kinds, n.Kind)
	}
	slices.Sort(kinds)
	if !slices.Equal(kinds, []string{"start", "upcoming"}) {
		t.Fatalf("kinds = %v, want [start upcoming]", kinds)
	}
	if none, err := s.SlotNotices(nil); err != nil || none != nil {
		t.Fatalf("SlotNotices(nil) = %v, %v, want nil, nil", none, err)
	}
}

func TestMarkSlotNoticePrunes(t *testing.T) {
	s := openAt(t, dbPath(t))
	defer s.Close()
	const t0 = int64(1_000_000_000)
	if _, err := s.MarkSlotNotice("plan:old", "start", t0); err != nil {
		t.Fatal(err)
	}
	if _, err := s.MarkSlotNotice("plan:new", "start", t0+6*24*3600_000); err != nil {
		t.Fatal(err)
	}
	if rows, _ := s.SlotNotices([]string{"plan:old"}); len(rows) != 1 {
		t.Fatalf("row kept at 6 days: %v, want 1 row", rows)
	}
	if _, err := s.MarkSlotNotice("plan:new2", "start", t0+8*24*3600_000); err != nil {
		t.Fatal(err)
	}
	if rows, _ := s.SlotNotices([]string{"plan:old"}); len(rows) != 0 {
		t.Fatalf("row after 8 days: %v, want pruned", rows)
	}
}

func TestUnmarkSlotNotice(t *testing.T) {
	s := openAt(t, dbPath(t))
	defer s.Close()
	const t0 = int64(1_000_000_000)
	if _, err := s.MarkSlotNotice("plan:k", "start", t0); err != nil {
		t.Fatal(err)
	}
	if _, err := s.MarkSlotNotice("plan:k", "upcoming", t0); err != nil {
		t.Fatal(err)
	}
	if err := s.UnmarkSlotNotice("plan:k", "start"); err != nil {
		t.Fatal(err)
	}
	if err := s.UnmarkSlotNotice("plan:none", "start"); err != nil {
		t.Fatalf("a missing row: %v, want nil", err)
	}
	got, err := s.SlotNotices([]string{"plan:k"})
	if err != nil || len(got) != 1 || got[0].Kind != "upcoming" {
		t.Fatalf("rows = %+v, %v, want only the upcoming one", got, err)
	}
	if ok, err := s.MarkSlotNotice("plan:k", "start", t0+1); err != nil || !ok {
		t.Fatalf("mark after unmark = %v, %v, want true", ok, err)
	}
}
