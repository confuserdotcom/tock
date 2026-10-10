package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"tock/internal/app"
	"tock/internal/classify"
	"tock/internal/config"
	"tock/internal/gitlog"
)

// System Settings panes for the grants trackd needs.
const (
	axSettingsURL         = "x-apple.systempreferences:com.apple.preference.security?Privacy_Accessibility"
	automationSettingsURL = "x-apple.systempreferences:com.apple.preference.security?Privacy_Automation"
	calendarSettingsURL   = "x-apple.systempreferences:com.apple.preference.security?Privacy_Calendars"
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
		doctorRepos(env, rs, ok, bad)
	}

	cfgPath := env.paths().Config
	cfg, cfgErr := config.Load(cfgPath)
	switch err := cfgErr; {
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
	case !h.TrackdUp && env.Linux:
		bad("trackd", "not running", "systemctl --user restart trackd")
	case !h.TrackdUp:
		bad("trackd", "not running", "launchctl kickstart -k gui/$UID/com.tock.trackd")
	default:
		v := h.Version
		if v == "" {
			v = "unknown"
		}
		ok("trackd", "running (version "+v+")")
	}
	if err == nil && env.Linux {
		if cfgErr == nil {
			if cfg.Focus.Notify {
				ok("notifications", "notify-send")
			} else {
				ok("notifications", "off (config.toml [focus] notify = false)")
			}
		}
	} else if err == nil {
		switch {
		case h.AXTrusted == nil:
			bad("accessibility", "unknown until trackd reports", "run tock doctor again in a minute")
		case !*h.AXTrusted:
			bad("accessibility", "not granted, no window titles", "System Settings › Privacy & Security › Accessibility")
			open(axSettingsURL)
		default:
			ok("accessibility", "granted")
		}
		if cfgErr == nil {
			doctorCalendar(env, cfg.Calendar.Enabled, h, ok, bad, open)
			doctorNotify(cfg.Focus.Notify, h, ok)
		}
		if len(h.AutomationOff) > 0 {
			bad("automation", "no URLs from "+strings.Join(h.AutomationOff, ", "),
				"System Settings › Privacy & Security › Automation")
			open(automationSettingsURL)
		} else {
			ok("automation", "no browser denied")
		}
	}

	if cfgErr == nil {
		doctorSync(env, cfg.Sync, ok, bad)
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

// calendarStaleMS is how old the last calendar sync may be while access is
// granted: three missed 10-minute syncs.
const calendarStaleMS = 30 * 60_000

// doctorCalendar reports trackd's calendar sync. The states are trackd's
// (collect.Calendar*), which tock does not link.
func doctorCalendar(env Env, enabled bool, h app.Health, ok func(name, detail string),
	bad func(name, detail, fix string), open func(url string)) {
	if !enabled {
		ok("calendar", "off (config.toml [calendar] enabled = false)")
		return
	}
	switch h.CalendarStatus {
	case "granted":
		age := env.Now().UnixMilli() - h.CalendarSyncMS
		if h.CalendarSyncMS == 0 || age > calendarStaleMS {
			bad("calendar", "no sync in the last 30 min", "see ~/.local/state/tock/trackd.log")
			return
		}
		ok("calendar", "synced "+time.UnixMilli(h.CalendarSyncMS).In(env.Now().Location()).Format("15:04"))
	case "denied":
		bad("calendar", "access denied", "System Settings › Privacy & Security › Calendars › trackd")
		open(calendarSettingsURL)
	case "error":
		bad("calendar", "EventKit read failed", "see ~/.local/state/tock/trackd.log")
	default:
		bad("calendar", "waiting for the permission prompt", "restart trackd: make install")
	}
}

// doctorNotify says how focus notifications go out. osascript always
// works, so this is never a problem.
func doctorNotify(on bool, h app.Health, ok func(name, detail string)) {
	switch {
	case !on:
		ok("notifications", "off (config.toml [focus] notify = false)")
	case !h.BarUp:
		ok("notifications", "plain, without buttons (menu bar app not running; make install)")
	case h.BarNotify == app.NotifyGranted:
		ok("notifications", "native, with buttons (menu bar)")
	case h.BarNotify == app.NotifyDenied:
		ok("notifications", "plain, without buttons (turned off in System Settings › Notifications › tock)")
	default:
		ok("notifications", "plain until the menu bar app is allowed to notify")
	}
}

// doctorSync checks the sync folder and names the other machines seen.
func doctorSync(env Env, s config.Sync, ok func(name, detail string), bad func(name, detail, fix string)) {
	if s.Dir == "" {
		ok("sync", "off (config.toml [sync] dir)")
		return
	}
	if fi, err := os.Stat(s.Dir); err != nil || !fi.IsDir() {
		bad("sync", "no folder "+s.Dir, "create it, or fix [sync] dir in config.toml")
		return
	}
	detail := "as " + s.MachineName() + ", no other machine yet"
	if st, err := openStore(env); err == nil {
		ms, err := st.RemoteMachines()
		st.Close()
		if err == nil && len(ms) > 0 {
			var names []string
			for _, m := range ms {
				n := m.Name
				if m.LastMS > 0 {
					n += " (last " + time.UnixMilli(m.LastMS).In(env.Now().Location()).Format("Mon 15:04") + ")"
				}
				names = append(names, n)
			}
			detail = "as " + s.MachineName() + " with " + strings.Join(names, ", ")
		}
	}
	ok("sync", detail)
}

// doctorRepos checks that git reads every [[project]] repo.
func doctorRepos(env Env, rs *classify.Ruleset, ok func(name, detail string), bad func(name, detail, fix string)) {
	run := env.Git
	if run == nil {
		run = gitlog.Exec
	}
	home, _ := os.UserHomeDir()
	n, failed := 0, false
	for _, p := range rs.Projects {
		for _, r := range p.Repos {
			n++
			if err := gitlog.Check(context.Background(), run, classify.ExpandHome(r, home)); err != nil {
				failed = true
				bad("git", fmt.Sprintf("project %s: %s is not a readable git repo (%v)", p.Name, r, err),
					"fix repos in rules.toml")
			}
		}
	}
	switch {
	case n == 0:
		ok("git", "no repos (rules.toml [[project]] repos)")
	case !failed:
		ok("git", fmt.Sprintf("%d %s", n, map[bool]string{true: "repo", false: "repos"}[n == 1]))
	}
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
