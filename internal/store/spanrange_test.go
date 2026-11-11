package store

import (
	"database/sql"
	"math"
	"strings"
	"testing"
	"time"

	"tock/internal/span"
)

// A range read must still find a long span that started long before the
// range and an open span that started before it, while scanning only rows
// that start near the range (TestSpansRangeIsBounded).
func TestSpansFindsLongAndOpenSpans(t *testing.T) {
	s := openAt(t, dbPath(t))
	defer s.Close()
	const h = 3600_000
	apply(t, s,
		// 10 h idle span from 0 to 10h.
		span.Op{Kind: span.OpOpen, Span: span.Span{ID: 1, Start: 0, BundleID: span.IdleBundleID, App: span.IdleApp}},
		span.Op{Kind: span.OpClose, ID: 1, TS: 10 * h},
		// Many short spans after it.
		span.Op{Kind: span.OpOpen, Span: span.Span{ID: 2, Start: 10 * h, BundleID: "a", App: "A"}},
		span.Op{Kind: span.OpClose, ID: 2, TS: 10*h + 60_000},
		span.Op{Kind: span.OpOpen, Span: span.Span{ID: 3, Start: 20 * h, BundleID: "b", App: "B"}},
		span.Op{Kind: span.OpClose, ID: 3, TS: 20*h + 60_000},
		// Open span since 30h.
		span.Op{Kind: span.OpOpen, Span: span.Span{ID: 4, Start: 30 * h, BundleID: "c", App: "C"}},
	)
	ids := func(from, to int64) string {
		t.Helper()
		got, err := s.Spans(from, to)
		if err != nil {
			t.Fatal(err)
		}
		var b strings.Builder
		for _, r := range got {
			b.WriteByte(byte('0' + r.ID))
		}
		return b.String()
	}
	cases := []struct {
		from, to int64
		want     string
	}{
		{9 * h, 9*h + 1, "1"},       // inside the long span, 9 h after it began
		{5 * h, 11 * h, "12"},       // long span plus a short one
		{10*h + 60_000, 20 * h, ""}, // nothing between
		{100 * h, 101 * h, "4"},     // open span, 70 h after it began
		{0, 200 * h, "1234"},
		{-5 * h, 0, ""},                        // before everything; span starting at to is excluded
		{math.MinInt64, math.MaxInt64, "1234"}, // AllSpans: from minus the longest must not wrap
	}
	for _, c := range cases {
		if got := ids(c.from, c.to); got != c.want {
			t.Errorf("Spans(%d h, %d h) = %q, want %q", c.from/h, c.to/h, got, c.want)
		}
	}
}

// The range read must not walk all history before the range: the query
// plan has to bound start from below as well as above.
func TestSpansRangeIsBounded(t *testing.T) {
	s := openAt(t, dbPath(t))
	defer s.Close()
	rows, err := s.db.Query(`EXPLAIN QUERY PLAN `+spansQuery, int64(5000), int64(1000))
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var plan []string
	for rows.Next() {
		var id, parent, notused int
		var detail string
		if err := rows.Scan(&id, &parent, &notused, &detail); err != nil {
			t.Fatal(err)
		}
		plan = append(plan, detail)
	}
	joined := strings.Join(plan, "\n")
	if !strings.Contains(joined, "start>? AND start<?") {
		t.Fatalf("span scan is not bounded on both sides:\n%s", joined)
	}
}

func TestRemoteViewsRangeIsBounded(t *testing.T) {
	s := openAt(t, dbPath(t))
	defer s.Close()
	rows, err := s.db.Query(`EXPLAIN QUERY PLAN `+remoteViewsQuery, int64(5000), int64(1000))
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var plan []string
	for rows.Next() {
		var id, parent, notused int
		var detail string
		if err := rows.Scan(&id, &parent, &notused, &detail); err != nil {
			t.Fatal(err)
		}
		plan = append(plan, detail)
	}
	if joined := strings.Join(plan, "\n"); !strings.Contains(joined, "start_ms>? AND start_ms<?") {
		t.Fatalf("remote view scan is not bounded on both sides:\n%s", joined)
	}
}

// Range reads must not take the write lock (the store's transactions are
// BEGIN IMMEDIATE): with trackd mid-write they run alongside it in WAL
// instead of waiting up to busy_timeout.
func TestRangeReadsDontTakeWriteLock(t *testing.T) {
	path := dbPath(t)
	s := openAt(t, path)
	defer s.Close()
	w, err := sql.Open("sqlite", "file:"+path+"?_txlock=immediate")
	if err != nil {
		t.Fatal(err)
	}
	defer w.Close()
	tx, err := w.Begin() // holds the write lock
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	start := time.Now()
	if _, err := s.Spans(0, 1000); err != nil {
		t.Fatal(err)
	}
	if _, err := s.RemoteViews(0, 1000); err != nil {
		t.Fatal(err)
	}
	if d := time.Since(start); d > time.Second {
		t.Fatalf("range reads waited %v for the write lock", d)
	}
}
