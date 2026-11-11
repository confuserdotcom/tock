package config

import (
	"errors"
	"fmt"
	"strings"
)

// MaxSessionMin is the longest focus session, in minutes.
const MaxSessionMin = 240

// MaxSnoozeMin is the longest distraction snooze, in minutes.
const MaxSnoozeMin = 240

// Focus is the [focus] section: session presets, break and distraction
// timing, the daily recap and meeting detection.
type Focus struct {
	// Enabled is the Focus group: off, no sessions, guard, break or
	// distraction notices, and every Focus member is off.
	Enabled          bool
	Presets          []int // session lengths in minutes; Presets[0] is the default
	BreakAfterMS     int64
	BreakRepeatMS    int64
	DistractGraceMS  int64
	DistractRepeatMS int64
	// SnoozeMS is how long a snooze holds distraction notices back.
	SnoozeMS     int64
	AutoMinMS    int64
	RecapAt      int // minutes after local midnight; -1 = off
	Notify       bool
	MeetingApps  []string // bundle ids
	MeetingHosts []string // URL hosts; subdomains match too
	// Block removes distractions during a manual session: the front tab is
	// closed or the app hidden once it has been in front BlockGraceMS.
	Block        bool
	BlockApps    []string // bundle ids blocked besides distraction-kind time
	BlockHosts   []string // lowercase URL hosts; subdomains match too
	BlockGraceMS int64
	// FocusOn and FocusOff name Shortcuts run when a session starts and
	// ends; "" = none.
	FocusOn, FocusOff string
	// UrgeSurf turns the distraction notice into an urge-surfing prompt with
	// counted answers.
	UrgeSurf bool
	// NoticeBudget is how many nudges tock sends unasked in a day (break,
	// away, start, midday, overworking, suggestions); 0 = no limit.
	// NoticeGapMS is the least time between two of them; 0 = none.
	NoticeBudget int
	NoticeGapMS  int64
	// BreakScreen shows a full-screen notice when a break you took runs
	// BreakScreenGraceMS past its end and you are not back at deep work.
	// Esc or a button closes it; it never locks anything.
	BreakScreen        bool
	BreakScreenGraceMS int64
	// WakeScreen covers the screens when a session has gone WakeAfterMS off
	// task (WakeWarnMS gets a banner first). An idle stretch counts as off
	// task once it lasts WakeIdleMS.
	WakeScreen                          bool
	WakeWarnMS, WakeAfterMS, WakeIdleMS int64
	// Hotkey makes ⌥⌘F open the start prompt from any app.
	Hotkey bool
}

// DefaultFocus is the [focus] section used when the file does not set it.
// Every call returns fresh slices.
func DefaultFocus() Focus {
	return Focus{
		Presets:      []int{25, 50, 90},
		BreakAfterMS: 50 * 60_000, BreakRepeatMS: 10 * 60_000,
		DistractGraceMS: 60_000, DistractRepeatMS: 120_000, SnoozeMS: 10 * 60_000,
		AutoMinMS: 25 * 60_000, RecapAt: 21 * 60, Notify: true,
		MeetingApps: []string{"us.zoom.xos", "com.apple.FaceTime", "com.microsoft.teams2",
			"com.microsoft.teams", "Cisco-Systems.Spark", "com.cisco.webexmeetingsapp"},
		MeetingHosts: []string{"meet.google.com", "teams.microsoft.com", "app.zoom.us", "whereby.com"},
		BlockGraceMS: 10_000,
		NoticeBudget: 8, NoticeGapMS: 20 * 60_000,
		BreakScreenGraceMS: 5 * 60_000,
		WakeWarnMS:         5 * 60_000, WakeAfterMS: 10 * 60_000, WakeIdleMS: 15 * 60_000,
		Hotkey: true, Enabled: true,
	}
}

type focusTOML struct {
	Enabled        *bool     `toml:"enabled"`
	Presets        *[]int    `toml:"presets"`
	BreakAfter     *int      `toml:"break_after"`
	BreakRepeat    *int      `toml:"break_repeat"`
	DistractGrace  *int      `toml:"distract_grace"`
	DistractRepeat *int      `toml:"distract_repeat"`
	Snooze         *int      `toml:"snooze"`
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
	UrgeSurf       *bool     `toml:"urge_surf"`
	NoticeBudget   *int      `toml:"notice_budget"`
	NoticeGap      *int      `toml:"notice_gap"`
	BreakScreen    *bool     `toml:"break_screen"`
	BreakGrace     *int      `toml:"break_screen_grace"`
	WakeScreen     *bool     `toml:"wake_screen"`
	WakeWarn       *int      `toml:"wake_warn"`
	WakeAfter      *int      `toml:"wake_after"`
	WakeIdle       *int      `toml:"wake_idle"`
	Hotkey         *bool     `toml:"hotkey"`
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
		{"snooze", f.Snooze, 1, MaxSnoozeMin, 60_000, &c.SnoozeMS},
		{"distract_grace", f.DistractGrace, 1, 3600, 1000, &c.DistractGraceMS},
		{"distract_repeat", f.DistractRepeat, 1, 3600, 1000, &c.DistractRepeatMS},
		{"auto_min", f.AutoMin, 5, 240, 60_000, &c.AutoMinMS},
		{"block_grace", f.BlockGrace, 1, 600, 1000, &c.BlockGraceMS},
		{"break_screen_grace", f.BreakGrace, 1, 60, 60_000, &c.BreakScreenGraceMS},
		{"wake_warn", f.WakeWarn, 1, 60, 60_000, &c.WakeWarnMS},
		{"wake_after", f.WakeAfter, 2, 120, 60_000, &c.WakeAfterMS},
		{"wake_idle", f.WakeIdle, 1, 120, 60_000, &c.WakeIdleMS},
	} {
		if d.v == nil {
			continue
		}
		if *d.v < d.lo || *d.v > d.hi {
			return fmt.Errorf("[focus] %s = %d, want %d to %d", d.key, *d.v, d.lo, d.hi)
		}
		*d.dst = int64(*d.v) * d.unitMS
	}
	if v := f.NoticeBudget; v != nil {
		if *v < 0 || *v > 100 {
			return fmt.Errorf("[focus] notice_budget = %d, want 0 (no limit) to 100", *v)
		}
		c.NoticeBudget = *v
	}
	if v := f.NoticeGap; v != nil {
		if *v < 0 || *v > 240 {
			return fmt.Errorf("[focus] notice_gap = %d, want 0 (none) to 240", *v)
		}
		c.NoticeGapMS = int64(*v) * 60_000
	}
	if f.Recap != nil {
		at, ok := parseClock(*f.Recap)
		if !ok {
			return fmt.Errorf(`[focus] recap = %q, want "HH:MM" or ""`, *f.Recap)
		}
		c.RecapAt = at
	}
	if c.WakeWarnMS >= c.WakeAfterMS {
		return fmt.Errorf("[focus] wake_warn = %d must be less than wake_after = %d", c.WakeWarnMS/60_000, c.WakeAfterMS/60_000)
	}
	if f.BreakScreen != nil {
		c.BreakScreen = *f.BreakScreen
	}
	if f.WakeScreen != nil {
		c.WakeScreen = *f.WakeScreen
	}
	if f.Hotkey != nil {
		c.Hotkey = *f.Hotkey
	}
	if f.UrgeSurf != nil {
		c.UrgeSurf = *f.UrgeSurf
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
