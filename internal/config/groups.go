package config

// Billing is the [billing] section: the billing group. Off hides rates,
// invoice issuing and priced exports; issued invoices stay readable.
type Billing struct {
	Enabled bool
}

type billingTOML struct {
	Enabled *bool `toml:"enabled"`
}

// Raw is the config as the file sets it, before the groups turn their
// members off: what tock choice shows and writes. A Config not made by
// Default or Parse is its own Raw.
func (c Config) Raw() Config {
	if c.File == nil {
		return c
	}
	return *c.File
}

// withoutFile is c with no file values attached.
func (c Config) withoutFile() Config {
	c.File = nil
	return c
}

// finish keeps c's values as the file values and applies the groups: a
// group that is off turns every member off. Nothing in the file changes.
func finish(c Config) Config {
	raw := c.withoutFile()
	c.File = &raw
	if !c.Focus.Enabled {
		c.Focus.BreakScreen, c.Focus.WakeScreen, c.Focus.Hotkey = false, false, false
		c.Focus.Block, c.Focus.UrgeSurf = false, false
		c.Recommend.Enabled = false
		c.Slack.Enabled = false  // its status follows sessions
		c.Plan.AutoStart = false // it starts sessions
	}
	if !c.Routine.Enabled {
		c.Routine.Start, c.Routine.Midday, c.Routine.End, c.Routine.Overwork = false, false, false, false
		c.Plan.Notify = false
		c.Focus.RecapAt = -1
	}
	return c
}
