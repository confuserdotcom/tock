package config

// Notch is the [notch] section: tockbar's panel in the notch of the Mac
// (a pill under the menu bar on a Mac without one).
type Notch struct {
	// Enabled shows sessions, breaks, meetings, session ends, the recap and
	// labelling in the notch instead of as notification banners. On by
	// default.
	Enabled bool
	// ShowInRecordings lets screen sharing and recordings see the panel. Off
	// by default, because it shows window titles.
	ShowInRecordings bool
}

// DefaultNotch is the [notch] section used when the file does not set it.
func DefaultNotch() Notch { return Notch{Enabled: true} }

type notchTOML struct {
	Enabled          *bool `toml:"enabled"`
	ShowInRecordings *bool `toml:"show_in_recordings"`
}

// decodeNotch applies the keys present in f to c.
func decodeNotch(f notchTOML, c *Notch) {
	if f.Enabled != nil {
		c.Enabled = *f.Enabled
	}
	if f.ShowInRecordings != nil {
		c.ShowInRecordings = *f.ShowInRecordings
	}
}
