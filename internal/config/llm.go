package config

import (
	"errors"
	"fmt"
	"regexp"
	"strings"
)

// LLM is the [llm] section: what may be sent to Claude. The deny lists hold
// the defaults followed by the user's additions.
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
		Enabled: false, Model: "claude-haiku-5-5", MinMS: 2 * 60_000, // opt-in: a backend is not consent
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

type llmTOML struct {
	Enabled    *bool     `toml:"enabled"`
	Model      *string   `toml:"model"`
	MinMinutes *int      `toml:"min_minutes"`
	DenyApps   *[]string `toml:"deny_apps"`
	DenyHosts  *[]string `toml:"deny_hosts"`
	DenyTitles *[]string `toml:"deny_titles"`
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
