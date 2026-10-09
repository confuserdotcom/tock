package main

import (
	"database/sql"
	"path/filepath"
	"testing"

	_ "modernc.org/sqlite"

	"tock/internal/collect"
	"tock/internal/span"
	"tock/internal/store"
)

type row struct {
	id       int64
	start    int64
	end      sql.NullInt64
	bundleID string
}

func runFake(t *testing.T, samples []span.Sample) []row {
	t.Helper()
	path := filepath.Join(t.TempDir(), "tock.db")
	st, err := store.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	ins := make([]span.Input, len(samples))
	for i := range samples {
		ins[i] = span.Input{Sample: &samples[i]}
	}
	src := collect.NewFake(ins)
	m := span.New(span.DefaultConfig())

	done := make(chan error, 1)
	go func() { done <- pipeline(src.Inputs(), m, st, nil) }()
	if err := src.Run(); err != nil {
		t.Fatal(err)
	}
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if err := st.Close(); err != nil {
		t.Fatal(err)
	}

	db, err := sql.Open("sqlite", "file:"+path+"?mode=ro")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	rs, err := db.Query(`SELECT id, start, "end", bundle_id FROM span ORDER BY id`)
	if err != nil {
		t.Fatal(err)
	}
	defer rs.Close()
	var out []row
	for rs.Next() {
		var r row
		if err := rs.Scan(&r.id, &r.start, &r.end, &r.bundleID); err != nil {
			t.Fatal(err)
		}
		out = append(out, r)
	}
	if err := rs.Err(); err != nil {
		t.Fatal(err)
	}
	return out
}

func TestPipelineFakeSourceWritesSpans(t *testing.T) {
	const t0 = int64(1_700_000_000_000)
	rows := runFake(t, []span.Sample{
		{TS: t0, BundleID: "a", App: "A"},
		{TS: t0 + 3000, BundleID: "b", App: "B"},
		{TS: t0 + 5000, BundleID: "b", App: "B"},
	})
	if len(rows) != 2 {
		t.Fatalf("want 2 rows, got %d: %+v", len(rows), rows)
	}
	want := []struct {
		bundle     string
		start, end int64
	}{
		{"a", t0, t0 + 3000},
		{"b", t0 + 3000, t0 + 5000},
	}
	for i, w := range want {
		r := rows[i]
		if r.bundleID != w.bundle || r.start != w.start || !r.end.Valid || r.end.Int64 != w.end {
			t.Errorf("row %d = %+v, want bundle=%s [%d,%d]", i, r, w.bundle, w.start, w.end)
		}
	}
}

func TestPipelineIgnoresEmptyBundle(t *testing.T) {
	rows := runFake(t, []span.Sample{
		{TS: 1_700_000_000_000, BundleID: "", App: "?"},
	})
	if len(rows) != 0 {
		t.Fatalf("want 0 rows, got %+v", rows)
	}
}
