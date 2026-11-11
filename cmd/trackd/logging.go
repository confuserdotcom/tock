package main

import (
	"io"
	"log/slog"
)

// newLogger returns the daemon logger. Callers pass only versions, pids,
// paths, counts, browser bundle ids, statuses and errors: never a Sample,
// title or URL (T-1-logleak).
func newLogger(w io.Writer) *slog.Logger {
	return slog.New(slog.NewTextHandler(w, &slog.HandlerOptions{Level: slog.LevelInfo}))
}
