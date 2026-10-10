//go:build !(darwin && cgo)

package collect

// MicInUse is always false without darwin and cgo.
func MicInUse() bool { return false }
