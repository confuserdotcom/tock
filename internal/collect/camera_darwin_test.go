//go:build darwin && cgo

package collect

import "testing"

// The camera state depends on the machine; this only proves the probe links
// and returns without a permission prompt.
func TestCameraInUseReturns(t *testing.T) {
	t.Logf("camera in use: %v", CameraInUse())
}
