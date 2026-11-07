package main

import "strings"

// managerStopping reads `systemctl --user is-system-running`: "stopping"
// while the user's service manager is going down.
func managerStopping(out string) bool { return strings.TrimSpace(out) == "stopping" }
