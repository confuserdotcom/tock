package collect

import "tock/internal/desktop"

// NewLinux returns the source for this session's desktop and its name
// (a desktop.* constant). hypr configures Hyprland; opts the others.
func NewLinux(hypr HyprOptions, opts LinuxOptions) (Source, string, error) {
	d, err := desktop.Detect(desktop.System())
	if err != nil {
		return nil, "", err
	}
	var src Source
	switch d {
	case desktop.Hyprland:
		src, err = NewHyprland(hypr)
	case desktop.Sway:
		src, err = NewSway("", opts)
	case desktop.X11:
		opts.IdleFile = "" // the X server knows the idle time itself
		src, err = NewX11("", opts)
	}
	return src, d, err
}

// HideWindows hides every window of class on desktop d: Hyprland's special
// workspace, Sway's scratchpad. X11 has no portable way.
func HideWindows(d, class string) (bool, error) {
	switch d {
	case desktop.Hyprland:
		return HyprHide(class)
	case desktop.Sway:
		return SwayHide(class)
	}
	return false, ErrUnsupported
}
