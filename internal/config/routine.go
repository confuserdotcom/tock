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
	// End turns the end-of-day recap notice on; false turns it off whatever
	// [focus] recap says.
	End bool
	// Overwork turns the overworking notice on: one notice when work goes
	// on past [hours] end.
	Overwork bool
	// OverGraceMS is how much work after [hours] end brings the first
	// overworking notice; OverRepeatMS is the least time between two of
	// them. Both come from [hours] over_grace and over_repeat.
	OverGraceMS, OverRepeatMS int64
}

// DefaultRoutine is the [routine] section used when the file does not set it.
func DefaultRoutine() Routine {
	return Routine{Start: true, MiddayAt: 13 * 60, End: true, Overwork: true,
		OverGraceMS: 30 * 60_000, OverRepeatMS: 60 * 60_000}
}

type routineTOML struct {
	Start  *bool   `toml:"start"`
	Midday *string `toml:"midday"`
	End    *bool   `toml:"end"`
	// Overwork turns the overworking notice on or off.
	Overwork *bool `toml:"overwork"`
}

// decodeRoutine applies the keys present in f to c.
func decodeRoutine(f routineTOML, c *Routine) error {
	if f.Start != nil {
		c.Start = *f.Start
	}
	if f.End != nil {
		c.End = *f.End
	}
	if f.Overwork != nil {
		c.Overwork = *f.Overwork
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
