package main

import (
	"database/sql"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	_ "modernc.org/sqlite"

	"tock/internal/collect"
	"tock/internal/config"
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
	return readRows(t, path)
}

func readRows(t *testing.T, path string) []row {
	t.Helper()
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

// TestCrashRecoveryPipeline crashes the pipeline (no Flush) on a 2 s poll
// schedule with adversarial lateness and checks Recover bounds the loss (D-16).
func TestCrashRecoveryPipeline(t *testing.T) {
	const t0 = int64(1700000000000)
	path := filepath.Join(t.TempDir(), "tock.db")
	st, err := store.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	next, err := st.NextID()
	if err != nil {
		t.Fatal(err)
	}
	cfg := span.DefaultConfig()
	cfg.NextID = next
	m := span.New(cfg)
	// 2000 ms schedule with delays 1, 0, 0, 0, 0, 500, 0, 0.
	for _, ts := range []int64{t0 + 1, t0 + 2000, t0 + 4000, t0 + 6000, t0 + 8000, t0 + 10500, t0 + 12000, t0 + 14000} {
		x := span.Sample{TS: ts, BundleID: "a", App: "A"}
		if err := st.Apply(m.Feed(span.Input{Sample: &x})); err != nil {
			t.Fatal(err)
		}
	}
	// Crash just before the next scheduled sample at t0+16000.
	if err := st.Close(); err != nil {
		t.Fatal(err)
	}
	const crashAt = t0 + 16000

	st, err = store.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if n, err := st.Recover(); err != nil || n != 1 {
		t.Fatalf("Recover = %d, %v; want 1, nil", n, err)
	}
	if err := st.Close(); err != nil {
		t.Fatal(err)
	}
	rows := readRows(t, path)
	if len(rows) != 1 {
		t.Fatalf("want 1 span, got %+v", rows)
	}
	r := rows[0]
	if r.start != t0+1 || !r.end.Valid || r.end.Int64 != t0+8000 {
		t.Fatalf("span = %+v; want [t0+1, t0+8000]", r)
	}
	if loss := crashAt - r.end.Int64; loss > 10000 {
		t.Fatalf("crash loss %d ms > 10000", loss)
	}
}

// TestPipelineTickAdvancesHeartbeat checks a tick with no new sample writes a
// heartbeat at the last sample time.
func TestPipelineTickAdvancesHeartbeat(t *testing.T) {
	const t0 = int64(1700000000000)
	path := filepath.Join(t.TempDir(), "tock.db")
	st, err := store.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	in := make(chan span.Input, 4)
	tick := make(chan time.Time)
	now := t0
	p := pipe{in: in, m: span.New(span.DefaultConfig()), st: st, tick: tick, now: func() int64 { return now }}
	done := make(chan error, 1)
	go func() { done <- p.run() }()

	in <- span.Input{Sample: &span.Sample{TS: t0, BundleID: "a", App: "A"}}
	in <- span.Input{Sample: &span.Sample{TS: t0 + 2000, BundleID: "a", App: "A"}}
	// Let the pipeline take both samples before ticking; it reads now only
	// after receiving the tick, so the unbuffered send orders the write.
	for len(in) > 0 {
		time.Sleep(time.Millisecond)
	}
	now = t0 + 12000
	tick <- time.Time{}
	tick <- time.Time{} // second tick proves the first was fully handled
	close(in)
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	v, ok, err := st.State(store.KeyHeartbeat)
	if err != nil || !ok {
		t.Fatalf("heartbeat_ts missing: %v %v", ok, err)
	}
	if v != strconv.FormatInt(t0+2000, 10) {
		t.Fatalf("heartbeat_ts = %s; want %d", v, t0+2000)
	}
}

// TestMergerConfigCleansTitles: the daemon's merger treats titles that differ
// only by a spinner glyph or counter as one span.
func TestMergerConfigCleansTitles(t *testing.T) {
	const t0 = int64(1_700_000_000_000)
	titles := []string{"[1/3] ✳ workflow", "[1/3] ◐ workflow", "[2/3] ✳ workflow", "(3) workflow"}
	m := span.New(mergerConfig(1, config.DefaultAFK()))
	opens := 0
	for i, title := range titles {
		ops := m.Feed(span.Input{Sample: &span.Sample{TS: t0 + int64(i)*2000, BundleID: "w", App: "W", Title: &title}})
		for _, op := range ops {
			if op.Kind == span.OpOpen {
				opens++
			}
		}
	}
	if opens != 1 {
		t.Fatalf("spinner/counter title changes opened %d spans, want 1", opens)
	}
	other := "other task"
	ops := m.Feed(span.Input{Sample: &span.Sample{TS: t0 + 10_000, BundleID: "w", App: "W", Title: &other}})
	if len(ops) == 0 {
		t.Fatal("a real title change must still produce ops")
	}
}
