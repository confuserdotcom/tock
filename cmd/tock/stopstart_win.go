package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"tock/internal/stopsignal"
)

// Seams for tests: how a stop is asked for, and how trackd.exe starts.
var (
	sendStop       = stopsignal.Send
	startTrackdExe = func(path string) error {
		if _, err := os.Stat(path); err != nil {
			return fmt.Errorf("no %s next to tock", filepath.Base(path))
		}
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
	return startTrackdExe(filepath.Join(filepath.Dir(exe), "trackd.exe"))
}
