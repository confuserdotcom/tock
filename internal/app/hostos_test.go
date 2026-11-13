package app

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"tock/internal/apptest"
)

// The fixtures were written for macOS, where every machine reports input;
// tests of Linux set a.goos themselves.
func TestMain(m *testing.M) {
	HostOS = "darwin"
	cleanup := apptest.Home()
	code := m.Run()
	cleanup()
	os.Exit(code)
}

// TestHomeIsolated: no test reads the developer's real home (agent logs,
// config); TestMain points HOME at a temp folder.
func TestHomeIsolated(t *testing.T) {
	h, err := os.UserHomeDir()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(filepath.Base(h), "tock-home-") {
		t.Fatalf("tests run with the real home %s", h)
	}
}
