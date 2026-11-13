package main

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"tock/internal/stopsignal"
)

// Seams for tests: how a stop is asked for, and how trackd.exe starts.
var (
	sendStop       = stopsignal.Send
	startTrackdExe = func(path, outPath string) error {
		if _, err := os.Stat(path); err != nil {
			return fmt.Errorf("no %s next to tock", filepath.Base(path))
		}
		if err := os.MkdirAll(filepath.Dir(outPath), 0o700); err != nil {
			return err
		}
		out, err := os.OpenFile(outPath, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o600)
		if err != nil {
			return err
		}
		defer out.Close()
		c, err := startDetached(path, nil, out)
		if err != nil {
			return err
		}
		return watchStart(c, outPath, startSettle)
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

// startWindows starts the trackd.exe next to this tock.exe. (At sign-in,
// Windows runs tock start itself once trackd install has written the Run
// key.)
func startWindows(env Env) error {
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	return startTrackdExe(filepath.Join(filepath.Dir(exe), "trackd.exe"), filepath.Join(env.StateDir, "trackd.out"))
}

// startSettle is how long tock start watches a new trackd before saying it
// started: one that cannot run (no database folder, a bad config) stops
// well within it.
const startSettle = 2 * time.Second

// watchStart waits settle for c to exit. Still running: it is let go and
// all is well. Gone: the error carries the end of what it wrote to outPath.
func watchStart(c *exec.Cmd, outPath string, settle time.Duration) error {
	done := make(chan error, 1)
	go func() { done <- c.Wait() }()
	select {
	case <-time.After(settle):
		return c.Process.Release()
	case err := <-done:
		b, _ := os.ReadFile(outPath)
		tail := strings.TrimSpace(string(b))
		if len(tail) > 2000 {
			tail = "…" + tail[len(tail)-2000:]
		}
		if tail == "" {
			tail = "(it wrote nothing; see " + outPath + ")"
		}
		return fmt.Errorf("trackd stopped right after starting (%v):\n%s", err, tail)
	}
}
