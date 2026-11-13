package main

import (
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"tock/internal/paths"
	"tock/internal/stopsignal"
)

func TestStopAndStart(t *testing.T) {
	te := newTestEnv(t, "")
	calls := stubService(t)
	if code, out := te.run(t, "", "stop"); code != 0 || !strings.Contains(out, "stopped") {
		t.Fatalf("stop: %d %q %s", code, out, te.errb.String())
	}
	if code, out := te.run(t, "", "start"); code != 0 || !strings.Contains(out, "started") {
		t.Fatalf("start: %d %q %s", code, out, te.errb.String())
	}
	if strings.Join(*calls, ",") != "stop,start" {
		t.Fatalf("calls = %v", *calls)
	}
	if code, _ := te.run(t, "", "stop", "now"); code != 2 {
		t.Fatalf("stop with an argument: %d", code)
	}
}

func TestStopFails(t *testing.T) {
	te := newTestEnv(t, "")
	old := restoreStop
	restoreStop = func(Env) error { return errors.New("nope") }
	t.Cleanup(func() { restoreStop = old })
	if code, _ := te.run(t, "", "stop"); code != 1 || !strings.Contains(te.errb.String(), "nope") {
		t.Fatalf("code %d err %s", code, te.errb.String())
	}
}

func TestWindowsStopWaitsForTrackdToGo(t *testing.T) {
	te := newTestEnv(t, "")
	old, oldGOOS := sendStop, setupGOOS
	t.Cleanup(func() { sendStop, setupGOOS = old, oldGOOS })
	setupGOOS = "windows"
	te.DataDir = paths.Standard().Data // the services' own dir
	te.Sleep = func(time.Duration) {}
	sends := 0
	sendStop = func(string) error {
		sends++
		if sends < 3 {
			return nil // still listening: still running
		}
		return stopsignal.ErrNotListening
	}
	if err := stopServices(te.Env); err != nil || sends != 3 {
		t.Fatalf("stop: %v after %d sends", err, sends)
	}
	sendStop = func(string) error { return nil } // never goes
	if err := stopServices(te.Env); err == nil || !strings.Contains(err.Error(), "still running") {
		t.Fatalf("stuck trackd: %v", err)
	}
	sendStop = func(string) error { return stopsignal.ErrNotListening } // not running
	if err := stopServices(te.Env); err != nil {
		t.Fatalf("not running: %v", err)
	}
}

func TestWindowsStartRunsTrackdNextToTock(t *testing.T) {
	te := newTestEnv(t, "")
	old, oldGOOS := startTrackdExe, setupGOOS
	t.Cleanup(func() { startTrackdExe, setupGOOS = old, oldGOOS })
	setupGOOS = "windows"
	te.DataDir = paths.Standard().Data
	var got string
	startTrackdExe = func(p, _ string) error { got = p; return nil }
	if err := startServices(te.Env); err != nil {
		t.Fatal(err)
	}
	if filepath.Base(got) != "trackd.exe" {
		t.Fatalf("started %q", got)
	}
}
