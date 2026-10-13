package main

import (
	"bytes"
	"log/slog"
	"strings"
	"testing"

	"tock/internal/app"
	"tock/internal/apptest"
)

func TestPruneLoop(t *testing.T) {
	e := apptest.New(t, apptest.Rules)
	e.Seed(t, apptest.Span{ID: 1, Start: "09:00", End: "10:00", Bundle: "term", App: "WezTerm", Title: "tock │ secret", Day: -10})
	var logb bytes.Buffer
	p := &pruneLoop{
		open: func() (*app.App, error) {
			return app.Open(app.Paths{DB: e.DB, Rules: e.Rules, Learned: e.Learned, Config: e.Config}, e.NowFunc())
		},
		log: slog.New(slog.NewTextHandler(&logb, nil)),
	}
	t.Cleanup(p.close)
	p.tick()
	if logb.Len() != 0 {
		t.Fatalf("off must be quiet: %s", logb.String())
	}
	e.WriteConfig(t, "[privacy]\nkeep_titles = 7\n")
	p.tick()
	if l := logb.String(); !strings.Contains(l, "titles deleted") || !strings.Contains(l, "spans=1") || strings.Contains(l, "secret") {
		t.Fatalf("log:\n%s", l)
	}
	logb.Reset()
	p.tick()
	if logb.Len() != 0 {
		t.Fatalf("nothing left to delete must be quiet: %s", logb.String())
	}
}
