//go:build !windows

package main

import (
	"io"
	"os"
)

// logOutput is stderr: launchd and systemd write it to trackd.log.
func logOutput(string) io.Writer { return os.Stderr }
