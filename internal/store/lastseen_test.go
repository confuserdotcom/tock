package store

import (
	"testing"

	"tock/internal/span"
)

func TestLastSeen(t *testing.T) {
	s := openAt(t, dbPath(t))
	defer s.Close()
	if got, err := s.LastSeen(); err != nil || got != 0 {
		t.Fatalf("empty db: %d, %v", got, err)
	}
	apply(t, s, span.Op{Kind: span.OpOpen, Span: span.Span{ID: 1, Start: 1000, BundleID: "a", App: "A"}})
	if got, _ := s.LastSeen(); got != 1000 {
		t.Fatalf("span start only: %d", got)
	}
	apply(t, s, span.Op{Kind: span.OpHeartbeat, TS: 9000})
	if got, _ := s.LastSeen(); got != 9000 {
		t.Fatalf("heartbeat newer than span: %d", got)
	}
	apply(t, s,
		span.Op{Kind: span.OpClose, ID: 1, TS: 9500},
		span.Op{Kind: span.OpOpen, Span: span.Span{ID: 2, Start: 12000, BundleID: "b", App: "B"}})
	if got, _ := s.LastSeen(); got != 12000 {
		t.Fatalf("new span after the last heartbeat: %d", got)
	}
}
