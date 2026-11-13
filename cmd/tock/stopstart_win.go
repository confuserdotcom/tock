package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"time"

	"tock/internal/stopsignal"
)

// Seams for tests: how a stop is asked for, and how trackd.exe starts.
var (
	sendStop       = stopsignal.Send
	startTrackdExe = func(path string) error {
		devnull, err := os.Open(os.DevNull)
		if err != nil {
			return err
		}
		defer devnull.Close()
		return detach(path, nil, devnull)
	}
)

// stopWindows asks trackd to stop over its named event and waits up to 10 s
// for it to stop listening, which it does as it exits.
func stopWindows(env Env) error {
	err := sendStop(stopsignal.Trackd)
	if errors.Is(err, stopsignal.ErrNotListening) {
		return nil // not running
	}
	if err != nil {
		return err
	}
	for i := 0; i < 40; i++ {
		env.Sleep(250 * time.Millisecond)
		if errors.Is(sendStop(stopsignal.Trackd), stopsignal.ErrNotListening) {
			return nil
		}
	}
	return errors.New("trackd still running after 10 s; end it in Task Manager")
}

// startWindows starts the trackd.exe next to this tock.exe. (Task
// Scheduler starts it at logon once tock setup registers it.)
func startWindows() error {
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	trackd := filepath.Join(filepath.Dir(exe), "trackd.exe")
	if runtime.GOOS == "windows" { // tests elsewhere fake the start
		if _, err := os.Stat(trackd); err != nil {
			return fmt.Errorf("no trackd.exe next to %s", exe)
		}
	}
	return startTrackdExe(trackd)
}
