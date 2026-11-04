package store

import (
	"slices"
	"testing"

	"tock/internal/span"
)

func TestSpanWords(t *testing.T) {
	s := openAt(t, dbPath(t))
	defer s.Close()
	apply(t, s, openOp(1, 1000), span.Op{Kind: span.OpClose, ID: 1, TS: 5000}, openOp(2, 5000), openOp(3, 9000))
	if err := s.SetSpanWords(1, []string{"kubectl", "rollout"}, 2000); err != nil {
		t.Fatal(err)
	}
	if err := s.SetSpanWords(2, []string{"matrix"}, 6000); err != nil {
		t.Fatal(err)
	}
	if err := s.SetSpanWords(1, []string{"pods"}, 3000); err != nil { // replaces
		t.Fatal(err)
	}
	got, err := s.SpanWords([]int64{1, 2, 3})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || !slices.Equal(got[1], []string{"pods"}) || !slices.Equal(got[2], []string{"matrix"}) {
		t.Fatalf("SpanWords = %v", got)
	}
	if none, err := s.SpanWords(nil); err != nil || len(none) != 0 {
		t.Fatalf("SpanWords(nil) = %v, %v", none, err)
	}
	if err := s.SetSpanWords(3, nil, 9500); err != nil { // nothing to keep: no row
		t.Fatal(err)
	}

	// A deleted span takes its words along.
	apply(t, s, span.Op{Kind: span.OpDelete, ID: 2})
	if got, _ := s.SpanWords([]int64{2}); len(got) != 0 {
		t.Fatalf("after delete = %v", got)
	}

	// Expiry: spans that ended before the cutoff lose their words, the
	// open span keeps them.
	if err := s.SetSpanWords(3, []string{"open"}, 9500); err != nil {
		t.Fatal(err)
	}
	n, err := s.PruneSpanWords(6000)
	if err != nil || n != 1 {
		t.Fatalf("PruneSpanWords = %d, %v, want 1", n, err)
	}
	if got, _ := s.SpanWords([]int64{1, 3}); len(got) != 1 || got[3] == nil {
		t.Fatalf("after prune = %v", got)
	}
}

func TestOCRStats(t *testing.T) {
	s := openAt(t, dbPath(t))
	defer s.Close()
	for _, c := range []struct {
		day, reason string
		cost        int64
	}{{"2026-11-04", "ok", 140}, {"2026-11-04", "ok", 160}, {"2026-11-04", "private", 0}, {"2026-11-03", "ok", 10}} {
		if err := s.CountOCR(c.day, c.reason, c.cost); err != nil {
			t.Fatal(err)
		}
	}
	got, err := s.OCRStats("2026-11-04")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got["ok"] != (OCRStat{N: 2, CostMS: 300}) || got["private"] != (OCRStat{N: 1}) {
		t.Fatalf("OCRStats = %+v", got)
	}
}
