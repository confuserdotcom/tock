//go:build !darwin && !linux && !windows

package main

// sessionEnding is never known here: every SIGTERM is a stop.
func sessionEnding() bool { return false }
