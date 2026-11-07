package main

import (
	"bytes"
	"log/slog"
	"strings"
	"testing"

	"tock/internal/app"
	"tock/internal/apptest"
	"tock/internal/snapshot"
	"tock/internal/store"
)

func TestSnapshotLoop(t *testing.T) {
	e := apptest.New(t, apptest.Rules)
	e.Seed(t, apptest.Span{ID: 1, Start: "09:00", End: "10:00", Bundle: "term", App: "WezTerm", Title: "secret title"})
	var logb bytes.Buffer
	p := &snapshotLoop{
		open: func() (*app.App, error) {
			return app.Open(app.Paths{DB: e.DB, Rules: e.Rules, Learned: e.Learned, Config: e.Config}, e.NowFunc())
		},
		log: slog.New(slog.NewTextHandler(&logb, nil)),
	}
	t.Cleanup(p.close)
	p.tick()
	if l := logb.String(); !strings.Contains(l, "snapshot written") || strings.Contains(l, "secret") {
		t.Fatalf("log:\n%s", l)
	}
	l, _ := snapshot.List(store.AutoDir(e.DB))
	if len(l) != 1 {
		t.Fatalf("%d snapshots, want 1", len(l))
	}
	logb.Reset()
	p.tick()
	if logb.Len() != 0 {
		t.Fatalf("not due must be quiet: %s", logb.String())
	}
}
