package web

import (
	"os"
	"testing"

	"tock/internal/app"
	"tock/internal/apptest"
)

// The fixtures were written for macOS, where every machine reports input.
func TestMain(m *testing.M) {
	app.HostOS = "darwin"
	cleanup := apptest.Home()
	code := m.Run()
	cleanup()
	os.Exit(code)
}
