package main

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"tock/internal/store"
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
	if code != 0 || strings.Count(out, "ok  ") != 8 || strings.Contains(out, "!!") {
		t.Fatalf("exit %d:\n%s%s", code, out, te.errb)
	}
	for _, want := range []string{"rules: 3 rules, 0 learned", "config: defaults", "trackd: running (version v1)",
		"accessibility: granted", "suggestions: on"} {
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
		code              int
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
