//go:build darwin && cgo

package collect

import "testing"

// A logout, restart or shutdown asked for or past cancelling counts; a
// cancel takes it back.
func TestSessionNotified(t *testing.T) {
	defer sessionSay(0)
	for _, step := range []struct {
		say  int32
		want bool
	}{{0, false}, {1, true}, {0, false}, {1, true}, {2, true}} {
		sessionSay(step.say)
		if got := SessionNotified(); got != step.want {
			t.Fatalf("after %d: SessionNotified = %v, want %v", step.say, got, step.want)
		}
	}
}
