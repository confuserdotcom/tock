package store_test

import (
	"database/sql"
	"path/filepath"
	"reflect"
	"sort"
	"testing"

	"tock/internal/span"
	"tock/internal/store"
)

// row is one span row as the tests compare it.
type row struct {
	ID     int64
	Start  int64
	End    sql.NullInt64
	Bundle string
	Title  sql.NullString
}

// replayRows is an oracle independent of the store: it applies merger ops to
// an in-memory table.
func replayRows(t *testing.T, ops []span.Op) []row {
	t.Helper()
	tbl := map[int64]*row{}
	for i, op := range ops {
		switch op.Kind {
		case span.OpOpen:
			r := &row{ID: op.Span.ID, Start: op.Span.Start, Bundle: op.Span.BundleID}
			if op.Span.Title != nil && !op.Span.Private {
				r.Title = sql.NullString{String: *op.Span.Title, Valid: true}
			}
			tbl[r.ID] = r
		case span.OpClose, span.OpExtend:
			r, ok := tbl[op.ID]
			if !ok {
				t.Fatalf("op %d: %s of unknown id %d", i, op.Kind, op.ID)
			}
			r.End = sql.NullInt64{Int64: op.TS, Valid: true}
		case span.OpReopen:
			r, ok := tbl[op.ID]
			if !ok {
				t.Fatalf("op %d: reopen of unknown id %d", i, op.ID)
			}
			r.End = sql.NullInt64{}
		case span.OpDelete:
			if _, ok := tbl[op.ID]; !ok {
				t.Fatalf("op %d: delete of unknown id %d", i, op.ID)
			}
			delete(tbl, op.ID)
		}
	}
	out := make([]row, 0, len(tbl))
	for _, r := range tbl {
		out = append(out, *r)
	}
	sort.Slice(out, func(a, b int) bool {
		if out[a].Start != out[b].Start {
			return out[a].Start < out[b].Start
		}
		return out[a].ID < out[b].ID
	})
	return out
}

// readRows reads the span table through a second read-only handle so the
// store's own connection is not involved in the comparison.
func readRows(t *testing.T, path string) []row {
	t.Helper()
	db, err := sql.Open("sqlite", "file:"+path+"?mode=ro")
	if err != nil {
		t.Fatalf("open ro: %v", err)
	}
	defer db.Close()
	rs, err := db.Query(`SELECT id, start, "end", bundle_id, title FROM span ORDER BY start, id`)
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	defer rs.Close()
	var out []row
	for rs.Next() {
		var r row
		if err := rs.Scan(&r.ID, &r.Start, &r.End, &r.Bundle, &r.Title); err != nil {
			t.Fatalf("scan: %v", err)
		}
		out = append(out, r)
	}
	if err := rs.Err(); err != nil {
		t.Fatalf("rows: %v", err)
	}
	return out
}

// runThroughStore feeds every input to a fresh merger, applies each Feed's ops
// to a real SQLite store, flushes, and returns both the store rows and the
// replay oracle's rows for the same ops.
func runThroughStore(t *testing.T, ins []span.Input) (got, want []row) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "tock.db")
	st, err := store.Open(path)
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	defer st.Close()

	m := span.New(span.DefaultConfig())
	var all []span.Op
	for i, in := range ins {
		ops := m.Feed(in)
		if err := st.Apply(ops); err != nil {
			t.Fatalf("Apply feed %d: %v", i, err)
		}
		all = append(all, ops...)
	}
	ops := m.Flush()
	if err := st.Apply(ops); err != nil {
		t.Fatalf("Apply flush: %v", err)
	}
	all = append(all, ops...)
	return readRows(t, path), replayRows(t, all)
}

const base = int64(1_000_000)

func sample(ts int64, bundle, title string) span.Input {
	s := &span.Sample{TS: ts, BundleID: bundle, App: bundle}
	if title != "" {
		tt := title
		s.Title = &tt
	}
	return span.Input{Sample: s}
}

func ns(s string) sql.NullString { return sql.NullString{String: s, Valid: true} }
func ni(v int64) sql.NullInt64   { return sql.NullInt64{Int64: v, Valid: true} }

func TestFoldOpsApplyToStore(t *testing.T) {
	act := sample(base+8000, "com.b", "old")
	act.Sample.Activation = true
	got, want := runThroughStore(t, []span.Input{
		sample(base, "com.a", ""),
		sample(base+5000, "com.a", ""),
		act,
		sample(base+8150, "com.b", "real"),
		sample(base+12000, "com.b", "real"),
	})

	if !reflect.DeepEqual(got, want) {
		t.Fatalf("store rows differ from replay oracle\n got:  %+v\n want: %+v", got, want)
	}
	exp := []row{
		{ID: 1, Start: base, End: ni(base + 8000), Bundle: "com.a"},
		{ID: 3, Start: base + 8000, End: ni(base + 12000), Bundle: "com.b", Title: ns("real")},
	}
	if !reflect.DeepEqual(got, exp) {
		t.Fatalf("store rows\n got:  %+v\n want: %+v", got, exp)
	}
	for _, r := range got {
		if r.ID == 2 {
			t.Fatalf("deleted span id 2 is still in the store: %+v", r)
		}
	}
}
