//go:build windows

package main

import (
	"io"
	"os"
	"path/filepath"

	"tock/internal/service"
)

// logOutput is where trackd's log goes. Nothing redirects a Windows
// process's stderr into a file (launchd and systemd do), so trackd appends
// to trackd.log itself, and still writes to the console when it has one.
func logOutput(stateDir string) io.Writer {
	if err := os.MkdirAll(stateDir, 0o700); err != nil {
		return os.Stderr
	}
	path := filepath.Join(stateDir, "trackd.log")
	// Rotate before trackd holds the file open; runDaemon's own rotate then
	// finds it small.
	_ = service.RotateLog(path, service.LogMaxBytes)
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return os.Stderr
	}
	return io.MultiWriter(f, os.Stderr)
}
