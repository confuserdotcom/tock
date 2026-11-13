// Package flock holds advisory file locks the same way on every platform:
// flock(2) on Unix, LockFileEx on Windows. A lock lives as long as its file
// stays open; closing the file, or the process exiting, releases it.
package flock

import "errors"

// ErrHeld means another open file holds a conflicting lock.
var ErrHeld = errors.New("flock: held by another process")

// Lock locks f: shared when shared is true, else exclusive. With wait false
// it returns ErrHeld at once instead of waiting for a conflicting lock.
func Lock(f File, shared, wait bool) error { return lock(f, shared, wait) }

// Alive reports whether a process with this pid is running. Holders of a
// lock record their pid; a dead one's record is stale.
func Alive(pid int) bool { return pid > 0 && alive(pid) }

// File is what Lock needs from an *os.File.
type File interface{ Fd() uintptr }
