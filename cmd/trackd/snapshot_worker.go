package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os/exec"
	"path/filepath"
	"time"

	"tock/internal/app"
	"tock/internal/classify"
	"tock/internal/config"
	"tock/internal/snapshot"
)

// The daily snapshot runs in a short-lived child, "trackd snapshot-worker".
// VACUUM INTO grows SQLite's C heap by about 10 MB that is never handed
// back to the system; in a child it goes away when the child exits, so the
// daemon stays inside its 40 MB budget.

// snapshotTimeout bounds one worker run.
const snapshotTimeout = 10 * time.Minute

// snapshotResult is the one JSON line a worker prints. It holds sizes and
// error text only.
type snapshotResult struct {
	Bytes   int64  `json:"bytes,omitempty"`
	Removed int    `json:"removed,omitempty"`
	Err     string `json:"err,omitempty"`
	Damaged string `json:"damaged,omitempty"`
}

// runSnapshotWorker takes the daily snapshot of <data>/tock.db and prints
// a snapshotResult. A snapshot that fails is still exit 0 with Err set;
// a nonzero exit means the worker itself could not run.
func runSnapshotWorker(args []string, out io.Writer) int {
	fs := flag.NewFlagSet("snapshot-worker", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	data := fs.String("data", "", "data directory")
	conf := fs.String("config", "", "config directory")
	if err := fs.Parse(args); err != nil || *data == "" || *conf == "" {
		return 2
	}
	a, err := app.Open(app.Paths{
		DB:      filepath.Join(*data, "tock.db"),
		Rules:   filepath.Join(*conf, classify.RulesFile),
		Learned: filepath.Join(*conf, classify.LearnedFile),
		Config:  filepath.Join(*conf, config.File),
	}, time.Now)
	var res snapshotResult
	if err != nil {
		res.Err = err.Error()
	} else {
		info, removed, serr := a.DailySnapshot()
		a.Close()
		res.Bytes, res.Removed = info.Size, removed
		if serr != nil {
			res.Err = serr.Error()
			var dmg *snapshot.DamagedError
			if errors.As(serr, &dmg) {
				res.Damaged = dmg.Detail
			}
		}
	}
	if err := json.NewEncoder(out).Encode(res); err != nil {
		return 1
	}
	return 0
}

// decodeSnapshot turns a worker's output and exit error into what
// app.DailySnapshot would have returned.
func decodeSnapshot(out []byte, runErr error) (snapshot.Info, int, error) {
	if runErr != nil {
		return snapshot.Info{}, 0, fmt.Errorf("snapshot worker: %w", runErr)
	}
	var res snapshotResult
	if err := json.Unmarshal(bytes.TrimSpace(out), &res); err != nil {
		return snapshot.Info{}, 0, fmt.Errorf("snapshot worker: unreadable output: %w", err)
	}
	switch {
	case res.Damaged != "":
		return snapshot.Info{}, 0, &snapshot.DamagedError{Detail: res.Damaged}
	case res.Err != "":
		return snapshot.Info{}, 0, errors.New(res.Err)
	}
	return snapshot.Info{Size: res.Bytes}, res.Removed, nil
}

// workerSnapshot runs exe as the snapshot worker. Only the binary itself
// is run, never a PATH lookup.
func workerSnapshot(exe, dataDir, configDir string) func() (snapshot.Info, int, error) {
	return func() (snapshot.Info, int, error) {
		ctx, cancel := context.WithTimeout(context.Background(), snapshotTimeout)
		defer cancel()
		out, err := exec.CommandContext(ctx, exe, "snapshot-worker", "--data", dataDir, "--config", configDir).Output()
		return decodeSnapshot(out, err)
	}
}
