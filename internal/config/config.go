// Package config reads the optional ~/.config/tock/config.toml.
package config

import (
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/BurntSushi/toml"

	"tock/internal/stats"
)

// File is the config file name in tock's config directory.
const File = "config.toml"

// Config is tock's settings.
type Config struct {
	Score stats.Weights
	Focus Focus
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
	}
}

// Default is the configuration used when there is no file.
func Default() Config { return Config{Score: stats.DefaultWeights, Focus: DefaultFocus()} }

type scoreTOML struct {
	Depth  *float64 `toml:"depth"`
	Blocks *float64 `toml:"blocks"`
	Calm   *float64 `toml:"calm"`
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
}

type fileTOML struct {
	Score scoreTOML `toml:"score"`
	Focus focusTOML `toml:"focus"`
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
	}{{"depth", f.Score.Depth, &c.Score.Depth}, {"blocks", f.Score.Blocks, &c.Score.Blocks}, {"calm", f.Score.Calm, &c.Score.Calm}} {
		if w.v == nil {
			continue
		}
		if *w.v < 0 || *w.v > 1 || math.IsNaN(*w.v) {
			return Default(), fmt.Errorf("%s: [score] %s = %g, want 0 to 1", name, w.key, *w.v)
		}
		*w.dst = *w.v
	}
	if sum := c.Score.Depth + c.Score.Blocks + c.Score.Calm; math.Abs(sum-1) > 0.001 {
		return Default(), fmt.Errorf("%s: [score] weights sum to %.2f, want 1", name, sum)
	}
	if err := decodeFocus(f.Focus, &c.Focus); err != nil {
		return Default(), fmt.Errorf("%s: %w", name, err)
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
