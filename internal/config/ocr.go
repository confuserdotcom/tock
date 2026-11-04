package config

import (
	"fmt"
	"regexp"
	"strings"
)

// OCR is the [ocr] section: reading the text of unclear windows on this Mac
// to label them. Only keywords are kept, never images or full text, and
// nothing of it leaves the machine.
type OCR struct {
	// Enabled turns screen reading on. Off by default.
	Enabled bool
	// Apps are the bundle ids read whatever their label (terminals, PDF
	// viewers, Anki, browsers); others are read only when unlabelled or
	// labelled by a generic rule.
	Apps []string
	// IntervalMS is the least time between two reads of one window.
	IntervalMS int64
	// MinDwellMS is how long a window must stay in front before its first read.
	MinDwellMS int64
	// DenyApps, DenyHosts and DenyTitles are never read, on top of the
	// [llm] deny lists. Hosts and titles are RE2 patterns, titles
	// case-insensitive.
	DenyApps, DenyHosts, DenyTitles []string
}

// DefaultOCR is the [ocr] section used when the file does not set it. Every
// call returns fresh slices.
func DefaultOCR() OCR {
	return OCR{
		Apps: []string{
			"com.apple.Terminal", "com.googlecode.iterm2", "com.mitchellh.ghostty", "net.kovidgoyal.kitty",
			"com.github.wez.wezterm", "com.apple.Preview", "net.sourceforge.skim-app.skim",
			"com.readdle.PDFExpert-Mac", "net.ankiweb.launcher", "com.apple.Safari", "com.google.Chrome",
			"org.mozilla.firefox", "company.thebrowser.Browser", "app.zen-browser.zen",
		},
		IntervalMS: 60_000,
		MinDwellMS: 20_000,
	}
}

type ocrTOML struct {
	Enabled    *bool     `toml:"enabled"`
	Apps       *[]string `toml:"apps"`
	Interval   *int      `toml:"interval"`
	MinDwell   *int      `toml:"min_dwell"`
	DenyApps   *[]string `toml:"deny_apps"`
	DenyHosts  *[]string `toml:"deny_hosts"`
	DenyTitles *[]string `toml:"deny_titles"`
}

// decodeOCR applies the keys present in f to c. apps replaces the default
// list; the deny lists start empty.
func decodeOCR(f ocrTOML, c *OCR) error {
	if f.Enabled != nil {
		c.Enabled = *f.Enabled
	}
	if v := f.Interval; v != nil {
		if *v < 15 || *v > 3600 {
			return fmt.Errorf("[ocr] interval = %d, want 15 to 3600 seconds", *v)
		}
		c.IntervalMS = int64(*v) * 1000
	}
	if v := f.MinDwell; v != nil {
		if *v < 5 || *v > 300 {
			return fmt.Errorf("[ocr] min_dwell = %d, want 5 to 300 seconds", *v)
		}
		c.MinDwellMS = int64(*v) * 1000
	}
	for _, l := range []struct {
		key string
		v   *[]string
		dst *[]string
	}{{"apps", f.Apps, &c.Apps}, {"deny_apps", f.DenyApps, &c.DenyApps}} {
		if l.v == nil {
			continue
		}
		for _, b := range *l.v {
			if strings.TrimSpace(b) == "" {
				return fmt.Errorf("[ocr] %s has an empty bundle id", l.key)
			}
		}
		*l.dst = append([]string{}, *l.v...)
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
				return fmt.Errorf("[ocr] %s has %q: %v", l.key, p, err)
			}
		}
		*l.dst = append([]string{}, *l.v...)
	}
	return nil
}
