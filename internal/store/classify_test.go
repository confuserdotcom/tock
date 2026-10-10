package store

import (
	"database/sql"
	"io/fs"
	"slices"
	"testing"
	"testing/fstest"

	"tock/internal/span"
)

func TestMigrateV1ToV2(t *testing.T) {
	path := dbPath(t)
	body, err := fs.ReadFile(migrationFS, "migrations/0001_init.sql")
	if err != nil {
		t.Fatal(err)
	}
	raw, err := sql.Open("sqlite", "file:"+path)
	if err != nil {
		t.Fatal(err)
	}
	v1 := fstest.MapFS{"migrations/0001_init.sql": {Data: body}}
	if err := migrateFS(raw, v1, "migrations"); err != nil {
		t.Fatal(err)
	}
	if _, err := raw.Exec(`INSERT INTO span(id, start, "end", bundle_id, app) VALUES(1, 1000, 2000, 'a', 'A')`); err != nil {
		t.Fatal(err)
	}
	raw.Close()

	s := openAt(t, path)
	defer s.Close()
	if got := versionRows(t, s.db); !slices.Equal(got, []int{1, 2, 3, 4, 5, 6}) {
		t.Fatalf("schema_version rows = %v, want [1 2 3 4 5 6]", got)
	}
	for _, tbl := range []string{"override", "inbox_ignore"} {
		if !objectExists(t, s.db, "table", tbl) {
			t.Errorf("table %s missing", tbl)
		}
	}
	rows, err := s.AllSpans()
	if err != nil || len(rows) != 1 || rows[0].ID != 1 {
		t.Fatalf("v1 span lost: %+v, %v", rows, err)
	}
}

func TestSpansOverlapAndFields(t *testing.T) {
	s := openAt(t, dbPath(t))
	defer s.Close()
	apply(t, s,
		span.Op{Kind: span.OpOpen, Span: span.Span{ID: 1, Start: 1000, BundleID: "a", App: "A", Title: strp("t1"), URL: strp("x.com/a")}},
		span.Op{Kind: span.OpClose, ID: 1, TS: 2000},
		span.Op{Kind: span.OpOpen, Span: span.Span{ID: 2, Start: 2000, BundleID: "b", App: "B", Private: true}},
		span.Op{Kind: span.OpClose, ID: 2, TS: 3000},
		span.Op{Kind: span.OpOpen, Span: span.Span{ID: 3, Start: 3000, BundleID: "c", App: "C"}},
	)
	got, err := s.Spans(1500, 2500)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0].ID != 1 || got[1].ID != 2 {
		t.Fatalf("Spans(1500,2500) = %+v, want ids 1,2", got)
	}
	r := got[0]
	if *r.End != 2000 || r.BundleID != "a" || r.App != "A" || *r.Title != "t1" || *r.URL != "x.com/a" || r.Private {
		t.Fatalf("row 1 = %+v", r)
	}
	if !got[1].Private || got[1].Title != nil || got[1].URL != nil {
		t.Fatalf("row 2 = %+v", got[1])
	}
	if got, _ := s.Spans(2000, 2001); len(got) != 1 || got[0].ID != 2 {
		t.Fatalf("half-open interval: span ending at from must be excluded, got %+v", got)
	}
	open, _ := s.Spans(5000, 6000)
	if len(open) != 1 || open[0].ID != 3 || open[0].End != nil {
		t.Fatalf("open span = %+v", open)
	}
	all, _ := s.AllSpans()
	if len(all) != 3 {
		t.Fatalf("AllSpans = %d rows", len(all))
	}
}

func TestOverridesMergeFields(t *testing.T) {
	s := openAt(t, dbPath(t))
	defer s.Close()
	apply(t, s, openOp(1, 1000), openOp(2, 2000))
	if err := s.SetOverrides([]Override{{SpanID: 1, Category: "Study", At: 5}}); err != nil {
		t.Fatal(err)
	}
	if err := s.SetOverrides([]Override{{SpanID: 1, Project: "ATLAS", At: 6}, {SpanID: 2, Project: "tock", At: 6}}); err != nil {
		t.Fatal(err)
	}
	got, err := s.Overrides()
	if err != nil {
		t.Fatal(err)
	}
	if o := got[1]; o.Category != "Study" || o.Project != "ATLAS" || o.At != 6 {
		t.Fatalf("span 1 override = %+v, want Study/ATLAS merged", o)
	}
	if o := got[2]; o.Category != "" || o.Project != "tock" {
		t.Fatalf("span 2 override = %+v", o)
	}
	if err := s.SetOverrides([]Override{{SpanID: 99, Category: "X", At: 1}}); err == nil {
		t.Fatal("override for a missing span must fail (foreign key)")
	}
}

func TestIgnoredRoundTrip(t *testing.T) {
	s := openAt(t, dbPath(t))
	defer s.Close()
	for _, k := range []string{"Arc · x.com", "Arc · x.com", "Finder"} {
		if err := s.SetIgnored(k, true, 1); err != nil {
			t.Fatal(err)
		}
	}
	if err := s.SetIgnored("Finder", false, 2); err != nil {
		t.Fatal(err)
	}
	got, err := s.Ignored()
	if err != nil || len(got) != 1 || !got["Arc · x.com"] {
		t.Fatalf("Ignored = %v, %v", got, err)
	}
}
