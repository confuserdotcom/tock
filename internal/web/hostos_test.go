package web

import (
	"os"
	"testing"

	"tock/internal/app"
)

// The fixtures were written for macOS, where every machine reports input.
func TestMain(m *testing.M) {
	app.HostOS = "darwin"
	os.Exit(m.Run())
}
