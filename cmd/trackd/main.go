// Command trackd is the tock tracking daemon.
package main

import (
	"os"
	"path/filepath"

	"tock/internal/collect"
)

// version is set via -ldflags "-X main.version=...".
var version = "dev"

func home() string {
	if h, err := os.UserHomeDir(); err == nil {
		return h
	}
	return os.Getenv("HOME")
}

func dataDir() string {
	if d := os.Getenv("TOCK_DATA_DIR"); d != "" {
		return d
	}
	return filepath.Join(home(), ".local", "share", "tock")
}

func stateDir() string {
	if d := os.Getenv("TOCK_STATE_DIR"); d != "" {
		return d
	}
	return filepath.Join(home(), ".local", "state", "tock")
}

func main() {
	os.Exit(runDaemon(daemonOpts{
		DataDir:  dataDir(),
		StateDir: stateDir(),
		NewSource: func() (collect.Source, error) {
			return collect.NewDarwin(collect.Options{PollInterval: collect.DefaultPollInterval})
		},
		Stderr: os.Stderr,
	}))
}
