package config

import "fmt"

// Inbox is the [inbox] section: what reaches the inbox list.
type Inbox struct {
	// MinMS is the least time a key needs in the inbox window to get its own
	// row; smaller keys are summed in one line. 0 lists every key.
	MinMS int64
	// AutoShare is the least vote share of a same-site or same-app guess
	// that tock applies on its own (0.85 for auto_accept = 85); 0 is off.
	AutoShare float64
}

// DefaultInbox is the [inbox] section used when the file does not set it.
func DefaultInbox() Inbox { return Inbox{MinMS: 2 * 60_000, AutoShare: 0.85} }

type inboxTOML struct {
	MinMinutes *int `toml:"min_minutes"`
	AutoAccept *int `toml:"auto_accept"`
}

// decodeInbox applies the keys present in f to c.
func decodeInbox(f inboxTOML, c *Inbox) error {
	if f.MinMinutes != nil {
		if v := *f.MinMinutes; v < 0 || v > 60 {
			return fmt.Errorf("[inbox] min_minutes = %d, want 0 to 60", v)
		}
		c.MinMS = int64(*f.MinMinutes) * 60_000
	}
	if f.AutoAccept != nil {
		if v := *f.AutoAccept; v != 0 && (v < 60 || v > 100) {
			return fmt.Errorf("[inbox] auto_accept = %d, want 0 (off) or 60 to 100", v)
		}
		c.AutoShare = float64(*f.AutoAccept) / 100
	}
	return nil
}
