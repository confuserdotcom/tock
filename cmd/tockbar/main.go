// Command tockbar is the tock menu bar app: today's total or the focus
// countdown in the menu bar, and trackd's notifications with buttons.
package main

import (
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"tock/internal/app"
	"tock/internal/classify"
	"tock/internal/config"
	"tock/internal/service"
	"tock/internal/shell"
)

// version is set via -ldflags "-X main.version=...".
var version = "dev"

const agentLabel = "com.tock.bar"

func home() string {
	if h, err := os.UserHomeDir(); err == nil {
		return h
	}
	return os.Getenv("HOME")
}

func envDir(key string, parts ...string) string {
	if d := os.Getenv(key); d != "" {
		return d
	}
	return filepath.Join(append([]string{home()}, parts...)...)
}

func dataDir() string   { return envDir("TOCK_DATA_DIR", ".local", "share", "tock") }
func configDir() string { return envDir("TOCK_CONFIG_DIR", ".config", "tock") }
func stateDir() string  { return envDir("TOCK_STATE_DIR", ".local", "state", "tock") }

func barAgent() service.Agent {
	program := filepath.Join(dataDir(), "tock.app", "Contents", "MacOS", "tockbar")
	return service.Agent{
		Label:   agentLabel,
		Program: program,
		LogPath: filepath.Join(stateDir(), "tockbar.log"),
		Missing: fmt.Sprintf("app not found at %s; run make bundle-bar", program),
	}
}

// tockBin is the tock command the dashboard runs: $TOCK_BIN, else the one
// make install puts in ~/.local/bin (launchd's PATH does not reach it).
func tockBin() string {
	if b := os.Getenv("TOCK_BIN"); b != "" {
		return b
	}
	return filepath.Join(home(), ".local", "bin", "tock")
}

func paths() app.Paths {
	return app.Paths{
		DB:       filepath.Join(dataDir(), "tock.db"),
		Rules:    filepath.Join(configDir(), classify.RulesFile),
		Learned:  filepath.Join(configDir(), classify.LearnedFile),
		Config:   filepath.Join(configDir(), config.File),
		ShellLog: filepath.Join(stateDir(), shell.File),
	}
}

func main() {
	if len(os.Args) > 1 {
		var err error
		switch os.Args[1] {
		case "install":
			err = service.Install(barAgent())
		case "uninstall":
			err = service.Uninstall(agentLabel)
		case "version":
			fmt.Println(version)
		case "notch-demo":
			// Plays made-up notch moments, for screen recordings. No database.
			runDemoUI()
		default:
			err = fmt.Errorf("usage: tockbar [install|uninstall|version|notch-demo]")
		}
		if err != nil {
			fmt.Fprintf(os.Stderr, "tockbar %s: %v\n", os.Args[1], err)
			os.Exit(1)
		}
		return
	}
	log := slog.New(slog.NewTextHandler(os.Stderr, nil))
	log.Info("tockbar starting", "version", version)
	open := func() (*app.App, error) { return app.Open(paths(), time.Now) }
	r := &runner{
		open:  open,
		now:   time.Now,
		log:   log,
		rules: paths().Rules,
		dash: &dash{cmd: func() *exec.Cmd {
			return exec.Command(tockBin(), "web", "--no-open", "--idle", dashIdle.String())
		}},
		// Ask for notification access only when launchd (or open) started
		// the app: from a terminal the prompt would name the terminal.
		prompt: os.Getppid() == 1,
	}
	runUI(r)
}
