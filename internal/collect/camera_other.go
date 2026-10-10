//go:build !(darwin && cgo)

package collect

// CameraInUse is always false without darwin and cgo.
func CameraInUse() bool { return false }
