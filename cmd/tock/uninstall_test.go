package main

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// uninstallRig points uninstall at a fake install under te's dirs and
// records what it unloaded.
func uninstallRig(t *testing.T, te *tenv, linux bool) (bin string, unloaded *[]string) {
	t.Helper()
	te.Linux = linux
	bin = filepath.Join(t.TempDir(), "bin")
	for _, p := range []string{filepath.Join(bin, "tock"), filepath.Join(bin, "trackd"),
		filepath.Join(te.DataDir, "trackd.app", "Contents", "MacOS", "trackd"), filepath.Join(te.DataDir, "tock.app", "x"),
		te.dbPath()} {
		if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, nil, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	var got []string
	oldSelf, oldAgent, oldUnit, oldGOOS := selfPath, unloadAgent, unloadUnit, setupGOOS
	selfPath = func() (string, error) { return filepath.Join(bin, "tock"), nil }
	unloadAgent = func(l string) error { got = append(got, l); return nil }
	unloadUnit = func(n string) error { got = append(got, n+".service"); return nil }
	// A launchd or systemd install; Windows uninstalls through trackd.exe.
	setupGOOS = "darwin"
	if linux {
		setupGOOS = "linux"
	}
	t.Cleanup(func() { selfPath, unloadAgent, unloadUnit, setupGOOS = oldSelf, oldAgent, oldUnit, oldGOOS })
	return bin, &got
}

func exists(p string) bool { _, err := os.Stat(p); return err == nil }

func TestUninstallMac(t *testing.T) {
	te := newTestEnv(t, testRules)
	bin, unloaded := uninstallRig(t, te, false)
	code, out := te.run(t, "", "uninstall")
	if code != 0 || !slices.Equal(*unloaded, []string{"com.tock.trackd", "com.tock.bar"}) {
		t.Fatalf("code %d unloaded %v err %q", code, *unloaded, te.errb)
	}
	if exists(filepath.Join(te.DataDir, "trackd.app")) || exists(filepath.Join(te.DataDir, "tock.app")) || exists(filepath.Join(bin, "tock")) {
		t.Error("apps or tock left behind")
	}
	// Data stays unless asked.
	if !exists(te.dbPath()) || !exists(te.rulesPath()) || !strings.Contains(out, "kept") {
		t.Errorf("data removed or not said:\n%s", out)
	}
}

func TestUninstallPurge(t *testing.T) {
	te := newTestEnv(t, testRules)
	uninstallRig(t, te, true)
	if code, _ := te.run(t, "no\n", "uninstall", "--purge"); code != 1 || !exists(te.dbPath()) {
		t.Fatalf("purge without yes: code %d, db there %v", code, exists(te.dbPath()))
	}
	snap := filepath.Join(te.DataDir, "backups", "auto", "daily-20261107T031200Z-v0023.db.gz")
	if err := os.MkdirAll(filepath.Dir(snap), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(snap, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	bin, unloaded := uninstallRig(t, te, true)
	code, out := te.run(t, "yes\n", "uninstall", "--purge")
	if code != 0 || !slices.Equal(*unloaded, []string{"trackd.service"}) {
		t.Fatalf("code %d unloaded %v err %q", code, *unloaded, te.errb)
	}
	if exists(filepath.Join(bin, "trackd")) || exists(te.DataDir) || exists(te.ConfigDir) || !strings.Contains(out, "deleted") {
		t.Errorf("left behind:\n%s", out)
	}
}

func TestUninstallLeavesBrewsTock(t *testing.T) {
	te := newTestEnv(t, testRules)
	uninstallRig(t, te, false)
	cellar := filepath.Join(t.TempDir(), "Cellar", "tock", "6.0.0", "bin", "tock")
	if err := os.MkdirAll(filepath.Dir(cellar), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(cellar, nil, 0o755); err != nil {
		t.Fatal(err)
	}
	selfPath = func() (string, error) { return cellar, nil }
	code, out := te.run(t, "", "uninstall")
	if code != 0 || !exists(cellar) || !strings.Contains(out, "brew uninstall tock") {
		t.Fatalf("code %d, tock there %v:\n%s", code, exists(cellar), out)
	}
}
