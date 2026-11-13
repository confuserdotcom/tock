package main

import (
	"io"
	"os"
	"testing"

	"tock/internal/app"
	"tock/internal/apptest"
	"tock/internal/service"
	"tock/internal/store"
)

// The fixtures were written for macOS, where every machine reports input.
func TestMain(m *testing.M) {
	if p := os.Getenv("TOCK_TEST_HOLD_DB"); p != "" {
		holdStore(p)
		return
	}
	// A portable stand-in for `sh -c 'echo …'` in tests that start a
	// process: print the value and exit.
	if s, ok := os.LookupEnv("TOCK_TEST_ECHO"); ok {
		os.Stdout.WriteString(s + "\n")
		os.Exit(0)
	}
	app.HostOS = "darwin"
	installMethod = func() service.Method { return service.Source }
	cleanup := apptest.Home()
	code := m.Run()
	cleanup()
	os.Exit(code)
}

// holdStore is the helper process of the restore lock test: it opens the
// store, says so, and keeps it open until stdin closes.
func holdStore(path string) {
	st, err := store.Open(path)
	if err != nil {
		os.Stdout.WriteString("error: " + err.Error() + "\n")
		os.Exit(1)
	}
	os.Stdout.WriteString("ready\n")
	_, _ = io.Copy(io.Discard, os.Stdin)
	_ = st.Close()
	os.Exit(0)
}
