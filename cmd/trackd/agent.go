package main

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"time"

	"tock/internal/collect"

	"tock/internal/service"
)

const agentLabel = "com.tock.trackd"

func trackdAgent(dataDir, stateDir string) service.Agent {
	program := filepath.Join(dataDir, "trackd.app", "Contents", "MacOS", "trackd")
	return service.Agent{
		Label:   agentLabel,
		Program: program,
		LogPath: filepath.Join(stateDir, "trackd.log"),
		Missing: fmt.Sprintf("bundle not found at %s; run make bundle", program),
	}
}

// unitName is trackd's systemd user unit on Linux.
const unitName = "trackd"

// installService installs trackd's background service: on macOS the
// LaunchAgent for the bundle under dataDir, on Linux a systemd user unit
// running this executable.
func installService(dataDir, stateDir string) error {
	if runtime.GOOS != "linux" {
		return service.Install(trackdAgent(dataDir, stateDir))
	}
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	if exe, err = filepath.EvalSymlinks(exe); err != nil {
		return err
	}
	return service.InstallUnit(service.Unit{Name: unitName, Description: "tock activity tracker", Program: exe,
		LogPath: filepath.Join(stateDir, "trackd.log"), Missing: "trackd not found; run make install-linux"})
}

// uninstallService removes the service. tock.db and the log are kept.
func uninstallService() error {
	if runtime.GOOS == "linux" {
		return service.UninstallUnit(unitName)
	}
	return service.Uninstall(agentLabel)
}

// runIdleHook is trackd idle [--after SECONDS] and trackd active, which
// hypridle runs on Linux: they write and clear the idle file the Hyprland
// collector reads.
func runIdleHook(cmd string, args []string, stateDir string, now time.Time) error {
	path := filepath.Join(stateDir, collect.IdleFile)
	if cmd == "active" {
		if len(args) > 0 {
			return errors.New("usage: trackd active")
		}
		return collect.ClearIdle(path)
	}
	fs := flag.NewFlagSet("idle", flag.ContinueOnError)
	after := fs.Int("after", 0, "seconds the user has already been idle (the hypridle timeout)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() > 0 || *after < 0 {
		return errors.New("usage: trackd idle [--after SECONDS]")
	}
	return collect.WriteIdle(path, now.Round(0).UnixMilli(), time.Duration(*after)*time.Second)
}
