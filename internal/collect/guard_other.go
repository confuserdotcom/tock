//go:build !(darwin && cgo)

package collect

// CloseTab is unavailable without darwin and cgo.
func CloseTab(bundle, host string) (bool, error) { return false, nil }

// HideApp is unavailable without darwin and cgo; Hyprland uses HyprHide.
func HideApp(bundle string) (bool, error) { return false, ErrUnsupported }
