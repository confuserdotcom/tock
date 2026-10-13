package store

import (
	"path/filepath"
	"testing"
)

func TestPruneSpans(t *testing.T) {
	s := openTemp(t)
	mustExec(t, s, `INSERT INTO span(id, start, "end", bundle_id, app, title, url) VALUES
		(1, 0, 100, 'arc', 'Arc', 'Secret page', 'bank.com/account/42?x=1'),
		(2, 100, 200, 'term', 'WezTerm', 'tock │ x', NULL),
		(3, 200, 300, 'arc', 'Arc', NULL, 'example.com'),
		(4, 300, 400, 'arc', 'Arc', 'Later', 'later.com/a'),
		(5, 400, NULL, 'arc', 'Arc', 'Open', 'open.com/a')`)
	rows, err := s.TitledBefore(300)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 2 || rows[0].ID != 1 || rows[1].ID != 2 {
		t.Fatalf("titled = %+v, want spans 1 and 2", rows)
	}
	if err := s.PruneSpans([]Kept{{SpanID: 1, Category: "Admin"}, {SpanID: 2, Category: "Code", Project: "tock"}}); err != nil {
		t.Fatal(err)
	}
	all, err := s.Spans(0, 1000)
	if err != nil {
		t.Fatal(err)
	}
	if all[0].Title != nil || all[0].URL == nil || *all[0].URL != "bank.com" || all[0].Kept == nil || all[0].Kept.Category != "Admin" {
		t.Fatalf("span 1 = %+v kept %+v", all[0], all[0].Kept)
	}
	if all[1].Title != nil || all[1].URL != nil || *all[1].Kept != (Kept{SpanID: 2, Category: "Code", Project: "tock"}) {
		t.Fatalf("span 2 = %+v", all[1])
	}
	if all[3].Title == nil || all[3].Kept != nil {
		t.Fatalf("span 4 must be untouched: %+v", all[3])
	}
	// A second prune finds nothing and keeps the first labels.
	if rows, _ := s.TitledBefore(300); len(rows) != 0 {
		t.Fatalf("titled after prune: %+v", rows)
	}
	if err := s.PruneSpans([]Kept{{SpanID: 1, Category: "Other"}}); err != nil {
		t.Fatal(err)
	}
	if all, _ := s.Spans(0, 100); all[0].Kept.Category != "Admin" {
		t.Errorf("kept label replaced: %+v", all[0].Kept)
	}
}

func TestPruneRemote(t *testing.T) {
	s := openTemp(t)
	mustExec(t, s, `INSERT INTO remote_view(machine, day, start_ms, end_ms, bundle, app, title, url) VALUES
		('ryuk', '2026-10-01', 0, 100, 'arc', 'Arc', 'Secret', 'mail.com/inbox/1'),
		('ryuk', '2026-10-01', 100, 200, 'arc', 'Arc', NULL, 'plain.com'),
		('ryuk', '2026-10-09', 900, 1000, 'arc', 'Arc', 'New', 'new.com/x')`)
	n, err := s.PruneRemote(500)
	if err != nil || n != 1 {
		t.Fatalf("pruned %d, %v; want 1", n, err)
	}
	var title, url *string
	if err := s.db.QueryRow(`SELECT title, url FROM remote_view WHERE start_ms = 0`).Scan(&title, &url); err != nil {
		t.Fatal(err)
	}
	if title != nil || url == nil || *url != "mail.com" {
		t.Fatalf("remote row title %v url %v", title, url)
	}
}

func TestURLHost(t *testing.T) {
	for in, want := range map[string]string{"a.com/b": "a.com", "a.com?q": "a.com", "a.com#x": "a.com", "a.com": "a.com", "": ""} {
		if got := URLHost(in); got != want {
			t.Errorf("URLHost(%q) = %q", in, got)
		}
	}
}

func openTemp(t *testing.T) *Store {
	t.Helper()
	s, err := Open(filepath.Join(t.TempDir(), "tock.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func mustExec(t *testing.T, s *Store, q string) {
	t.Helper()
	if _, err := s.db.Exec(q); err != nil {
		t.Fatal(err)
	}
}
