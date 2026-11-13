//go:build unix

package flock

import (
	"errors"
	"syscall"
)

func lock(f File, shared, wait bool) error {
	how := syscall.LOCK_EX
	if shared {
		how = syscall.LOCK_SH
	}
	if !wait {
		how |= syscall.LOCK_NB
	}
	for {
		err := syscall.Flock(int(f.Fd()), how)
		switch {
		case err == nil:
			return nil
		case errors.Is(err, syscall.EINTR):
			continue
		case errors.Is(err, syscall.EWOULDBLOCK):
			return ErrHeld
		default:
			return err
		}
	}
}

func alive(pid int) bool {
	// Signal 0 checks the pid without sending anything. EPERM means it
	// exists under another user.
	return !errors.Is(syscall.Kill(pid, 0), syscall.ESRCH)
}
