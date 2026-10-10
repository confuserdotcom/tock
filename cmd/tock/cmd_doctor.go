package main

import (
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"tock/internal/app"
	"tock/internal/classify"
	"tock/internal/config"
)

// System Settings panes for the grants trackd needs.
const (
	axSettingsURL         = "x-apple.systempreferences:com.apple.preference.security?Privacy_Accessibility"
	automationSettingsURL = "x-apple.systempreferences:com.apple.preference.security?Privacy_Automation"
)

// doctorWait is how long --wait gives trackd to report its first heartbeat.
const doctorWait = 15 * time.Second

// cmdDoctor checks the install: rules, config, trackd and its permissions.
// It exits 1 when anything needs fixing.
func cmdDoctor(env Env, args []string) error {
	fs := newFlags("doctor")
	wait := fs.Bool("wait", false, "wait up to 15 s for trackd to report")
	openPanes := fs.Bool("open", false, "open System Settings for missing permissions")
	if err := parseFlags(fs, args); err != nil {
		return err
	}
	if fs.NArg() > 0 {
		return usagef("doctor takes only flags")
	}
	problems := 0
	ok := func(name, detail string) { fmt.Fprintf(env.Stdout, "ok  %s: %s\n", name, detail) }
	bad := func(name, detail, fix string) {
		problems++
		fmt.Fprintf(env.Stdout, "!!  %s: %s — %s\n", name, detail, fix)
	}
	open := func(url string) {
		if *openPanes && env.Open != nil {
			if err := env.Open(url); err != nil {
				fmt.Fprintf(env.Stderr, "tock: cannot open System Settings: %v\n", err)
			}
		}
	}

	switch rs, err := classify.Load(env.rulesPath(), env.learnedPath()); {
	case errors.Is(err, classify.ErrNoRules):
		bad("rules", "no rules.toml", "tock init")
	case err != nil:
		bad("rules", err.Error(), "fix the file (tock rules check)")
	default:
		ok("rules", fmt.Sprintf("%d rules, %d learned", len(rs.Rules), len(rs.Learned)))
	}

	cfgPath := env.paths().Config
	switch _, err := config.Load(cfgPath); {
	case err != nil:
		bad("config", err.Error(), "fix "+cfgPath)
	default:
		if _, statErr := os.Stat(cfgPath); statErr != nil {
			ok("config", "defaults (no config.toml)")
		} else {
			ok("config", config.File)
		}
	}

	h, err := doctorHealth(env, *wait)
	switch {
	case errors.Is(err, errNoDB):
		bad("trackd", "no database yet", "make install")
	case err != nil:
		bad("trackd", err.Error(), "make install")
	case !h.TrackdUp:
		bad("trackd", "not running", "launchctl kickstart -k gui/$UID/com.tock.trackd")
	default:
		v := h.Version
		if v == "" {
			v = "unknown"
		}
		ok("trackd", "running (version "+v+")")
	}
	if err == nil {
		switch {
		case h.AXTrusted == nil:
			bad("accessibility", "unknown until trackd reports", "run tock doctor again in a minute")
		case !*h.AXTrusted:
			bad("accessibility", "not granted, no window titles", "System Settings › Privacy & Security › Accessibility")
			open(axSettingsURL)
		default:
			ok("accessibility", "granted")
		}
		if len(h.AutomationOff) > 0 {
			bad("automation", "no URLs from "+strings.Join(h.AutomationOff, ", "),
				"System Settings › Privacy & Security › Automation")
			open(automationSettingsURL)
		} else {
			ok("automation", "no browser denied")
		}
	}

	if env.APIKey != "" {
		ok("suggestions", "on (ANTHROPIC_API_KEY set)")
	} else {
		ok("suggestions", "off (set ANTHROPIC_API_KEY to enable)")
	}

	if problems > 0 {
		if problems == 1 {
			return errors.New("doctor: 1 problem")
		}
		return fmt.Errorf("doctor: %d problems", problems)
	}
	return nil
}

// doctorHealth reads trackd's state, polling once a second for up to
// doctorWait while trackd has not reported, when wait is set.
func doctorHealth(env Env, wait bool) (app.Health, error) {
	deadline := env.Now().Add(doctorWait)
	for {
		h, err := readHealth(env)
		if (err == nil && h.TrackdUp) || !wait || !env.Now().Before(deadline) || env.Sleep == nil {
			return h, err
		}
		env.Sleep(time.Second)
	}
}

func readHealth(env Env) (app.Health, error) {
	st, err := openStore(env)
	if err != nil {
		return app.Health{}, err
	}
	defer st.Close()
	return app.ReadHealth(st, env.Now().UnixMilli())
}
