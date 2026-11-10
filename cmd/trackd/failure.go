package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"tock/internal/store"
)

// failFile holds the last write failure until the next start counts it:
// "<unix ms>\n<error>". The error names the operation and SQLite's code,
// never a title or URL (store errors carry no values).
func failFile(stateDir string) string { return filepath.Join(stateDir, "trackd.failed") }

const maxErrLen = 300

func noteFailure(stateDir string, nowMS int64, err error) error {
	msg := err.Error()
	if len(msg) > maxErrLen {
		msg = msg[:maxErrLen]
	}
	return os.WriteFile(failFile(stateDir), []byte(strconv.FormatInt(nowMS, 10)+"\n"+msg), 0o600)
}

// countFailure adds a noted failure to the counters and removes the note.
func countFailure(stateDir string, st *store.Store, nowMS int64) error {
	b, err := os.ReadFile(failFile(stateDir))
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	at, msg, _ := strings.Cut(string(b), "\n")
	ms, err := strconv.ParseInt(at, 10, 64)
	if err != nil {
		ms = nowMS
	}
	if err := st.AddState(store.KeyApplyFailed, 1, nowMS); err != nil {
		return err
	}
	if err := st.SetState(store.KeyLastError, msg, nowMS); err != nil {
		return err
	}
	if err := st.SetState(store.KeyLastErrorMS, strconv.FormatInt(ms, 10), nowMS); err != nil {
		return err
	}
	if err := os.Remove(failFile(stateDir)); err != nil {
		return fmt.Errorf("remove %s: %w", failFile(stateDir), err)
	}
	return nil
}
