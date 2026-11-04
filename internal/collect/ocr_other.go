//go:build !(darwin && cgo)

package collect

// OCRSupported reports whether this build can read the screen: macOS only.
const OCRSupported = false

// RunOCRWorker is unavailable without darwin and cgo.
func RunOCRWorker(bundle string) (string, string) { return "", OCRUnavailable }

// GrantScreen is unavailable without darwin and cgo.
func GrantScreen() bool { return false }

// SecureInput is always false without darwin and cgo.
func SecureInput() bool { return false }
