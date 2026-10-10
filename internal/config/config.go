// Package config reads the optional ~/.config/tock/config.toml.
package config

import (
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/BurntSushi/toml"

	"tock/internal/stats"
)

// File is the config file name in tock's config directory.
const File = "config.toml"

// Config is tock's settings.
type Config struct {
	Score    stats.Weights
	Focus    Focus
	LLM      LLM
	Shell    Shell
	Away     Away
	Calendar Calendar
	Sync     Sync
	Slack    Slack
}

// Slack is the [slack] section: the status and do-not-disturb tock sets in
// Slack while a focus session runs.
type Slack struct {
	Enabled bool
	Status  string // status text
	Emoji   string // ":name:"
	DND     bool   // snooze notifications for the session
}

// DefaultSlack is the [slack] section used when the file does not set it.
func DefaultSlack() Slack {
	return Slack{Status: "In a focus session", Emoji: ":dart:", DND: true}
}

// Sync is the [sync] section: the folder shared with tock's other machines
// (synced by Syncthing or the like) and this machine's name in it.
type Sync struct {
	Dir     string // absolute; "" = off
	Machine string // "" = the host name
}

// machineRE is a valid machine name: it becomes a folder name.
var machineRE = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,31}$`)

// MachineName is this machine's name in the sync folder: the configured
// one, else the short host name, lowercased and reduced to a valid name.
func (s Sync) MachineName() string {
	if s.Machine != "" {
		return s.Machine
	}
	h, _ := os.Hostname()
	h, _, _ = strings.Cut(strings.ToLower(h), ".")
	var b strings.Builder
	for _, r := range h {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
		case b.Len() > 0:
			b.WriteRune('-')
		}
	}
	name := strings.Trim(b.String(), "-")
	if len(name) > 32 {
		name = name[:32]
	}
	if !machineRE.MatchString(name) {
		return "this-machine"
	}
	return name
}

// Calendar is the [calendar] section: whether trackd reads calendar events
// through EventKit, and from which calendars.
type Calendar struct {
	Enabled   bool
	Calendars []string // calendar names; empty = all
}

// Away is the [away] section: which absences tock asks about.
type Away struct {
	MinMS, MaxMS int64
}

// DefaultAway is the [away] section used when the file does not set it.
func DefaultAway() Away { return Away{MinMS: 10 * 60_000, MaxMS: 240 * 60_000} }

// Shell is the [shell] section: which apps are terminals, so their views get
// the working directory the zsh hook records.
type Shell struct {
	Terminals []string // bundle ids
}

// DefaultShell is the [shell] section used when the file does not set it.
// Every call returns a fresh slice.
func DefaultShell() Shell {
	return Shell{Terminals: []string{"com.github.wez.wezterm", "com.apple.Terminal", "com.googlecode.iterm2",
		"com.mitchellh.ghostty", "net.kovidgoyal.kitty", "org.alacritty", "dev.warp.Warp-Stable",
		// Hyprland window classes on Linux.
		"kitty", "org.wezfurlong.wezterm", "Alacritty", "foot"}}
}

// MaxSessionMin is the longest focus session, in minutes.
const MaxSessionMin = 240

// Focus is the [focus] section: session presets, break and distraction
// timing, the daily recap and meeting detection.
type Focus struct {
	Presets          []int // session lengths in minutes; Presets[0] is the default
	BreakAfterMS     int64
	BreakRepeatMS    int64
	DistractGraceMS  int64
	DistractRepeatMS int64
	AutoMinMS        int64
	RecapAt          int // minutes after local midnight; -1 = off
	Notify           bool
	MeetingApps      []string // bundle ids
	MeetingHosts     []string // URL hosts; subdomains match too
	// Block removes distractions during a manual session: the front tab is
	// closed or the app hidden once it has been in front BlockGraceMS.
	Block        bool
	BlockApps    []string // bundle ids blocked besides distraction-kind time
	BlockHosts   []string // lowercase URL hosts; subdomains match too
	BlockGraceMS int64
	// FocusOn and FocusOff name Shortcuts run when a session starts and
	// ends; "" = none.
	FocusOn, FocusOff string
}

// DefaultFocus is the [focus] section used when the file does not set it.
// Every call returns fresh slices.
func DefaultFocus() Focus {
	return Focus{
		Presets:      []int{25, 50, 90},
		BreakAfterMS: 50 * 60_000, BreakRepeatMS: 10 * 60_000,
		DistractGraceMS: 60_000, DistractRepeatMS: 120_000,
		AutoMinMS: 25 * 60_000, RecapAt: 21 * 60, Notify: true,
		MeetingApps: []string{"us.zoom.xos", "com.apple.FaceTime", "com.microsoft.teams2",
			"com.microsoft.teams", "Cisco-Systems.Spark", "com.cisco.webexmeetingsapp"},
		MeetingHosts: []string{"meet.google.com", "teams.microsoft.com", "app.zoom.us", "whereby.com"},
		BlockGraceMS: 10_000,
	}
}

// lists hold the defaults followed by the user's additions.
type LLM struct {
	Enabled    bool
	Model      string
	MinMS      int64    // least inbox time a key needs before it is sent
	DenyApps   []string // bundle ids never sent
	DenyHosts  []string // RE2 patterns against the URL host
	DenyTitles []string // RE2 patterns, case-insensitive, against the title text
}

// DefaultLLM is the [llm] section used when the file does not set it. Every
// call returns fresh slices.
func DefaultLLM() LLM {
	return LLM{
		Enabled: true, Model: "claude-haiku-5-5", MinMS: 2 * 60_000,
		DenyApps: []string{
			"com.1password.1password", "com.agilebits.onepassword7", "com.bitwarden.desktop",
			"com.apple.keychainaccess", "com.apple.Passwords",
			"com.apple.mail", "com.microsoft.Outlook", "com.readdle.smartemail-Mac",
			"com.apple.MobileSMS", "net.whatsapp.WhatsApp", "org.whispersystems.signal-desktop",
			"ru.keepcoder.Telegram", "com.tinyspeck.slackmacgap",
		},
		DenyHosts: []string{
			`(^|\.)(bank|banque|paypal|revolut|wise|boursorama|lcl|credit-agricole|societegenerale|bnpparibas|n26|qonto)(\.|$)`,
			`(^|\.)(mail\.google\.com|outlook\.(live|office)\.com|proton\.me)$`,
		},
	}
}

// Default is the configuration used when there is no file.
func Default() Config {
	return Config{Score: stats.DefaultWeights, Focus: DefaultFocus(), LLM: DefaultLLM(), Shell: DefaultShell(), Away: DefaultAway(), Slack: DefaultSlack()}
}

type scoreTOML struct {
	Depth   *float64 `toml:"depth"`
	Blocks  *float64 `toml:"blocks"`
	Calm    *float64 `toml:"calm"`
	Stretch *float64 `toml:"stretch"`
}

type focusTOML struct {
	Presets        *[]int    `toml:"presets"`
	BreakAfter     *int      `toml:"break_after"`
	BreakRepeat    *int      `toml:"break_repeat"`
	DistractGrace  *int      `toml:"distract_grace"`
	DistractRepeat *int      `toml:"distract_repeat"`
	AutoMin        *int      `toml:"auto_min"`
	Recap          *string   `toml:"recap"`
	Notify         *bool     `toml:"notify"`
	MeetingApps    *[]string `toml:"meeting_apps"`
	MeetingHosts   *[]string `toml:"meeting_hosts"`
	Block          *bool     `toml:"block"`
	BlockApps      *[]string `toml:"block_apps"`
	BlockHosts     *[]string `toml:"block_hosts"`
	BlockGrace     *int      `toml:"block_grace"`
	FocusOn        *string   `toml:"focus_on"`
	FocusOff       *string   `toml:"focus_off"`
}

type slackTOML struct {
	Enabled *bool   `toml:"enabled"`
	Status  *string `toml:"status"`
	Emoji   *string `toml:"emoji"`
	DND     *bool   `toml:"dnd"`
}

type llmTOML struct {
	Enabled    *bool     `toml:"enabled"`
	Model      *string   `toml:"model"`
	MinMinutes *int      `toml:"min_minutes"`
	DenyApps   *[]string `toml:"deny_apps"`
	DenyHosts  *[]string `toml:"deny_hosts"`
	DenyTitles *[]string `toml:"deny_titles"`
}

type shellTOML struct {
	Terminals *[]string `toml:"terminals"`
}

type awayTOML struct {
	Min *int `toml:"min"`
	Max *int `toml:"max"`
}

type calendarTOML struct {
	Enabled   *bool     `toml:"enabled"`
	Calendars *[]string `toml:"calendars"`
}

type syncTOML struct {
	Dir     *string `toml:"dir"`
	Machine *string `toml:"machine"`
}

type fileTOML struct {
	Score    scoreTOML    `toml:"score"`
	Focus    focusTOML    `toml:"focus"`
	LLM      llmTOML      `toml:"llm"`
	Shell    shellTOML    `toml:"shell"`
	Away     awayTOML     `toml:"away"`
	Calendar calendarTOML `toml:"calendar"`
	Sync     syncTOML     `toml:"sync"`
	Slack    slackTOML    `toml:"slack"`
}

// Load reads path. A missing file is Default() and no error. Any other
// problem returns Default() and an error naming the file.
func Load(path string) (Config, error) {
	c := Default()
	b, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return c, nil
	}
	name := filepath.Base(path)
	if err != nil {
		return Default(), fmt.Errorf("%s: %w", name, err)
	}
	var f fileTOML
	md, err := toml.Decode(string(b), &f)
	if err != nil {
		return Default(), fmt.Errorf("%s: %w", name, err)
	}
	if un := md.Undecoded(); len(un) > 0 {
		keys := make([]string, len(un))
		for i, k := range un {
			keys[i] = k.String()
		}
		return Default(), fmt.Errorf("%s: unknown key %s", name, strings.Join(keys, ", "))
	}
	for _, w := range []struct {
		key string
		v   *float64
		dst *float64
	}{{"depth", f.Score.Depth, &c.Score.Depth}, {"blocks", f.Score.Blocks, &c.Score.Blocks}, {"calm", f.Score.Calm, &c.Score.Calm},
		{"stretch", f.Score.Stretch, &c.Score.Stretch}} {
		if w.v == nil {
			continue
		}
		if *w.v < 0 || *w.v > 1 || math.IsNaN(*w.v) {
			return Default(), fmt.Errorf("%s: [score] %s = %g, want 0 to 1", name, w.key, *w.v)
		}
		*w.dst = *w.v
	}
	if sum := c.Score.Depth + c.Score.Blocks + c.Score.Calm + c.Score.Stretch; math.Abs(sum-1) > 0.001 {
		return Default(), fmt.Errorf("%s: [score] weights sum to %.2f, want 1", name, sum)
	}
	if err := decodeFocus(f.Focus, &c.Focus); err != nil {
		return Default(), fmt.Errorf("%s: %w", name, err)
	}
	if err := decodeLLM(f.LLM, &c.LLM); err != nil {
		return Default(), fmt.Errorf("%s: %w", name, err)
	}
	if f.Away.Min != nil {
		c.Away.MinMS = int64(*f.Away.Min) * 60_000
	}
	if f.Away.Max != nil {
		c.Away.MaxMS = int64(*f.Away.Max) * 60_000
	}
	switch mn, mx := c.Away.MinMS/60_000, c.Away.MaxMS/60_000; {
	case mn < 5:
		return Default(), fmt.Errorf("%s: [away] min = %d, want at least 5", name, mn)
	case mx <= mn || mx > 720:
		return Default(), fmt.Errorf("%s: [away] max = %d, want more than min (%d) and at most 720", name, mx, mn)
	}
	if err := decodeSlack(f.Slack, &c.Slack); err != nil {
		return Default(), fmt.Errorf("%s: %w", name, err)
	}
	if f.Calendar.Enabled != nil {
		c.Calendar.Enabled = *f.Calendar.Enabled
	}
	if f.Calendar.Calendars != nil {
		for _, n := range *f.Calendar.Calendars {
			if strings.TrimSpace(n) == "" {
				return Default(), fmt.Errorf("%s: [calendar] calendars has an empty name", name)
			}
		}
		c.Calendar.Calendars = append([]string{}, *f.Calendar.Calendars...)
	}
	if f.Shell.Terminals != nil {
		c.Shell.Terminals = append([]string{}, *f.Shell.Terminals...)
	}
	if f.Sync.Dir != nil && strings.TrimSpace(*f.Sync.Dir) != "" {
		d := *f.Sync.Dir
		if rest, ok := strings.CutPrefix(d, "~/"); ok {
			home, err := os.UserHomeDir()
			if err != nil {
				return Default(), fmt.Errorf("%s: [sync] dir: %w", name, err)
			}
			d = filepath.Join(home, rest)
		}
		if !filepath.IsAbs(d) {
			return Default(), fmt.Errorf("%s: [sync] dir = %q, want an absolute path or ~/…", name, *f.Sync.Dir)
		}
		c.Sync.Dir = filepath.Clean(d)
	}
	if f.Sync.Machine != nil {
		if !machineRE.MatchString(*f.Sync.Machine) {
			return Default(), fmt.Errorf("%s: [sync] machine = %q, want lowercase letters, digits and -, at most 32", name, *f.Sync.Machine)
		}
		c.Sync.Machine = *f.Sync.Machine
	}
	return c, nil
}

// decodeFocus applies the keys present in f to c and validates them.
func decodeFocus(f focusTOML, c *Focus) error {
	if f.Presets != nil {
		if len(*f.Presets) == 0 {
			return errors.New("[focus] presets is empty, want at least one length")
		}
		for _, p := range *f.Presets {
			if p < 1 || p > MaxSessionMin {
				return fmt.Errorf("[focus] presets has %d, want 1 to %d minutes", p, MaxSessionMin)
			}
		}
		c.Presets = append([]int{}, *f.Presets...)
	}
	for _, d := range []struct {
		key    string
		v      *int
		lo, hi int
		unitMS int64
		dst    *int64
	}{
		{"break_after", f.BreakAfter, 1, 480, 60_000, &c.BreakAfterMS},
		{"break_repeat", f.BreakRepeat, 1, 240, 60_000, &c.BreakRepeatMS},
		{"distract_grace", f.DistractGrace, 1, 3600, 1000, &c.DistractGraceMS},
		{"distract_repeat", f.DistractRepeat, 1, 3600, 1000, &c.DistractRepeatMS},
		{"auto_min", f.AutoMin, 5, 240, 60_000, &c.AutoMinMS},
		{"block_grace", f.BlockGrace, 1, 600, 1000, &c.BlockGraceMS},
	} {
		if d.v == nil {
			continue
		}
		if *d.v < d.lo || *d.v > d.hi {
			return fmt.Errorf("[focus] %s = %d, want %d to %d", d.key, *d.v, d.lo, d.hi)
		}
		*d.dst = int64(*d.v) * d.unitMS
	}
	if f.Recap != nil {
		at, ok := parseClock(*f.Recap)
		if !ok {
			return fmt.Errorf(`[focus] recap = %q, want "HH:MM" or ""`, *f.Recap)
		}
		c.RecapAt = at
	}
	if f.Notify != nil {
		c.Notify = *f.Notify
	}
	if f.MeetingApps != nil {
		c.MeetingApps = append([]string{}, *f.MeetingApps...)
	}
	if f.MeetingHosts != nil {
		c.MeetingHosts = append([]string{}, *f.MeetingHosts...)
	}
	if f.Block != nil {
		c.Block = *f.Block
	}
	if f.BlockApps != nil {
		c.BlockApps = append([]string{}, *f.BlockApps...)
	}
	if f.BlockHosts != nil {
		c.BlockHosts = nil
		for _, h := range *f.BlockHosts {
			h = strings.ToLower(strings.TrimSpace(h))
			if !hostRE.MatchString(h) {
				return fmt.Errorf("[focus] block_hosts has %q, want a host such as youtube.com", h)
			}
			c.BlockHosts = append(c.BlockHosts, h)
		}
	}
	if f.FocusOn != nil {
		c.FocusOn = strings.TrimSpace(*f.FocusOn)
	}
	if f.FocusOff != nil {
		c.FocusOff = strings.TrimSpace(*f.FocusOff)
	}
	return nil
}

// hostRE is a URL host as block_hosts takes it.
var hostRE = regexp.MustCompile(`^[a-z0-9]([a-z0-9-]*[a-z0-9])?(\.[a-z0-9]([a-z0-9-]*[a-z0-9])?)*$`)

// emojiRE is a Slack emoji code.
var emojiRE = regexp.MustCompile(`^:[a-z0-9_+'-]+:$`)

// decodeSlack applies the keys present in f to c and validates them.
func decodeSlack(f slackTOML, c *Slack) error {
	if f.Enabled != nil {
		c.Enabled = *f.Enabled
	}
	if f.Status != nil {
		s := strings.TrimSpace(*f.Status)
		if s == "" || len([]rune(s)) > 100 {
			return fmt.Errorf("[slack] status = %q, want 1 to 100 characters", *f.Status)
		}
		c.Status = s
	}
	if f.Emoji != nil {
		if !emojiRE.MatchString(*f.Emoji) {
			return fmt.Errorf(`[slack] emoji = %q, want a code such as ":dart:"`, *f.Emoji)
		}
		c.Emoji = *f.Emoji
	}
	if f.DND != nil {
		c.DND = *f.DND
	}
	return nil
}

// decodeLLM applies the keys present in f to c and validates them. Lists
// are appended to the defaults.
func decodeLLM(f llmTOML, c *LLM) error {
	if f.Enabled != nil {
		c.Enabled = *f.Enabled
	}
	if f.Model != nil {
		if strings.TrimSpace(*f.Model) == "" {
			return errors.New("[llm] model is empty")
		}
		c.Model = *f.Model
	}
	if f.MinMinutes != nil {
		if *f.MinMinutes < 1 || *f.MinMinutes > 600 {
			return fmt.Errorf("[llm] min_minutes = %d, want 1 to 600", *f.MinMinutes)
		}
		c.MinMS = int64(*f.MinMinutes) * 60_000
	}
	if f.DenyApps != nil {
		c.DenyApps = append(append([]string{}, c.DenyApps...), *f.DenyApps...)
	}
	for _, l := range []struct {
		key, prefix string
		v, dst      *[]string
	}{
		{"deny_hosts", "", f.DenyHosts, &c.DenyHosts},
		{"deny_titles", "(?i)", f.DenyTitles, &c.DenyTitles},
	} {
		if l.v == nil {
			continue
		}
		for _, p := range *l.v {
			if _, err := regexp.Compile(l.prefix + p); err != nil {
				return fmt.Errorf("[llm] %s has %q: %v", l.key, p, err)
			}
		}
		*l.dst = append(append([]string{}, *l.dst...), *l.v...)
	}
	return nil
}

// parseClock reads "HH:MM" as minutes after midnight; "" is -1 (off).
func parseClock(s string) (int, bool) {
	if s == "" {
		return -1, true
	}
	t, err := time.Parse("15:04", s)
	if err != nil || len(s) != 5 {
		return 0, false
	}
	return t.Hour()*60 + t.Minute(), true
}
