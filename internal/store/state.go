package store

// daemon_state keys. Phase 3's banner reads ax_trusted, automation.* and
// version; heartbeat_ts drives crash recovery.
const (
	// KeyHeartbeat is the daemon_state key holding the last heartbeat timestamp.
	KeyHeartbeat = "heartbeat_ts"
	// KeyAXTrusted holds "0" or "1": whether Accessibility is granted.
	KeyAXTrusted = "ax_trusted"
	// KeyVersion holds the trackd version string.
	KeyVersion = "version"
)

// AutomationKey returns the daemon_state key for the Automation permission of
// one browser bundle id. Its value is "granted", "denied", "unknown" or
// "not_running".
func AutomationKey(bundleID string) string {
	return "automation." + bundleID
}
