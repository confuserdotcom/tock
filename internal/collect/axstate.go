package collect

import "errors"

// ErrAXStale is returned by Source.Run when the process reports Accessibility
// as trusted but AX calls keep failing with kAXErrorAPIDisabled. The daemon
// exits non-zero so KeepAlive restarts it with a fresh TCC state.
var ErrAXStale = errors.New("collect: accessibility granted but AX API still disabled; restart required")

// AXError codes from AXError.h that the poll can observe.
const (
	axErrAPIDisabled          = -25211
	axErrCannotComplete       = -25204
	axErrNoValue              = -25212
	axErrAttributeUnsupported = -25205
	axErrNotImplemented       = -25208
	axErrInvalidUIElement     = -25202
)

// axStaleThreshold is the number of consecutive trusted-but-API-disabled polls
// after which the grant is considered stuck.
const axStaleThreshold = 3

// axTracker follows Accessibility trust across polls: it reports transitions
// (so the daemon logs once per change) and detects a grant that never took
// effect in this process.
type axTracker struct {
	known       bool
	trusted     bool
	disabledRun int
}

// observe feeds one poll result. changed is true on the first observation and
// whenever trust flips. stale is true once trusted polls have returned
// kAXErrorAPIDisabled axStaleThreshold times in a row; any other result
// resets the run.
func (t *axTracker) observe(trusted bool, axErr int) (changed, nowTrusted, stale bool) {
	changed = !t.known || t.trusted != trusted
	t.known = true
	t.trusted = trusted

	if trusted && axErr == axErrAPIDisabled {
		t.disabledRun++
	} else {
		t.disabledRun = 0
	}
	return changed, trusted, trusted && t.disabledRun >= axStaleThreshold
}
