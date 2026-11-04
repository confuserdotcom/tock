package config

import "fmt"

// Inbox is the [inbox] section: what reaches the inbox list.
type Inbox struct {
	// MinMS is the least time a key needs in the inbox window to get its own
	// row; smaller keys are summed in one line. 0 lists every key.
	MinMS int64
}

// DefaultInbox is the [inbox] section used when the file does not set it.
func DefaultInbox() Inbox { return Inbox{MinMS: 2 * 60_000} }

type inboxTOML struct {
	MinMinutes *int `toml:"min_minutes"`
}

// decodeInbox applies the keys present in f to c.
func decodeInbox(f inboxTOML, c *Inbox) error {
	if f.MinMinutes != nil {
		if v := *f.MinMinutes; v < 0 || v > 60 {
			return fmt.Errorf("[inbox] min_minutes = %d, want 0 to 60", v)
		}
		c.MinMS = int64(*f.MinMinutes) * 60_000
	}
	return nil
}
