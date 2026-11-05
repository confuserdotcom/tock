package app

import (
	"os"
	"testing"
)

// The fixtures were written for macOS, where every machine reports input;
// tests of Linux set a.goos themselves.
func TestMain(m *testing.M) {
	HostOS = "darwin"
	os.Exit(m.Run())
}
