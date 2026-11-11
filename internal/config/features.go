package config

// Feature is one setting tock choice turns on and off: the bool key Key
// in [Table] of config.toml. A group (IsGroup) turns its members off
// without touching their keys; a member names its group in Needs.
type Feature struct {
	ID      string // the name `tock choice <id> on|off` takes
	Group   string // the heading it is listed under
	Name    string
	Note    string // one line on what it does
	Table   string
	Key     string
	On      func(Config) bool
	IsGroup bool
	Needs   string // the group feature that must be on too; "" = none
}

// Features are the toggles, in the order tock choice lists them: each
// heading's group toggle, if it has one, comes first.
var Features = []Feature{
	{ID: "model", Group: "General", Name: "Local model labels", Note: "labels activity on this machine", Table: "model", Key: "enabled",
		On: func(c Config) bool { return c.Model.Enabled }},
	{ID: "notify", Group: "General", Name: "Notifications", Note: "every notice tock sends", Table: "focus", Key: "notify",
		On: func(c Config) bool { return c.Focus.Notify }},
	{ID: "notch", Group: "General", Name: "Notch", Note: "sessions, recaps and labelling in the notch", Table: "notch", Key: "enabled",
		On: func(c Config) bool { return c.Notch.Enabled }},

	{ID: "focus", Group: "Focus", Name: "Focus", Note: "sessions, the focus score, break and distraction notices", Table: "focus", Key: "enabled",
		On: func(c Config) bool { return c.Focus.Enabled }, IsGroup: true},
	{ID: "recommend", Group: "Focus", Name: "Suggestions", Note: "when to start focus or take a break", Table: "recommend", Key: "enabled",
		On: func(c Config) bool { return c.Recommend.Enabled }, Needs: "focus"},
	{ID: "break_screen", Group: "Focus", Name: "Break screen", Note: "covers the screens when a break runs over", Table: "focus", Key: "break_screen",
		On: func(c Config) bool { return c.Focus.BreakScreen }, Needs: "focus"},
	{ID: "wake_screen", Group: "Focus", Name: "Wake screen", Note: "covers the screens when a session goes off task", Table: "focus", Key: "wake_screen",
		On: func(c Config) bool { return c.Focus.WakeScreen }, Needs: "focus"},
	{ID: "hotkey", Group: "Focus", Name: "Start hotkey", Note: "⌥⌘F opens a prompt to start focus from any app", Table: "focus", Key: "hotkey",
		On: func(c Config) bool { return c.Focus.Hotkey }, Needs: "focus"},
	{ID: "block", Group: "Focus", Name: "Block distractions", Note: "closes distracting tabs and apps in a session", Table: "focus", Key: "block",
		On: func(c Config) bool { return c.Focus.Block }, Needs: "focus"},
	{ID: "urge_surf", Group: "Focus", Name: "Urge surfing", Note: "the distraction notice asks what you feel", Table: "focus", Key: "urge_surf",
		On: func(c Config) bool { return c.Focus.UrgeSurf }, Needs: "focus"},

	{ID: "routine", Group: "Day routine", Name: "Day routine", Note: "notices that frame the work day", Table: "routine", Key: "enabled",
		On: func(c Config) bool { return c.Routine.Enabled }, IsGroup: true},
	{ID: "start", Group: "Day routine", Name: "Start of day", Note: "a notice when your day starts", Table: "routine", Key: "start",
		On: func(c Config) bool { return c.Routine.Start }, Needs: "routine"},
	{ID: "recap", Group: "Day routine", Name: "Evening recap", Note: "the day in one notice", Table: "routine", Key: "end",
		On: func(c Config) bool { return c.Routine.End }, Needs: "routine"},
	{ID: "overwork", Group: "Day routine", Name: "Overworking", Note: "a notice when work runs past [hours] end", Table: "routine", Key: "overwork",
		On: func(c Config) bool { return c.Routine.Overwork }, Needs: "routine"},
	{ID: "plan", Group: "Day routine", Name: "Plan reminders", Note: "up next and starting now, for timed plan items", Table: "plan", Key: "notify",
		On: func(c Config) bool { return c.Plan.Notify }, Needs: "routine"},

	{ID: "billing", Group: "Billing", Name: "Billing", Note: "rates, invoices and priced exports", Table: "billing", Key: "enabled",
		On: func(c Config) bool { return c.Billing.Enabled }, IsGroup: true},

	{ID: "claude", Group: "AI", Name: "Claude", Note: "opt-in; sends app, site and title snippets to Anthropic", Table: "llm", Key: "enabled",
		On: func(c Config) bool { return c.LLM.Enabled }},

	{ID: "calendar", Group: "Integrations", Name: "Calendar meetings", Note: "events from macOS Calendar explain idle time", Table: "calendar", Key: "enabled",
		On: func(c Config) bool { return c.Calendar.Enabled }},
	{ID: "ocr", Group: "Integrations", Name: "Screen text", Note: "reads the front window; needs Screen Recording", Table: "ocr", Key: "enabled",
		On: func(c Config) bool { return c.OCR.Enabled }},
	{ID: "slack", Group: "Integrations", Name: "Slack status", Note: "status and do-not-disturb during sessions", Table: "slack", Key: "enabled",
		On: func(c Config) bool { return c.Slack.Enabled }, Needs: "focus"},
}

// Set is whether config.toml turns f on, whatever its group says.
func (f Feature) Set(c Config) bool { return f.On(c.Raw()) }

// Waits is whether config.toml turns f on but its group is off.
func (f Feature) Waits(c Config) bool {
	if f.Needs == "" || !f.Set(c) {
		return false
	}
	g := FeatureByID(f.Needs)
	return g != nil && !g.On(c.Raw())
}

// State is "on", "off" or "on (waits for <group>)".
func (f Feature) State(c Config) string {
	switch {
	case f.Waits(c):
		return "on (waits for " + FeatureByID(f.Needs).Name + ")"
	case f.Set(c):
		return "on"
	}
	return "off"
}

// FeatureByID is the feature with id, or nil.
func FeatureByID(id string) *Feature {
	for i := range Features {
		if Features[i].ID == id {
			return &Features[i]
		}
	}
	return nil
}
