package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"

	"tock/internal/app"
	"tock/internal/classify"
	"tock/internal/config"
	"tock/internal/fronttab"
	"tock/internal/gitlog"
	"tock/internal/store"
	"tock/internal/update"
)

// System Settings panes for the grants trackd needs.
const (
	axSettingsURL         = "x-apple.systempreferences:com.apple.preference.security?Privacy_Accessibility"
	automationSettingsURL = "x-apple.systempreferences:com.apple.preference.security?Privacy_Automation"
	calendarSettingsURL   = "x-apple.systempreferences:com.apple.preference.security?Privacy_Calendars"
)

// updateLagMS is how long a Linux machine may run an older build than the
// one published before doctor on the Mac says so.
const updateLagMS = 60 * 60_000

// doctorWait is how long --wait gives trackd to report its first heartbeat.
const doctorWait = 15 * time.Second

// cmdDoctor checks the install: rules, config, trackd and its permissions.
// It exits 1 when anything needs fixing.
func cmdDoctor(env Env, args []string) error {
	fs := newFlags("doctor")
	wait := fs.Bool("wait", false, "wait up to 15 s for trackd to report")
	openPanes := fs.Bool("open", false, "open System Settings for missing permissions")
	deep := fs.Bool("deep", false, "also run a full integrity check of tock.db")
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
		detail := fmt.Sprintf("%d rules, %d learned", len(rs.Rules), len(rs.Learned))
		if n := len(classify.Duplicates(rs)); n > 0 {
			detail += fmt.Sprintf(" · %s look alike (tock rules dupes)", plural(n, "name pair"))
		}
		ok("rules", detail)
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
		if k := cfg.Privacy.KeepTitles; k > 0 {
			ok("privacy", fmt.Sprintf("window titles kept %s, then deleted hourly by trackd", plural(k, "day")))
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
		if h.Paused() {
			bad("recording", "paused, nothing is recorded: "+h.CapturePaused,
				"fix config.toml; trackd starts recording again within seconds")
		}
	}
	if err == nil && cfgErr == nil {
		doctorBackup(env, cfg.Backup.Keep, h, ok, bad)
	}
	if *deep && err == nil {
		doctorDeep(env, ok, bad)
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
		doctorAFK(env, cfg, ok, bad)
		doctorBrowser(env, ok)
		doctorSync(env, cfg.Sync, ok, bad)
		doctorUpdates(env, cfg.Sync, ok, bad)
		doctorGuard(env, cfg, ok, bad)
		doctorOCR(env, cfg.OCR, h, ok, bad)
	}

	if env.hasClaude() {
		ok("suggestions", "on via "+env.claudeVia())
	} else {
		ok("suggestions", "off (install Claude Code and log in, or set ANTHROPIC_API_KEY)")
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

// doctorAFK names the AFK windows, and what makes them guesses: a Linux
// machine whose hypridle does not run trackd idle, other machines that do
// not say they report input.
func doctorAFK(env Env, cfg config.Config, ok func(name, detail string), bad func(name, detail, fix string)) {
	stale := classify.FormatDur(cfg.AFK.StaleMS)
	if env.Linux && (env.Hypridle == nil || !env.Hypridle()) {
		bad("idle", "hypridle does not run trackd idle, so trackd never sees you leave; windows over "+stale+" are cut",
			"add the listener in README › Linux to hypridle's config (the -c file it runs with, else ~/.config/hypr/hypridle.conf)")
	}
	detail := "idle after 5m without input"
	if rs, err := classify.Load(env.rulesPath(), env.learnedPath()); err == nil {
		var longer []string
		for _, c := range rs.Categories {
			if c.AFKMS > 0 && c.AFKMS != classify.DefaultAFKMS {
				longer = append(longer, c.Name+" "+classify.FormatDur(c.AFKMS))
			}
		}
		if len(longer) > 0 {
			detail += " (" + strings.Join(longer, ", ") + ")"
		}
	}
	if !env.Linux {
		detail += ", " + classify.FormatDur(cfg.AFK.VideoMS) + " while a video plays"
	}
	ok("afk", detail)
	if cfg.Sync.Dir == "" {
		return
	}
	st, err := openStore(env)
	if err != nil {
		return
	}
	defer st.Close()
	ms, err := st.RemoteMachines()
	if err != nil {
		return
	}
	var blind []string
	long, _ := st.RemoteLong(env.Now().UnixMilli()-7*24*60*60_000, cfg.AFK.TrustMS)
	trust := classify.FormatDur(cfg.AFK.TrustMS)
	for _, m := range ms {
		if v, _, _ := st.State(app.RemoteInputKey(m.Name)); v != "1" {
			blind = append(blind, m.Name)
			continue
		}
		if hid, _, _ := st.State(app.RemoteHIDKey(m.Name)); hid != "1" && long[m.Name] > 0 {
			bad("afk", fmt.Sprintf("%s says it reports input, yet %d of its windows in the last 7 days ran over %s: its hypridle missed idle, and those windows keep only their afk window",
				m.Name, long[m.Name], trust),
				"on "+m.Name+": check that hypridle runs (pgrep -a hypridle) and that nothing inhibits it, then tock doctor there")
		}
	}
	if len(blind) > 0 {
		bad("afk", strings.Join(blind, ", ")+" does not report input; its windows over "+stale+" are cut",
			"update tock there and add the hypridle listener (README › Linux)")
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
		if err == nil && len(ms) > 0 {
			var names []string
			for _, m := range ms {
				n := m.Name
				if v, _, _ := st.State(app.RemoteVersionKey(m.Name)); v != "" {
					n += " " + v
				}
				if m.LastMS > 0 {
					n += " (last " + time.UnixMilli(m.LastMS).In(env.Now().Location()).Format("Mon 15:04") + ")"
				}
				names = append(names, n)
			}
			detail = "as " + s.MachineName() + " with " + strings.Join(names, ", ")
		}
		st.Close()
	}
	ok("sync", detail)
	switch {
	case s.NoRules:
		ok("rules sync", "off ([sync] rules = false)")
	default:
		detail := "on: the newest rules.toml wins, the one it replaces is kept as rules.toml.bak"
		if st, err := openStore(env); err == nil {
			v, _, _ := st.State(app.KeyRulesFrom)
			st.Close()
			if name, ms, ok := strings.Cut(v, " "); ok {
				if n, err := strconv.ParseInt(ms, 10, 64); err == nil {
					detail = "on; last took " + name + "'s, changed " + time.UnixMilli(n).In(env.Now().Location()).Format("Mon 2 Jan 15:04")
				}
			}
		}
		ok("rules sync", detail)
		if st, err := openStore(env); err == nil {
			if v, _, _ := st.State(app.KeyRulesRefused); v != "" {
				bad("rules sync", "kept this machine's rules.toml: the newer one from "+v, "fix what it names, then tock sync")
			}
			if ms, err := st.RemoteMachines(); err == nil {
				for _, m := range ms {
					if v, _, _ := st.State(app.RemoteRulesRefusedKey(m.Name)); v != "" {
						bad("rules sync", m.Name+" kept its rules.toml: the newer one from "+v, "on "+m.Name+": fix what it names, then tock sync")
					}
				}
			}
			st.Close()
		}
	}
}

// doctorUpdates checks the self-update. On Linux: whether it is on and the
// last error. On the Mac: whether each Linux machine runs the build that was
// published, or has lagged it for more than an hour.
func doctorUpdates(env Env, s config.Sync, ok func(name, detail string), bad func(name, detail, fix string)) {
	if s.Dir == "" {
		return
	}
	var st *store.Store
	if st, _ = openStore(env); st != nil {
		defer st.Close()
	}
	pub, published := update.Published(filepath.Join(s.Dir, update.BinDir))
	if env.Linux {
		var e string
		if st != nil {
			e, _, _ = st.State(update.KeyError)
		}
		switch {
		case s.NoUpdate:
			ok("updates", "off ([sync] update = false)")
		case e != "":
			bad("updates", e, "see ~/.local/state/tock/trackd.log and README › Updates")
		case published:
			ok("updates", "on; published "+pub.Version)
		default:
			ok("updates", "on; nothing published yet")
		}
		return
	}
	if st == nil {
		return
	}
	ms, err := st.RemoteMachines()
	if err != nil {
		return
	}
	home, _ := os.UserHomeDir()
	for _, m := range ms {
		if hid, _, _ := st.State(app.RemoteHIDKey(m.Name)); hid == "1" {
			continue // only Linux machines take published builds
		}
		if !published {
			ok("updates", "nothing published for Linux (make hooks, then commit on main; or make publish-linux)")
			return
		}
		v, _, _ := st.State(app.RemoteVersionKey(m.Name))
		e, _, _ := st.State(app.RemoteUpdateErrorKey(m.Name))
		says := ""
		if e != "" {
			says = "; it says: " + e
		}
		runs := v
		if runs == "" {
			runs = "unknown"
		}
		age := env.Now().UnixMilli() - pub.BuiltAt.UnixMilli()
		switch {
		case v == pub.Version:
			ok("updates", m.Name+" runs the published "+v)
		case age <= updateLagMS:
			ok("updates", fmt.Sprintf("%s runs %s; %s published %s ago, not taken yet%s", m.Name, runs, pub.Version, classify.FormatDur(age), says))
		case v == "":
			bad("updates", fmt.Sprintf("%s runs %s, but %s was published %s ago%s", m.Name, runs, pub.Version, classify.FormatDur(age), says),
				"run once on "+m.Name+": "+update.BootstrapCommand(s.Dir, home, "amd64"))
		default:
			bad("updates", fmt.Sprintf("%s runs %s, but %s was published %s ago%s", m.Name, runs, pub.Version, classify.FormatDur(age), says),
				"on "+m.Name+": check that Syncthing syncs "+filepath.Join(s.Dir, update.BinDir)+", then see ~/.local/state/tock/trackd.log")
		}
	}
}

// doctorRepos checks that git reads every [[project]] repo.
func doctorRepos(env Env, rs *classify.Ruleset, ok func(name, detail string), bad func(name, detail, fix string)) {
	run := env.Git
	if run == nil {
		run = gitlog.Exec
	}
	exists := env.DirExists
	if exists == nil {
		exists = func(dir string) bool {
			_, err := os.Stat(dir)
			return !errors.Is(err, os.ErrNotExist)
		}
	}
	home, _ := os.UserHomeDir()
	n, absent, failed := 0, 0, false
	for _, p := range rs.Projects {
		for _, r := range p.Repos {
			dir := classify.ExpandHome(r, home)
			if !exists(dir) {
				absent++ // rules.toml synced from a machine that has it
				continue
			}
			n++
			if err := gitlog.Check(context.Background(), run, dir); err != nil {
				failed = true
				bad("git", fmt.Sprintf("project %s: %s is not a readable git repo (%v)", p.Name, r, err),
					"fix repos in rules.toml")
			}
		}
	}
	elsewhere := ""
	if absent > 0 {
		elsewhere = fmt.Sprintf(", %d not on this machine", absent)
	}
	switch {
	case n == 0 && absent == 0:
		ok("git", "no repos (rules.toml [[project]] repos)")
	case !failed:
		ok("git", fmt.Sprintf("%d %s%s", n, map[bool]string{true: "repo", false: "repos"}[n == 1], elsewhere))
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

// doctorGuard reports the focus guard: blocking, the Focus mode shortcuts
// (which must exist in the Shortcuts app) and Slack.
func doctorGuard(env Env, cfg config.Config, ok func(name, detail string), bad func(name, detail, fix string)) {
	f := cfg.Focus
	if f.Block {
		ok("blocking", fmt.Sprintf("on in sessions: distractions, %s, %s",
			plural(len(f.BlockApps), "extra app"), plural(len(f.BlockHosts), "extra site")))
	} else {
		ok("blocking", "off (config.toml [focus] block = true)")
	}
	var names []string
	for _, n := range []string{f.FocusOn, f.FocusOff} {
		if n != "" {
			names = append(names, n)
		}
	}
	switch {
	case len(names) == 0:
		ok("focus mode", "off (config.toml [focus] focus_on / focus_off)")
	case env.Linux:
		bad("focus mode", "Shortcuts exist only on macOS", "remove focus_on / focus_off")
	default:
		var missing []string
		if env.ListShortcuts != nil {
			have, err := env.ListShortcuts()
			if err != nil {
				bad("focus mode", "cannot list shortcuts: "+err.Error(), "open the Shortcuts app once")
				break
			}
			for _, n := range names {
				if !slices.Contains(have, n) {
					missing = append(missing, strconv.Quote(n))
				}
			}
		}
		if len(missing) > 0 {
			bad("focus mode", "no shortcut "+strings.Join(missing, ", "), "create it in Shortcuts (README: Focus mode)")
			break
		}
		ok("focus mode", "shortcuts "+strings.Join(quoteAll(names), " / "))
	}
	switch {
	case !cfg.Slack.Enabled:
		ok("slack", "off (config.toml [slack] enabled = true)")
	case env.slackToken() == "":
		bad("slack", "on, but no token", "tock slack login")
	default:
		ok("slack", "on (tock slack checks the token)")
	}
}

func quoteAll(ss []string) []string {
	out := make([]string, len(ss))
	for i, s := range ss {
		out[i] = strconv.Quote(s)
	}
	return out
}

// doctorOCR reports screen reading: off, its permission and today's reads.
func doctorOCR(env Env, cfg config.OCR, h app.Health, ok func(name, detail string), bad func(name, detail, fix string)) {
	var stats map[string]store.OCRStat
	if cfg.Enabled && !env.Linux {
		if st, err := openStore(env); err == nil {
			stats, _ = st.OCRStats(env.Now().Format("2006-01-02"))
			st.Close()
		}
	}
	if detail, fix := ocrDoctor(cfg, env.Linux, h.OCRPerm, stats); fix != "" {
		bad("ocr", detail, fix)
	} else {
		ok("ocr", detail)
	}
}

// doctorBrowser says, on Linux, whether the tock browser extension sends
// the front tab: without it browser windows have titles but no address.
func doctorBrowser(env Env, ok func(name, detail string)) {
	if !env.Linux {
		return
	}
	st, found := fronttab.Read(filepath.Join(env.StateDir, fronttab.File))
	if !found {
		ok("browser", "off: browser windows get no web address (trackd browser install)")
		return
	}
	ok("browser", "the tock extension sends the tab in front (last "+time.UnixMilli(st.AtMS).In(env.Now().Location()).Format("15:04")+")")
}
