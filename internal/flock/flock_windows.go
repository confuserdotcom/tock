//go:build windows

package flock

import (
	"errors"

	"golang.org/x/sys/windows"
)

// The lock covers one byte far past any content, so it never blocks reads
// or writes of the file itself (Windows byte-range locks are mandatory).
const lockOffsetHigh = 0x7fffffff

func lock(f File, shared, wait bool) error {
	var flags uint32
	if !shared {
		flags |= windows.LOCKFILE_EXCLUSIVE_LOCK
	}
	if !wait {
		flags |= windows.LOCKFILE_FAIL_IMMEDIATELY
	}
	ol := windows.Overlapped{OffsetHigh: lockOffsetHigh}
	err := windows.LockFileEx(windows.Handle(f.Fd()), flags, 0, 1, 0, &ol)
	if errors.Is(err, windows.ERROR_LOCK_VIOLATION) || errors.Is(err, windows.ERROR_IO_PENDING) {
		return ErrHeld
	}
	return err
}

// stillActive is GetExitCodeProcess's code for a running process.
const stillActive = 259

func alive(pid int) bool {
	h, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, uint32(pid))
	if err != nil {
		// Access denied: it exists, under another user or elevated.
		return errors.Is(err, windows.ERROR_ACCESS_DENIED)
	}
	defer windows.CloseHandle(h)
	var code uint32
	if err := windows.GetExitCodeProcess(h, &code); err != nil {
		return true
	}
	return code == stillActive
}
