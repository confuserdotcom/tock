// Package stopsignal asks a tock background process to stop cleanly where
// the system has no SIGTERM for it: on Windows, a named event only the
// same user can open. Elsewhere launchd and systemd send signals, and both
// calls return ErrUnsupported.
package stopsignal

import "errors"

var (
	// ErrUnsupported: this platform stops services with signals.
	ErrUnsupported = errors.New("stopsignal: not used on this platform")
	// ErrNotListening: no process waits for that stop signal.
	ErrNotListening = errors.New("stopsignal: nothing is listening")
)

// Listen waits for Send(name). The channel closes when it arrives; release
// stops waiting without closing it. Call release when done.
func Listen(name string) (stop <-chan struct{}, release func(), err error) { return listen(name) }

// Send asks the process listening on name to stop.
func Send(name string) error { return send(name) }

// Trackd is the name trackd listens on.
const Trackd = "trackd"
