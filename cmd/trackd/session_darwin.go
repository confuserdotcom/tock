package main

import (
	"golang.org/x/sys/unix"

	"tock/internal/collect"
)

// sessionEnding reports whether macOS has said the user's session is
// ending: loginwindow announced a logout, restart or shutdown (the Apple
// menu, an installer's restart), or launchd has begun shutting the system
// down (kern.willshutdown, as `shutdown` and `reboot` do). A plain SIGTERM
// (tock, launchctl, kill) has neither.
func sessionEnding() bool {
	if collect.SessionNotified() {
		return true
	}
	v, err := unix.SysctlUint32("kern.willshutdown")
	return err == nil && v != 0
}
