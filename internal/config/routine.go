package config

// Routine is the [routine] section: which daily routine notices trackd
// sends.
type Routine struct {
	// Start turns the start-of-day notice on.
	Start bool
}

// DefaultRoutine is the [routine] section used when the file does not set it.
func DefaultRoutine() Routine { return Routine{Start: true} }

type routineTOML struct {
	Start *bool `toml:"start"`
}

// decodeRoutine applies the keys present in f to c.
func decodeRoutine(f routineTOML, c *Routine) error {
	if f.Start != nil {
		c.Start = *f.Start
	}
	return nil
}
