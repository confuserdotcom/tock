//go:build !windows

package collect

// NewWindows is unavailable off Windows.
func NewWindows(LinuxOptions) (Source, error) { return nil, ErrUnsupported }
