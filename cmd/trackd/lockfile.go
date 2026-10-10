package main

import (
	"errors"
	"os"
	"syscall"
)

// errLockHeld means another trackd process holds the single-writer lock.
var errLockHeld = errors.New("trackd: lock held by another instance")

// acquireLock takes an exclusive, non-blocking flock on path (D-22). The lock
// lives as long as the returned file stays open; the kernel releases it when
// the process exits, so a launchd restart never finds a stale lock.
func acquireLock(path string) (release func(), err error) {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, err
	}
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		_ = f.Close()
		if errors.Is(err, syscall.EWOULDBLOCK) {
			return nil, errLockHeld
		}
		return nil, err
	}
	return func() { _ = f.Close() }, nil
}
