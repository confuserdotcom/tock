//go:build !darwin && !linux && !windows

package main

// bootMS is unknown here: 0.
func bootMS() int64 { return 0 }
