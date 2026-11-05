package main

import (
	"bytes"
	"log/slog"
	"testing"

	"tock/internal/app"
	"tock/internal/apptest"
)

func TestLoopsShareOneApp(t *testing.T) {
	e := apptest.New(t, apptest.Rules)
	e.Seed(t, apptest.DaySpans()...)
	opens := 0
	sh := &shared{open: func() (*app.App, error) {
		opens++
		return app.Open(app.Paths{DB: e.DB, Rules: e.Rules, Learned: e.Learned, Config: e.Config}, e.NowFunc())
	}}
	log := slog.New(slog.NewTextHandler(&bytes.Buffer{}, nil))
	f := &focusLoop{open: sh.opener, log: log}
	loops := []loop{sh.turn(f), sh.turn(&syncLoop{open: sh.opener, log: log}), sh.turn(&pruneLoop{open: sh.opener, log: log})}
	for range 2 {
		for _, l := range loops {
			l.tick()
		}
	}
	for _, l := range loops {
		l.close()
	}
	if opens != 1 {
		t.Fatalf("opened %d app sessions; want 1", opens)
	}
	if sh.a == nil {
		t.Fatal("a loop's close closed the shared session")
	}
	sh.close()
	if sh.a != nil {
		t.Fatal("session kept after close")
	}
	sh.close() // twice is fine
}
