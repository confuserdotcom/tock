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

func configDir() string {
	if d := os.Getenv("TOCK_CONFIG_DIR"); d != "" {
		return d
	}
	return filepath.Join(home(), ".config", "tock")
}

func stateDir() string {
	if d := os.Getenv("TOCK_STATE_DIR"); d != "" {
		return d
	}
	return filepath.Join(home(), ".local", "state", "tock")
}

// newSource builds the macOS collector. Permission prompts are allowed only
// when launchd (or open) started trackd: a terminal-launched process would
// attribute them to the terminal (RESEARCH Q1).
func newSource() (collect.Source, error) {
	underLaunchd := os.Getppid() == 1
	return collect.NewDarwin(collect.Options{
		PollInterval:  collect.DefaultPollInterval,
		PromptAX:      underLaunchd,
		AskAutomation: underLaunchd,
	})
}

func main() {
	if len(os.Args) > 1 {
		var run func() error
		switch os.Args[1] {
		case "install":
			run = func() error { return installAgent(dataDir(), stateDir()) }
		case "uninstall":
			run = uninstallAgent
		}
		if run != nil {
			if err := run(); err != nil {
				fmt.Fprintf(os.Stderr, "trackd %s: %v\n", os.Args[1], err)
				os.Exit(1)
			}
			os.Exit(0)
		}
	}

	dump := flag.Bool("dump", false, "print live samples to stdout and exit on SIGINT; never touches the database")
	flag.Parse()

	if *dump {
		src, err := newSource()
		if err != nil {
			fmt.Fprintf(os.Stderr, "trackd: %v\n", err)
			os.Exit(1)
		}
		os.Exit(runDump(src, os.Stdout, os.Getppid()))
	}

	os.Exit(runDaemon(daemonOpts{
		DataDir:   dataDir(),
		StateDir:  stateDir(),
		ConfigDir: configDir(),
		NewSource: newSource,
		Stderr:    os.Stderr,
		Notifier:  osascriptNotifier{},
		Camera:    collect.CameraInUse,
		Mic:       collect.MicInUse,
		Calendar:  collect.EventKit{},
		// A terminal-launched trackd would attribute the prompt to the
		// terminal (see newSource).
		CalendarPrompt: os.Getppid() == 1,
	}))
}
