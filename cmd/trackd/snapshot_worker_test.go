package main

import (
	"bytes"
	"errors"
	"io"
	"log/slog"
	"path/filepath"
	"testing"

	"tock/internal/app"
	"tock/internal/apptest"
	"tock/internal/snapshot"
	"tock/internal/store"
)

func TestSnapshotWorkerWritesAndReports(t *testing.T) {
	e := apptest.New(t, apptest.Rules)
	e.Seed(t, apptest.Span{ID: 1, Start: "09:00", End: "10:00", Bundle: "term", App: "WezTerm", Title: "secret title"})
	var out bytes.Buffer
	rc := runSnapshotWorker([]string{"--data", filepath.Dir(e.DB), "--config", filepath.Dir(e.Config)}, &out)
	if rc != 0 {
		t.Fatalf("rc %d, out %q", rc, out.String())
	}
	info, removed, err := decodeSnapshot(out.Bytes(), nil)
	if err != nil || info.Size == 0 || removed != 0 {
		t.Fatalf("info %+v removed %d err %v (out %q)", info, removed, err, out.String())
	}
	if bytes.Contains(out.Bytes(), []byte("secret")) {
		t.Fatalf("worker output leaks content: %q", out.String())
	}
	l, _ := snapshot.List(store.AutoDir(e.DB))
	if len(l) != 1 {
		t.Fatalf("%d snapshots, want 1", len(l))
	}
}

func TestDecodeSnapshot(t *testing.T) {
	var dmg *snapshot.DamagedError
	if _, _, err := decodeSnapshot([]byte(`{"damaged":"row 3 missing","err":"snapshot: database damaged: row 3 missing"}`+"\n"), nil); !errors.As(err, &dmg) || dmg.Detail != "row 3 missing" {
		t.Fatalf("damaged: %v", err)
	}
	if _, _, err := decodeSnapshot([]byte(`{"err":"snapshot: not enough free disk space"}`), nil); err == nil || err.Error() != "snapshot: not enough free disk space" {
		t.Fatalf("plain error: %v", err)
	}
	if _, _, err := decodeSnapshot(nil, errors.New("exit status 2")); err == nil {
		t.Fatal("a failed worker must be an error")
	}
	if _, _, err := decodeSnapshot([]byte("garbage"), nil); err == nil {
		t.Fatal("unreadable output must be an error")
	}
	info, removed, err := decodeSnapshot([]byte(`{"bytes":42,"removed":2}`), nil)
	if err != nil || info.Size != 42 || removed != 2 {
		t.Fatalf("ok: %+v %d %v", info, removed, err)
	}
}

func TestSnapshotLoopUsesTake(t *testing.T) {
	e := apptest.New(t, apptest.Rules)
	e.Seed(t, apptest.Span{ID: 1, Start: "09:00", End: "10:00", Bundle: "term", App: "WezTerm"})
	called := 0
	p := &snapshotLoop{
		open: func() (*app.App, error) {
			return app.Open(app.Paths{DB: e.DB, Rules: e.Rules, Learned: e.Learned, Config: e.Config}, e.NowFunc())
		},
		log:  slog.New(slog.NewTextHandler(io.Discard, nil)),
		take: func() (snapshot.Info, int, error) { called++; return snapshot.Info{Size: 1}, 0, nil },
	}
	t.Cleanup(p.close)
	p.tick()
	if called != 1 {
		t.Fatalf("take called %d times, want 1", called)
	}
}
