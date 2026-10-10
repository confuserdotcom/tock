package main

import (
	"fmt"
	"path/filepath"

	"tock/internal/launchd"
)

const agentLabel = "com.tock.trackd"

func trackdAgent(dataDir, stateDir string) launchd.Agent {
	program := filepath.Join(dataDir, "trackd.app", "Contents", "MacOS", "trackd")
	return launchd.Agent{
		Label:   agentLabel,
		Program: program,
		LogPath: filepath.Join(stateDir, "trackd.log"),
		Missing: fmt.Sprintf("bundle not found at %s; run make bundle", program),
	}
}

// installAgent writes the plist for the bundle under dataDir and (re)loads it.
func installAgent(dataDir, stateDir string) error {
	return launchd.Install(trackdAgent(dataDir, stateDir))
}

// uninstallAgent unloads the agent and removes its plist. tock.db and the log
// are kept.
func uninstallAgent() error { return launchd.Uninstall(agentLabel) }
