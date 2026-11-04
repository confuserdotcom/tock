package main

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
	"tock/internal/config"
	"tock/internal/fronttab"

	"tock/internal/store"
	"tock/internal/update"
)

func setStates(t *testing.T, te *tenv, kv map[string]string) {
	t.Helper()
	st, err := store.Open(te.dbPath())
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	for k, v := range kv {
		if err := st.SetState(k, v, testNow.UnixMilli()); err != nil {
			t.Fatal(err)
		}
	}
}

// nowMS is a heartbeat at testNow.
func nowMS() string { return strconv.FormatInt(testNow.UnixMilli(), 10) }

func doctorEnv(t *testing.T, rules string) (*tenv, *[]string) {
	t.Helper()
	te := newTestEnv(t, rules)
	var opened []string
	te.Open = func(url string) error { opened = append(opened, url); return nil }
	te.Sleep = func(d time.Duration) { te.now = te.now.Add(d) }
	return te, &opened
}

func TestDoctorAllGood(t *testing.T) {
	te, opened := doctorEnv(t, testRules)
	te.APIKey = "test-key"
	te.seed(t, daySpans()...)
	setStates(t, te, map[string]string{
		store.KeyHeartbeat: nowMS(), store.KeyAXTrusted: "1", store.KeyVersion: "v1",
	})
	code, out := te.run(t, "", "doctor", "--open")
	if code != 0 || strings.Count(out, "ok  ") != 15 || strings.Contains(out, "!!") {
		t.Fatalf("exit %d:\n%s%s", code, out, te.errb)
	}
	for _, want := range []string{"rules: 3 rules, 0 learned", "config: defaults", "trackd: running (version v1)",
		"accessibility: granted", "suggestions: on", "afk: idle after 5m without input, 3h while a video plays"} {
		if !strings.Contains(out, want) {
			t.Errorf("output lacks %q:\n%s", want, out)
		}
	}
	if strings.Contains(out+te.errb.String(), "test-key") {
		t.Fatal("doctor printed the API key")
	}
	if len(*opened) != 0 {
		t.Errorf("opened %v with nothing missing", *opened)
	}
}

func TestDoctorMissingGrants(t *testing.T) {
	te, opened := doctorEnv(t, testRules)
	te.seed(t, daySpans()...)
	setStates(t, te, map[string]string{
		store.KeyHeartbeat: nowMS(), store.KeyAXTrusted: "0",
		store.AutomationKey("com.google.Chrome"): "denied",
	})
	code, out := te.run(t, "", "doctor", "--open")
	if code != 1 || !strings.Contains(out, "!!  accessibility: not granted") ||
		!strings.Contains(out, "!!  automation: no URLs from Chrome") || !strings.Contains(out, "suggestions: off") {
		t.Fatalf("exit %d:\n%s", code, out)
	}
	if te.errb.String() != "tock: doctor: 2 problems\n" {
		t.Errorf("stderr = %q", te.errb.String())
	}
	if want := []string{axSettingsURL, automationSettingsURL}; len(*opened) != 2 || (*opened)[0] != want[0] || (*opened)[1] != want[1] {
		t.Errorf("opened = %v", *opened)
	}
	if _, _ = te.run(t, "", "doctor"); len(*opened) != 2 {
		t.Error("opened panes without --open")
	}
}

func TestDoctorWaitsForTrackd(t *testing.T) {
	te, _ := doctorEnv(t, testRules)
	start := te.now
	code, out := te.run(t, "", "doctor", "--wait")
	if code != 1 || !strings.Contains(out, "!!  trackd: no database yet") {
		t.Fatalf("exit %d:\n%s", code, out)
	}
	if waited := te.now.Sub(start); waited != doctorWait {
		t.Errorf("waited %v, want %v", waited, doctorWait)
	}
	if strings.Contains(out, "accessibility") {
		t.Error("permission checks need trackd's state")
	}
}

func TestDoctorRulesProblems(t *testing.T) {
	te, _ := doctorEnv(t, "")
	if _, out := te.run(t, "", "doctor"); !strings.Contains(out, "!!  rules: no rules.toml — tock init") {
		t.Fatalf("no rules:\n%s", out)
	}
	te, _ = doctorEnv(t, "[[rule]]\napp = 'x'\ncategory = \"Nope\"\n")
	if _, out := te.run(t, "", "doctor"); !strings.Contains(out, "unknown category") {
		t.Fatalf("bad rules:\n%s", out)
	}
	if code, _ := te.run(t, "", "doctor", "x"); code != 2 {
		t.Fatal("extra argument accepted")
	}
}

func TestDoctorCalendar(t *testing.T) {
	recent := strconv.FormatInt(testNow.Add(-5*time.Minute).UnixMilli(), 10)
	stale := strconv.FormatInt(testNow.Add(-31*time.Minute).UnixMilli(), 10)
	for _, c := range []struct {
		status, sync, want string
		code               int
	}{
		{"granted", recent, "ok  calendar: synced 14:55", 0},
		{"granted", stale, "!!  calendar: no sync in the last 30 min", 1},
		{"denied", "", "!!  calendar: access denied — System Settings › Privacy & Security › Calendars › trackd", 1},
		{"error", "", "!!  calendar: EventKit read failed", 1},
		{"", "", "!!  calendar: waiting for the permission prompt — restart trackd: make install", 1},
	} {
		te, opened := doctorEnv(t, testRules)
		te.seed(t, daySpans()...)
		writeFile(t, te.paths().Config, "[calendar]\nenabled = true\n")
		kv := map[string]string{store.KeyHeartbeat: nowMS(), store.KeyAXTrusted: "1"}
		if c.status != "" {
			kv[store.KeyCalendarStatus] = c.status
		}
		if c.sync != "" {
			kv[store.KeyCalendarSync] = c.sync
		}
		setStates(t, te, kv)
		code, out := te.run(t, "", "doctor", "--open")
		if code != c.code || !strings.Contains(out, c.want) {
			t.Errorf("status %q sync %q: exit %d\n%s", c.status, c.sync, code, out)
		}
		if (c.status == "denied") != (len(*opened) == 1) {
			t.Errorf("status %q: opened %v", c.status, *opened)
		}
	}
}

func writeFile(t *testing.T, path, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestDoctorNotifications(t *testing.T) {
	fresh := nowMS()
	old := strconv.FormatInt(testNow.Add(-time.Minute).UnixMilli(), 10)
	for _, c := range []struct {
		config, beat, access, want string
	}{
		{"", "", "", "ok  notifications: plain, without buttons (menu bar app not running; make install)"},
		{"", old, "granted", "ok  notifications: plain, without buttons (menu bar app not running; make install)"},
		{"", fresh, "granted", "ok  notifications: native, with buttons (menu bar)"},
		{"", fresh, "denied", "ok  notifications: plain, without buttons (turned off in System Settings › Notifications › tock)"},
		{"", fresh, "unknown", "ok  notifications: plain until the menu bar app is allowed to notify"},
		{"[focus]\nnotify = false\n", fresh, "granted", "ok  notifications: off (config.toml [focus] notify = false)"},
	} {
		te, _ := doctorEnv(t, testRules)
		te.seed(t, daySpans()...)
		if c.config != "" {
			writeFile(t, te.paths().Config, c.config)
		}
		kv := map[string]string{store.KeyHeartbeat: nowMS(), store.KeyAXTrusted: "1"}
		if c.beat != "" {
			kv[store.KeyBarBeat], kv[store.KeyBarNotify] = c.beat, c.access
		}
		setStates(t, te, kv)
		if _, out := te.run(t, "", "doctor"); !strings.Contains(out, c.want) {
			t.Errorf("beat %q access %q:\n%s", c.beat, c.access, out)
		}
	}
}

func TestDoctorLinux(t *testing.T) {
	te, opened := doctorEnv(t, testRules)
	te.Linux = true
	te.Hypridle = func() bool { return true }
	te.seed(t, daySpans()...)
	setStates(t, te, map[string]string{store.KeyHeartbeat: nowMS(), store.KeyAXTrusted: "1"})
	code, out := te.run(t, "", "doctor", "--open")
	if code != 0 || !strings.Contains(out, "ok  notifications: notify-send") ||
		strings.Contains(out, "accessibility") || strings.Contains(out, "automation") || strings.Contains(out, "calendar") || len(*opened) != 0 {
		t.Fatalf("exit %d:\n%s", code, out)
	}
	if !strings.Contains(out, "ok  browser: off: browser windows get no web address (trackd browser install)") {
		t.Fatalf("no extension:\n%s", out)
	}
	os.MkdirAll(te.StateDir, 0o700)
	os.WriteFile(filepath.Join(te.StateDir, fronttab.File), []byte(`{"at_ms":`+nowMS()+`,"focused":true}`), 0o600)
	if _, out := te.run(t, "", "doctor"); !strings.Contains(out, "ok  browser: the tock extension sends the tab in front (last 15:00)") {
		t.Fatalf("extension:\n%s", out)
	}
	te, _ = doctorEnv(t, testRules)
	te.Linux = true
	te.seed(t, daySpans()...)
	if _, out := te.run(t, "", "doctor"); !strings.Contains(out, "!!  trackd: not running — systemctl --user restart trackd") ||
		!strings.Contains(out, "!!  idle: hypridle does not run trackd idle") {
		t.Fatalf("down:\n%s", out)
	}
}

// lagDoctor is tock doctor on a Mac that syncs with ryuk. ryuk's machine.json
// is ryukJSON, and the Mac published pubVer pubAge before testNow ("" for none).
func lagDoctor(t *testing.T, ryukJSON, pubVer string, pubAge time.Duration) (code int, out string) {
	t.Helper()
	dir := t.TempDir()
	te, _ := doctorEnv(t, testRules)
	te.seed(t, daySpans()...)
	setStates(t, te, map[string]string{store.KeyHeartbeat: nowMS(), store.KeyAXTrusted: "1", store.KeyVersion: "mac1"})
	writeFile(t, te.paths().Config, "[sync]\ndir = \""+dir+"\"\nmachine = \"mac\"\n")
	os.MkdirAll(filepath.Join(dir, "ryuk"), 0o700)
	line := `{"s":` + strconv.FormatInt(at(t, "11:45"), 10) + `,"e":` + strconv.FormatInt(at(t, "12:15"), 10) + `,"b":"kitty","a":"kitty","c":"Code"}` + "\n"
	os.WriteFile(filepath.Join(dir, "ryuk", "2026-10-10.jsonl"), []byte(line), 0o600)
	os.WriteFile(filepath.Join(dir, "ryuk", "machine.json"), []byte(ryukJSON), 0o600)
	if pubVer != "" {
		os.MkdirAll(filepath.Join(dir, "bin", "linux-amd64"), 0o700)
		m := `{"version":"` + pubVer + `","built_at":"` + testNow.Add(-pubAge).UTC().Format(time.RFC3339) + `","files":{}}`
		os.WriteFile(filepath.Join(dir, "bin", "linux-amd64", "manifest.json"), []byte(m), 0o600)
	}
	if code, out := te.run(t, "", "sync"); code != 0 {
		t.Fatalf("sync: %d %s", code, out)
	}
	return te.run(t, "", "doctor")
}

func TestDoctorUpdatesLag(t *testing.T) {
	const old = `{"input":true,"version":"old1","update_error":"update: manifest.sig does not match"}`
	_, out := lagDoctor(t, old, "new2", 2*time.Hour)
	for _, want := range []string{"!!  updates: ryuk runs old1, but new2 was published 2h ago", "it says: update: manifest.sig does not match",
		"ok  sync: as mac with ryuk old1 (last "} {
		if !strings.Contains(out, want) {
			t.Errorf("output lacks %q:\n%s", want, out)
		}
	}
	// a recent publish is not behind yet
	code, out := lagDoctor(t, old, "new2", 10*time.Minute)
	if !strings.Contains(out, "ok  updates: ryuk runs old1; new2 published 10m ago, not taken yet") || strings.Contains(out, "!!  updates") {
		t.Errorf("10 min:\n%s", out)
	}
	if code != 0 {
		t.Errorf("exit %d with ryuk 10 min behind:\n%s", code, out)
	}
	_, out = lagDoctor(t, `{"input":true,"version":"new2"}`, "new2", 2*time.Hour)
	if !strings.Contains(out, "ok  updates: ryuk runs the published new2") || strings.Contains(out, "!!  updates") {
		t.Errorf("up to date:\n%s", out)
	}
}

func TestDoctorUpdatesUnknownVersion(t *testing.T) {
	_, out := lagDoctor(t, `{"input":true}`, "new2", 2*time.Hour)
	if !strings.Contains(out, "!!  updates: ryuk runs unknown, but new2 was published 2h ago") || !strings.Contains(out, "trackd install") {
		t.Errorf("output:\n%s", out)
	}
}

func TestDoctorUpdatesNothingPublished(t *testing.T) {
	code, out := lagDoctor(t, `{"input":true}`, "", 0)
	if code != 0 || !strings.Contains(out, "ok  updates: nothing published for Linux") || strings.Contains(out, "!!") {
		t.Errorf("exit %d:\n%s", code, out)
	}
	if !strings.Contains(out, "ok  sync: as mac with ryuk (last ") {
		t.Errorf("sync line changed:\n%s", out)
	}
}

func TestDoctorUpdatesLinux(t *testing.T) {
	linux := func(cfg string, states map[string]string) string {
		te, _ := doctorEnv(t, testRules)
		te.Linux = true
		te.Hypridle = func() bool { return true }
		te.seed(t, daySpans()...)
		states[store.KeyHeartbeat], states[store.KeyAXTrusted] = nowMS(), "1"
		setStates(t, te, states)
		writeFile(t, te.paths().Config, strings.ReplaceAll(cfg, "SYNC", t.TempDir()))
		_, out := te.run(t, "", "doctor")
		return out
	}
	if out := linux("[sync]\ndir = \"SYNC\"\n", map[string]string{update.KeyError: "update: key changed"}); !strings.Contains(out, "!!  updates: update: key changed") {
		t.Errorf("error:\n%s", out)
	}
	if out := linux("[sync]\ndir = \"SYNC\"\nupdate = false\n", map[string]string{update.KeyError: "x"}); !strings.Contains(out, "ok  updates: off ([sync] update = false)") {
		t.Errorf("off:\n%s", out)
	}
	if out := linux("[sync]\ndir = \"SYNC\"\n", map[string]string{}); !strings.Contains(out, "ok  updates: on; nothing published yet") {
		t.Errorf("on:\n%s", out)
	}
}

func TestDoctorGuardLines(t *testing.T) {
	te := newTestEnv(t, "")
	cfg := config.Default()
	var lines []string
	ok := func(n, d string) { lines = append(lines, "ok "+n+": "+d) }
	bad := func(n, d, fix string) { lines = append(lines, "!! "+n+": "+d) }
	doctorGuard(te.Env, cfg, ok, bad)
	if strings.Join(lines, "\n") != "ok blocking: off (config.toml [focus] block = true)\n"+
		"ok focus mode: off (config.toml [focus] focus_on / focus_off)\nok slack: off (config.toml [slack] enabled = true)" {
		t.Fatalf("defaults:\n%s", strings.Join(lines, "\n"))
	}
	lines = nil
	cfg.Focus.Block, cfg.Focus.BlockHosts = true, []string{"youtube.com"}
	cfg.Focus.FocusOn, cfg.Focus.FocusOff = "Work", "Off"
	cfg.Slack.Enabled = true
	te.ListShortcuts = func() ([]string, error) { return []string{"Work", "Other"}, nil }
	doctorGuard(te.Env, cfg, ok, bad)
	got := strings.Join(lines, "\n")
	for _, want := range []string{"ok blocking: on in sessions: distractions, 0 extra apps, 1 extra site",
		`!! focus mode: no shortcut "Off"`, "!! slack: on, but no token"} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in\n%s", want, got)
		}
	}
}
