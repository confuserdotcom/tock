package config

// Notch is the [notch] section: tockbar's panel in the notch of the Mac.
// Displays without a notch get one drawn at the top of their menu bar.
type Notch struct {
	// Enabled shows sessions, breaks, meetings, session ends, the recap and
	// labelling in the notch instead of as notification banners. On by
	// default.
	Enabled bool
	// ShowInRecordings lets screen sharing and recordings see the panel. Off
	// by default, because it shows window titles.
	ShowInRecordings bool
	// AllDisplays draws a notch on every display, the one under the pointer
	// answering. Off: only the display with a notch (else the main one).
	// On by default.
	AllDisplays bool
	// OpenOnHover opens the panel when the pointer rests on the notch. Off
	// by default: the notch grows a little under the pointer and a click
	// (or a swipe down) opens it.
	OpenOnHover bool
}

// DefaultNotch is the [notch] section used when the file does not set it.
func DefaultNotch() Notch { return Notch{Enabled: true, AllDisplays: true} }

type notchTOML struct {
	Enabled          *bool `toml:"enabled"`
	ShowInRecordings *bool `toml:"show_in_recordings"`
	AllDisplays      *bool `toml:"all_displays"`
	OpenOnHover      *bool `toml:"open_on_hover"`
}

// decodeNotch applies the keys present in f to c.
func decodeNotch(f notchTOML, c *Notch) {
	set := func(dst *bool, v *bool) {
		if v != nil {
			*dst = *v
		}
	}
	set(&c.Enabled, f.Enabled)
	set(&c.ShowInRecordings, f.ShowInRecordings)
	set(&c.AllDisplays, f.AllDisplays)
	set(&c.OpenOnHover, f.OpenOnHover)
}
