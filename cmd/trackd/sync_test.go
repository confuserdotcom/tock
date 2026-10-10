package main

import (
	"bytes"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"tock/internal/app"
	"tock/internal/apptest"
)

func TestSyncLoop(t *testing.T) {
	e := apptest.New(t, apptest.Rules)
	e.Seed(t, apptest.DaySpans()...)
	var logb bytes.Buffer
	s := &syncLoop{
		open: func() (*app.App, error) {
			return app.Open(app.Paths{DB: e.DB, Rules: e.Rules, Learned: e.Learned, Config: e.Config}, e.NowFunc())
		},
		log: slog.New(slog.NewTextHandler(&logb, nil)),
	}
	t.Cleanup(s.close)
	s.tick()
	if logb.Len() != 0 {
		t.Fatalf("sync off must be quiet: %s", logb.String())
	}
	dir := t.TempDir()
	e.WriteConfig(t, "[sync]\ndir = \""+dir+"\"\nmachine = \"mac\"\n")
	s.tick()
	if _, err := os.Stat(filepath.Join(dir, "mac", "2026-10-10.jsonl")); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(logb.String(), "sync on") || !strings.Contains(logb.String(), "written=2") ||
		strings.Contains(logb.String(), "tock │") {
		t.Fatalf("log:\n%s", logb.String())
	}
}
