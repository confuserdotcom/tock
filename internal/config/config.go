// Package config reads the optional ~/.config/tock/config.toml.
package config

import (
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"time"

	"github.com/BurntSushi/toml"

	"tock/internal/paths"
	"tock/internal/stats"
)

// File is the config file name in tock's config directory.
const File = "config.toml"

// Config is tock's settings.
type Config struct {
	Score     stats.Weights
	Focus     Focus
	LLM       LLM
	Shell     Shell
	Away      Away
	AFK       AFK
	Calendar  Calendar
	Sync      Sync
	Slack     Slack
	Hours     stats.Hours // the work schedule the score is limited to
	Invoice   Invoice
	Privacy   Privacy
	Plan      Plan
	Routine   Routine
	Inbox     Inbox
	Recommend Recommend
	Model     Model
	OCR       OCR
	Notch     Notch
	Backup    Backup
	Billing   Billing
	// File is the config as config.toml sets it, before the groups turn
	// their members off; see Raw.
	File *Config
}

// Model is the [model] section: the local classifier that labels what the
// rules miss (see classify.Model). It never leaves the machine.
type Model struct {
	Enabled bool
}

// Privacy is the [privacy] section.
type Privacy struct {
	// KeepTitles is how many days window titles are kept; older spans
	// lose their title and the path of their URL. 0 = forever.
	KeepTitles int
	// HideApps (bundle ids) and HideSites (hosts, with their subdomains)
	// are tracked as time only, and are never sent to Claude or read by OCR.
	HideApps, HideSites []string
}

// Backup is the [backup] section: how many daily snapshots of tock.db
// trackd keeps in backups/auto. 0 turns daily snapshots off; a copy is
// still kept before each migration.
type Backup struct {
	Keep int
}

// MaxBackupKeep is the most daily snapshots kept.
const MaxBackupKeep = 365

// DefaultBackup keeps a week of daily snapshots.
func DefaultBackup() Backup { return Backup{Keep: 7} }

type backupTOML struct {
	Keep *int `toml:"keep"`
}

// Plan is the [plan] section: the todo.txt file the plan tab shows, and a
// command that tock runs when the tab opens, after each change it makes to
// the file and once a minute. The command may rewrite the file (a planner
// that rolls the day over); its stdout, JSON {"lines": [...], "seen":
// {"project": seconds}} or plain lines, is shown above the list.
type Plan struct {
	File    string // absolute; "" = no plan tab content
	Command string // run with sh -c; "" = none
	// Notify turns the up-next and starting-now notices of timed plan items
	// and calendar events on.
	Notify bool
	// NotifyBeforeMS is how far ahead the up-next notice comes; 0 = none.
	NotifyBeforeMS int64
	// AutoStart makes a module-tagged plan item start a focus session of its
	// slot length when it begins.
	AutoStart bool
}

// MaxKeepTitles is the longest keep_titles, in days.
const MaxKeepTitles = 3650

// Sync is the [sync] section: the folder shared with tock's other machines
// (synced by Syncthing or the like) and this machine's name in it.
type Sync struct {
	Dir     string // absolute; "" = off
	Machine string // "" = the host name
	// NoRules keeps rules.toml out of the sync: by default each machine
	// shares its copy and takes a newer one from another machine.
	NoRules bool
	// Update lets trackd on Linux install the builds the Mac publishes to
	// <dir>/bin, signed by a key pinned with tock update trust. Opt-in.
	Update bool
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
	AutoOpen  bool     // tockbar opens a starting event's meeting link by itself
}

// Away is the [away] section: which absences tock asks about.
type Away struct {
	MinMS, MaxMS int64
}

// DefaultAway is the [away] section used when the file does not set it.
func DefaultAway() Away { return Away{MinMS: 10 * 60_000, MaxMS: 240 * 60_000} }

// AFK is the [afk] section: when time in front of an app stops counting.
// The window per category is `afk` in rules.toml.
type AFK struct {
	// VideoMS is how long trackd waits without input while the front app
	// plays a video before it goes idle. trackd reads it at start.
	VideoMS int64
	// StaleMS: on a machine that reports no input (Hyprland without the
	// hypridle hook), one unchanged window longer than this keeps its
	// category's window and the rest is idle.
	StaleMS int64
	// TrustMS: on a machine whose input comes from hypridle, which can
	// miss idle, one unchanged window longer than this keeps its category's
	// window and the rest is idle, even while it reports input.
	TrustMS int64
}

// DefaultAFK is the [afk] section used when the file does not set it.
func DefaultAFK() AFK { return AFK{VideoMS: 180 * 60_000, StaleMS: 20 * 60_000, TrustMS: 180 * 60_000} }

// Shell is the [shell] section: which apps are terminals, so their views get
// the working directory the zsh hook records.
type Shell struct {
	Terminals []string // bundle ids
	// Agents: coding agents' session logs give the working directory too
	// (agents = false turns it off). Read on this machine only.
	Agents bool
}

// DefaultShell is the [shell] section used when the file does not set it.
// Every call returns a fresh slice.
func DefaultShell() Shell {
	return Shell{Agents: true, Terminals: []string{"com.github.wez.wezterm", "com.apple.Terminal", "com.googlecode.iterm2",
		"com.mitchellh.ghostty", "net.kovidgoyal.kitty", "org.alacritty", "dev.warp.Warp-Stable",
		// Hyprland window classes on Linux.
		"kitty", "org.wezfurlong.wezterm", "Alacritty", "foot"}}
}

// Default is the configuration used when there is no file.
func Default() Config {
	return finish(Config{Score: stats.DefaultWeights, Focus: DefaultFocus(), LLM: DefaultLLM(), Shell: DefaultShell(), Away: DefaultAway(), AFK: DefaultAFK(), Slack: DefaultSlack(), Routine: DefaultRoutine(), Inbox: DefaultInbox(),
		Recommend: DefaultRecommend(), Plan: DefaultPlan(), Model: Model{Enabled: true}, OCR: DefaultOCR(), Notch: DefaultNotch(), Backup: DefaultBackup(), Invoice: DefaultInvoice(),
		Billing: Billing{Enabled: true}})
}

type scoreTOML struct {
	Depth   *float64 `toml:"depth"`
	Blocks  *float64 `toml:"blocks"`
	Calm    *float64 `toml:"calm"`
	Stretch *float64 `toml:"stretch"`
}

type shellTOML struct {
	Terminals *[]string `toml:"terminals"`
	Agents    *bool     `toml:"agents"`
}

type awayTOML struct {
	Min *int `toml:"min"`
	Max *int `toml:"max"`
}

type afkTOML struct {
	Video *int `toml:"video"`
	Stale *int `toml:"stale"`
	Trust *int `toml:"trust"`
}

type calendarTOML struct {
	Enabled   *bool     `toml:"enabled"`
	Calendars *[]string `toml:"calendars"`
	AutoOpen  *bool     `toml:"auto_open"`
}

type syncTOML struct {
	Dir     *string `toml:"dir"`
	Machine *string `toml:"machine"`
	Rules   *bool   `toml:"rules"`
	Update  *bool   `toml:"update"`
}

type modelTOML struct {
	Enabled *bool `toml:"enabled"`
}

type privacyTOML struct {
	KeepTitles *int      `toml:"keep_titles"`
	HideApps   *[]string `toml:"hide_apps"`
	HideSites  *[]string `toml:"hide_sites"`
}

type planTOML struct {
	File         *string `toml:"file"`
	Command      *string `toml:"command"`
	Notify       *bool   `toml:"notify"`
	NotifyBefore *int    `toml:"notify_before"`
	AutoStart    *bool   `toml:"auto_start"`
}

type fileTOML struct {
	Score     scoreTOML     `toml:"score"`
	Focus     focusTOML     `toml:"focus"`
	LLM       llmTOML       `toml:"llm"`
	Shell     shellTOML     `toml:"shell"`
	Away      awayTOML      `toml:"away"`
	AFK       afkTOML       `toml:"afk"`
	Calendar  calendarTOML  `toml:"calendar"`
	Sync      syncTOML      `toml:"sync"`
	Slack     slackTOML     `toml:"slack"`
	Hours     hoursTOML     `toml:"hours"`
	Invoice   invoiceTOML   `toml:"invoice"`
	Privacy   privacyTOML   `toml:"privacy"`
	Plan      planTOML      `toml:"plan"`
	Routine   routineTOML   `toml:"routine"`
	Inbox     inboxTOML     `toml:"inbox"`
	Recommend recommendTOML `toml:"recommend"`
	Model     modelTOML     `toml:"model"`
	OCR       ocrTOML       `toml:"ocr"`
	Notch     notchTOML     `toml:"notch"`
	Backup    backupTOML    `toml:"backup"`
	Billing   billingTOML   `toml:"billing"`
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
	return Parse(name, b)
}

// Parse reads the settings in b; name labels the errors. An empty b is
// the defaults.
func Parse(name string, b []byte) (Config, error) {
	c := Default()
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
	if f.AFK.Video != nil {
		if v := *f.AFK.Video; v < 5 || v > 720 {
			return Default(), fmt.Errorf("%s: [afk] video = %d, want 5 to 720 minutes", name, v)
		}
		c.AFK.VideoMS = int64(*f.AFK.Video) * 60_000
	}
	if f.AFK.Stale != nil {
		if v := *f.AFK.Stale; v < 10 || v > 240 {
			return Default(), fmt.Errorf("%s: [afk] stale = %d, want 10 to 240 minutes", name, v)
		}
		c.AFK.StaleMS = int64(*f.AFK.Stale) * 60_000
	}
	if f.AFK.Trust != nil {
		if v := *f.AFK.Trust; v < 30 || v > 720 {
			return Default(), fmt.Errorf("%s: [afk] trust = %d, want 30 to 720 minutes", name, v)
		}
		c.AFK.TrustMS = int64(*f.AFK.Trust) * 60_000
	}
	if err := decodeHours(f.Hours, &c.Hours); err != nil {
		return Default(), fmt.Errorf("%s: %w", name, err)
	}
	if err := decodeOver(f.Hours, &c.Routine); err != nil {
		return Default(), fmt.Errorf("%s: %w", name, err)
	}
	if err := decodeRoutine(f.Routine, &c.Routine); err != nil {
		return Default(), fmt.Errorf("%s: %w", name, err)
	}
	if err := decodeInbox(f.Inbox, &c.Inbox); err != nil {
		return Default(), fmt.Errorf("%s: %w", name, err)
	}
	if err := decodeRecommend(f.Recommend, &c.Recommend); err != nil {
		return Default(), fmt.Errorf("%s: %w", name, err)
	}
	decodeNotch(f.Notch, &c.Notch)
	if err := decodeOCR(f.OCR, &c.OCR); err != nil {
		return Default(), fmt.Errorf("%s: %w", name, err)
	}
	if c.Hours.Set && f.Focus.Recap == nil {
		c.Focus.RecapAt = c.Hours.EndMin // the recap closes the work day
	}
	if !c.Routine.End {
		c.Focus.RecapAt = -1 // [routine] end = false: no end-of-day notice, RecapDue is never true
	}
	if f.Model.Enabled != nil {
		c.Model.Enabled = *f.Model.Enabled
	}
	if k := f.Privacy.KeepTitles; k != nil {
		if *k < 1 || *k > MaxKeepTitles {
			return Default(), fmt.Errorf("%s: [privacy] keep_titles = %d, want 1 to %d days", name, *k, MaxKeepTitles)
		}
		c.Privacy.KeepTitles = *k
	}
	if k := f.Backup.Keep; k != nil {
		if *k < 0 || *k > MaxBackupKeep {
			return Default(), fmt.Errorf("%s: [backup] keep = %d, want 0 to %d", name, *k, MaxBackupKeep)
		}
		c.Backup.Keep = *k
	}
	if err := decodePrivacy(f.Privacy, &c); err != nil {
		return Default(), fmt.Errorf("%s: %w", name, err)
	}
	if err := decodeInvoice(f.Invoice, &c.Invoice); err != nil {
		return Default(), fmt.Errorf("%s: %w", name, err)
	}
	if err := decodeSlack(f.Slack, &c.Slack); err != nil {
		return Default(), fmt.Errorf("%s: %w", name, err)
	}
	if f.Calendar.Enabled != nil {
		c.Calendar.Enabled = *f.Calendar.Enabled
	}
	if f.Calendar.AutoOpen != nil {
		c.Calendar.AutoOpen = *f.Calendar.AutoOpen
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
	if f.Shell.Agents != nil {
		c.Shell.Agents = *f.Shell.Agents
	}
	if f.Sync.Dir != nil && strings.TrimSpace(*f.Sync.Dir) != "" {
		d, err := absPath(*f.Sync.Dir)
		if err != nil {
			return Default(), fmt.Errorf("%s: [sync] dir = %q, want an absolute path or ~/…", name, *f.Sync.Dir)
		}
		c.Sync.Dir = d
	}
	if f.Plan.File != nil && strings.TrimSpace(*f.Plan.File) != "" {
		p, err := absPath(*f.Plan.File)
		if err != nil {
			return Default(), fmt.Errorf("%s: [plan] file: %w", name, err)
		}
		c.Plan.File = p
	}
	if f.Plan.Command != nil {
		c.Plan.Command = strings.TrimSpace(*f.Plan.Command)
	}
	if err := decodePlanNotices(f.Plan, &c.Plan); err != nil {
		return Default(), fmt.Errorf("%s: %w", name, err)
	}
	if f.Sync.Machine != nil {
		if !machineRE.MatchString(*f.Sync.Machine) {
			return Default(), fmt.Errorf("%s: [sync] machine = %q, want lowercase letters, digits and -, at most 32", name, *f.Sync.Machine)
		}
		if *f.Sync.Machine == "bin" { // the folder the builds are published to
			return Default(), fmt.Errorf("%s: [sync] machine = %q is taken by the published builds; pick another name", name, *f.Sync.Machine)
		}
		c.Sync.Machine = *f.Sync.Machine
	}
	if f.Sync.Rules != nil {
		c.Sync.NoRules = !*f.Sync.Rules
	}
	if f.Sync.Update != nil {
		c.Sync.Update = *f.Sync.Update
	}
	if f.Focus.Enabled != nil {
		c.Focus.Enabled = *f.Focus.Enabled
	}
	if f.Routine.Enabled != nil {
		c.Routine.Enabled = *f.Routine.Enabled
	}
	if f.Billing.Enabled != nil {
		c.Billing.Enabled = *f.Billing.Enabled
	}
	return finish(c), nil
}

// hostRE is a URL host as block_hosts takes it.
var hostRE = regexp.MustCompile(`^[a-z0-9]([a-z0-9-]*[a-z0-9])?(\.[a-z0-9]([a-z0-9-]*[a-z0-9])?)*$`)

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

// absPath expands a leading ~/ (and on Windows ~\ and %VAR%) and requires
// an absolute result.
func absPath(p string) (string, error) {
	d := p
	if strings.HasPrefix(d, "~") || strings.HasPrefix(d, "%") {
		home := paths.Home()
		if home == "" {
			return "", errors.New("no home directory")
		}
		d = paths.Expand(runtime.GOOS, os.Getenv, home, d)
	}
	if !filepath.IsAbs(d) {
		return "", fmt.Errorf("%q, want an absolute path or ~/…", p)
	}
	return filepath.Clean(d), nil
}
