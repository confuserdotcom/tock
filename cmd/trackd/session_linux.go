package main

import (
	"context"
	"os/exec"
	"time"
)

// sessionEnding reports whether systemd is stopping the user's service
// manager: the last session logged out, or the system is shutting down
// (user@.service is stopped either way). `systemctl --user stop trackd`
// leaves the manager running, so it is not a session ending.
func sessionEnding() bool {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	// It exits non-zero for every state but running; the state is on stdout.
	out, _ := exec.CommandContext(ctx, "systemctl", "--user", "is-system-running").Output()
	return managerStopping(string(out))
}
