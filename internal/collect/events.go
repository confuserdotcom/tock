package collect

import "tock/internal/span"

// eventKindFromCode maps the numeric event code sent by the C shim
// (1 sleep, 2 wake, 3 lock, 4 unlock) to a span.EventKind. Unknown codes
// report ok=false and must be ignored.
func eventKindFromCode(code int) (span.EventKind, bool) {
	switch k := span.EventKind(code); k {
	case span.EventSleep, span.EventWake, span.EventLock, span.EventUnlock:
		return k, true
	}
	return 0, false
}
