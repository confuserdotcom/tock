package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"tock/internal/store"
)

// A migration runs in a short-lived child, "trackd migrate-worker", before
// trackd opens the store. The snapshot taken before a migration (VACUUM
// INTO, then quick_check on a fresh connection) grows SQLite's C heap by
// about 7 MB that is freed but stays mapped until the process exits; in
// the child it goes when the child does. The child runs the same
// store.Open, so the snapshot, the migrations and the refusal of a newer
// schema are unchanged, and trackd's own Open finds nothing left to do.

// migrateTimeout bounds one worker run.
const migrateTimeout = 10 * time.Minute

// runMigrateWorker opens <data>/tock.db, migrating it if needed, and
// closes it: 0 done, 1 the store would not open (the error is printed),
// 2 bad arguments.
func runMigrateWorker(args []string, out io.Writer) int {
	fs := flag.NewFlagSet("migrate-worker", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	data := fs.String("data", "", "data directory")
	if err := fs.Parse(args); err != nil || *data == "" {
		return 2
	}
	st, err := store.Open(filepath.Join(*data, "tock.db"))
	if err != nil {
		fmt.Fprintln(out, err)
		return 1
	}
	if err := st.Close(); err != nil {
		fmt.Fprintln(out, err)
		return 1
	}
	return 0
}

// workerMigrate runs exe as the migrate worker on dataDir.
func workerMigrate(exe string) func(dataDir string) error {
	return func(dataDir string) error {
		ctx, cancel := context.WithTimeout(context.Background(), migrateTimeout)
		defer cancel()
		out, err := exec.CommandContext(ctx, exe, "migrate-worker", "--data", dataDir).CombinedOutput()
		if err != nil {
			return fmt.Errorf("migrate worker: %w: %s", err, strings.TrimSpace(string(out)))
		}
		return nil
	}
}
