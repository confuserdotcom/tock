package config

// Feature is one setting tock choice turns on and off: the bool key Key
// in [Table] of config.toml.
type Feature struct {
	ID    string // the name `tock choice <id> on|off` takes
	Group string
	Name  string
	Note  string // one line on what it does
	Table string
	Key   string
	On    func(Config) bool
}

// Features are the toggles, in the order tock choice lists them.
var Features = []Feature{
	{"model", "Tracking", "Local model labels", "labels activity on this machine", "model", "enabled",
		func(c Config) bool { return c.Model.Enabled }},
	{"claude", "Tracking", "Claude", "suggestions, summaries and tock ask", "llm", "enabled",
		func(c Config) bool { return c.LLM.Enabled }},
	{"ocr", "Tracking", "Screen text", "reads the front window; needs Screen Recording", "ocr", "enabled",
		func(c Config) bool { return c.OCR.Enabled }},
	{"calendar", "Tracking", "Calendar meetings", "events from macOS Calendar explain idle time", "calendar", "enabled",
		func(c Config) bool { return c.Calendar.Enabled }},
	{"notify", "Focus", "Notifications", "every notice tock sends", "focus", "notify",
		func(c Config) bool { return c.Focus.Notify }},
	{"recommend", "Focus", "Suggestions", "when to start focus or take a break", "recommend", "enabled",
		func(c Config) bool { return c.Recommend.Enabled }},
	{"break_screen", "Focus", "Break screen", "covers the screens when a break runs over", "focus", "break_screen",
		func(c Config) bool { return c.Focus.BreakScreen }},
	{"wake_screen", "Focus", "Wake screen", "covers the screens when a session goes off task", "focus", "wake_screen",
		func(c Config) bool { return c.Focus.WakeScreen }},
	{"hotkey", "Focus", "Start hotkey", "⌥⌘F opens a prompt to start focus from any app", "focus", "hotkey",
		func(c Config) bool { return c.Focus.Hotkey }},
	{"block", "Focus", "Block distractions", "closes distracting tabs and apps in a session", "focus", "block",
		func(c Config) bool { return c.Focus.Block }},
	{"urge_surf", "Focus", "Urge surfing", "the distraction notice asks what you feel", "focus", "urge_surf",
		func(c Config) bool { return c.Focus.UrgeSurf }},
	{"start", "Day", "Start of day", "a notice when your day starts", "routine", "start",
		func(c Config) bool { return c.Routine.Start }},
	{"recap", "Day", "Evening recap", "the day in one notice", "routine", "end",
		func(c Config) bool { return c.Routine.End }},
	{"overwork", "Day", "Overworking", "a notice when work runs past [hours] end", "routine", "overwork",
		func(c Config) bool { return c.Routine.Overwork }},
	{"plan", "Day", "Plan reminders", "up next and starting now, for timed plan items", "plan", "notify",
		func(c Config) bool { return c.Plan.Notify }},
	{"slack", "Other", "Slack status", "status and do-not-disturb during sessions", "slack", "enabled",
		func(c Config) bool { return c.Slack.Enabled }},
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
