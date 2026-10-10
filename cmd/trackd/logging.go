package main

import (
	"errors"
	"io"
	"log/slog"
	"os"
)

// logMaxBytes is the size above which trackd.log is truncated at startup.
const logMaxBytes = 10 << 20

// truncateLogIfLarge empties the log at path when it is larger than max. A
// missing file is fine. launchd keeps its own append-mode descriptor, so new
// lines land at the start of the emptied file.
func truncateLogIfLarge(path string, max int64) error {
	fi, err := os.Stat(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	if fi.Size() <= max {
		return nil
	}
	return os.Truncate(path, 0)
}

// newLogger returns the daemon logger. Callers pass only versions, pids,
// paths, counts, browser bundle ids, statuses and errors: never a Sample,
// title or URL (T-1-logleak).
func newLogger(w io.Writer) *slog.Logger {
	return slog.New(slog.NewTextHandler(w, &slog.HandlerOptions{Level: slog.LevelInfo}))
}
