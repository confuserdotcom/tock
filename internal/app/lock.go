package app

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"syscall"
	"time"
)

// rulesLockFile, in the config dir, serializes every read-modify-write of
// rules.toml and learned.toml across processes: tock commands, the TUI,
// tock web and trackd's sync can otherwise each read the files, change
// them and lose the other's change.
const rulesLockFile = ".rules.lock"

// rulesLockWait is how long a writer waits for another to finish.
const rulesLockWait = 10 * time.Second

// withRulesLock runs fn holding the rule files' lock.
func (a *App) withRulesLock(fn func() error) error {
	path := filepath.Join(filepath.Dir(a.paths.Rules), rulesLockFile)
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("rules lock: %w", err)
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return fmt.Errorf("rules lock: %w", err)
	}
	defer f.Close() // closing releases the lock
	deadline := time.Now().Add(rulesLockWait)
	for {
		err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
		if err == nil {
			break
		}
		if !errors.Is(err, syscall.EWOULDBLOCK) || time.Now().After(deadline) {
			return fmt.Errorf("rules lock: another tock is changing the rules: %w", err)
		}
		time.Sleep(20 * time.Millisecond)
	}
	return fn()
}
