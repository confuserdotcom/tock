package store

import "testing"

func TestAutoLabel(t *testing.T) {
	s := openAt(t, dbPath(t))
	defer s.Close()
	a := AutoLabel{Key: "Arc · chess.com", Category: "Entertainment", Reason: "same site", Share: 0.9, Rule: "url=x", MS: 300_000, AtMS: 100}
	if err := s.AddAutoLabel(a); err != nil {
		t.Fatal(err)
	}
	b := a
	b.Category, b.AtMS = "Study", 200
	if err := s.AddAutoLabel(b); err != nil { // same key: first row kept
		t.Fatal(err)
	}
	if err := s.AddAutoLabel(AutoLabel{Key: "Arc · x.org", Category: "Code", Reason: "same app", Share: 1, Rule: "r", AtMS: 50}); err != nil {
		t.Fatal(err)
	}
	got, err := s.AutoLabels(60)
	if err != nil || len(got) != 1 || got[0] != a {
		t.Fatalf("AutoLabels(60) = %+v, %v", got, err)
	}
	seen, err := s.AutoLabelled()
	if err != nil || !seen["Arc · chess.com"] || !seen["Arc · x.org"] || len(seen) != 2 {
		t.Fatalf("AutoLabelled = %v, %v", seen, err)
	}
}
