//go:build darwin && cgo

package collect

import "testing"

// The microphone state depends on the machine; this only proves the probe
// links and returns without a permission prompt.
func TestMicInUseReturns(t *testing.T) {
	t.Logf("mic in use: %v", MicInUse())
}
