package collect

import (
	"testing"

	"tock/internal/span"
)

func TestEventKindFromCode(t *testing.T) {
	good := map[int]span.EventKind{
		1: span.EventSleep,
		2: span.EventWake,
		3: span.EventLock,
		4: span.EventUnlock,
	}
	for code, want := range good {
		got, ok := eventKindFromCode(code)
		if !ok || got != want {
			t.Errorf("eventKindFromCode(%d) = %v, %v; want %v, true", code, got, ok, want)
		}
	}
	for _, code := range []int{0, 5, -1} {
		if _, ok := eventKindFromCode(code); ok {
			t.Errorf("eventKindFromCode(%d) ok = true; want false", code)
		}
	}
}

// The C shim sends the numeric codes 1..4 directly; this guards the contract.
func TestEventKindValuesMatchC(t *testing.T) {
	want := map[span.EventKind]int{
		span.EventSleep:  1,
		span.EventWake:   2,
		span.EventLock:   3,
		span.EventUnlock: 4,
	}
	for k, v := range want {
		if int(k) != v {
			t.Errorf("%v = %d; want %d", k, int(k), v)
		}
	}
}
