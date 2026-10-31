package config

import "fmt"

// Routine is the [routine] section: which daily routine notices trackd
// sends.
type Routine struct {
	// Start turns the start-of-day notice on.
	Start bool
	// MiddayAt is the midday check-in time, minutes after local midnight;
	// -1 = off.
	MiddayAt int
}

// DefaultRoutine is the [routine] section used when the file does not set it.
func DefaultRoutine() Routine { return Routine{Start: true, MiddayAt: 13 * 60} }

type routineTOML struct {
	Start  *bool   `toml:"start"`
	Midday *string `toml:"midday"`
}

// decodeRoutine applies the keys present in f to c.
func decodeRoutine(f routineTOML, c *Routine) error {
	if f.Start != nil {
		c.Start = *f.Start
	}
	if f.Midday != nil {
		v := *f.Midday
		if v == "off" {
			c.MiddayAt = -1
		} else if m, ok := parseClock(v); ok && m >= 0 {
			c.MiddayAt = m
		} else {
			return fmt.Errorf("[routine] midday = %q, want \"HH:MM\" or \"off\"", v)
		}
	}
	return nil
}
