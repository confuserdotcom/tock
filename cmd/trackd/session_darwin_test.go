package main

import (
	"testing"

	"golang.org/x/sys/unix"
)

// On a running Mac that is not shutting down, kern.willshutdown reads (as
// a 4-byte int) and is 0, and nothing says the session is ending.
func TestSessionEndingQuiet(t *testing.T) {
	v, err := unix.SysctlUint32("kern.willshutdown")
	if err != nil {
		t.Fatalf("kern.willshutdown: %v", err)
	}
	if v != 0 || sessionEnding() {
		t.Fatalf("willshutdown = %d, sessionEnding = %v; want 0, false", v, sessionEnding())
	}
}
