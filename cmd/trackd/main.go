// Command trackd is the tock tracking daemon.
package main

import (
	"flag"
	"fmt"
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

func newSource() (collect.Source, error) {
	return collect.NewDarwin(collect.Options{PollInterval: collect.DefaultPollInterval})
}

func main() {
	dump := flag.Bool("dump", false, "print live samples to stdout and exit on SIGINT; never touches the database")
	flag.Parse()

	if *dump {
		src, err := newSource()
		if err != nil {
			fmt.Fprintf(os.Stderr, "trackd: %v\n", err)
			os.Exit(1)
		}
		os.Exit(runDump(src, os.Stdout))
	}

	os.Exit(runDaemon(daemonOpts{
		DataDir:   dataDir(),
		StateDir:  stateDir(),
		NewSource: newSource,
		Stderr:    os.Stderr,
	}))
}
