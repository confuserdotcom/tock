//go:build windows

package main

import "tock/internal/collect"

// sessionEnding reports whether Windows said the session ends (logoff,
// restart, shut down): the collector's window heard it.
func sessionEnding() bool { return collect.WindowsSessionEnding() }
