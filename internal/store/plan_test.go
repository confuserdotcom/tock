package store

import "testing"

func TestPlan(t *testing.T) {
	s := openAt(t, dbPath(t))
	if p, err := s.Plan("2026-10-05"); err != nil || p != "" {
		t.Fatalf("no plan = %q, %v", p, err)
	}
	if err := s.SetPlan("2026-10-05", "ship tier 2", 1); err != nil {
		t.Fatal(err)
	}
	if err := s.SetPlan("2026-10-05", "write the report", 2); err != nil {
		t.Fatal(err)
	}
	if p, err := s.Plan("2026-10-05"); err != nil || p != "write the report" {
		t.Fatalf("plan = %q, %v", p, err)
	}
}
