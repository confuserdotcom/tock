package main

import (
	"bytes"
	"crypto/ed25519"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"tock/internal/collect"
	"tock/internal/store"
	"tock/internal/testfs"
	"tock/internal/update"
)

var updBuilt = time.Date(2026, 10, 20, 12, 0, 0, 0, time.UTC)

// updFix is a Linux machine with a sync folder holding a published build.
type updFix struct {
	t        *testing.T
	dir      string
	cfgPath  string
	pinned   string
	binDir   string
	syncBin  string
	key      ed25519.PrivateKey
	st       *store.Store
	logb     *bytes.Buffer
	restarts int
	arch     string
	noTrust  bool // publish leaves the key unpinned
	newLoop  func() *updateLoop
}

// newUpdFix makes the fixture. cfg is config.toml, with SYNC for the sync
// dir; "" is a config that sets it.
func newUpdFix(t *testing.T, cfg string) *updFix {
	t.Helper()
	dir := t.TempDir()
	f := &updFix{t: t, dir: dir, arch: "amd64", logb: &bytes.Buffer{}}
	f.cfgPath = filepath.Join(dir, "config", "config.toml")
	f.pinned = filepath.Join(dir, "config", "update.pub")
	f.binDir = filepath.Join(dir, "local-bin")
	f.syncBin = filepath.Join(dir, "sync", update.BinDir)
	for _, d := range []string{filepath.Dir(f.cfgPath), f.binDir} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if cfg == "" {
		cfg = "[sync]\ndir = \"SYNC\"\nupdate = true\n"
	}
	cfg = strings.ReplaceAll(cfg, "SYNC", filepath.Join(dir, "sync"))
	if err := os.WriteFile(f.cfgPath, []byte(cfg), 0o600); err != nil {
		t.Fatal(err)
	}
	for n, body := range map[string]string{"trackd": "OLD-TRACKD", "tock": "OLD-TOCK"} {
		if err := os.WriteFile(filepath.Join(f.binDir, n), []byte(body), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	var err error
	if f.key, err = update.LoadOrCreateKey(filepath.Join(dir, "key", "update.key")); err != nil {
		t.Fatal(err)
	}
	if f.st, err = store.Open(filepath.Join(dir, "tock.db")); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { f.st.Close() })
	f.newLoop = func() *updateLoop {
		return &updateLoop{cfgPath: f.cfgPath, pinned: f.pinned, binDir: f.binDir, arch: f.arch,
			run: update.Running{Version: "v1"}, st: f.st, log: slog.New(slog.NewTextHandler(f.logb, nil)),
			now: func() int64 { return 1000 }, restart: func() { f.restarts++ }}
	}
	return f
}

// publish puts a build with version v and the given trackd body in the sync folder.
func (f *updFix) publish(v, trackd string) {
	f.t.Helper()
	src := f.t.TempDir()
	os.WriteFile(filepath.Join(src, "trackd"), []byte(trackd), 0o755)
	os.WriteFile(filepath.Join(src, "tock"), []byte("TOCK-"+v), 0o755)
	if err := update.Publish(f.syncBin, f.arch, src, v, updBuilt, f.key); err != nil {
		f.t.Fatal(err)
	}
	if !f.noTrust {
		f.trust()
	}
}

// trust pins the fixture's key the way tock update trust does.
func (f *updFix) trust() {
	f.t.Helper()
	fp := update.Fingerprint(f.key.Public().(ed25519.PublicKey))
	if _, err := update.Trust(f.pinned, filepath.Join(f.syncBin, update.PubFile), fp); err != nil {
		f.t.Fatal(err)
	}
}

func (f *updFix) state(key string) string {
	f.t.Helper()
	v, _, err := f.st.State(key)
	if err != nil {
		f.t.Fatal(err)
	}
	return v
}

func (f *updFix) body(name string) string {
	f.t.Helper()
	b, err := os.ReadFile(filepath.Join(f.binDir, name))
	if err != nil {
		f.t.Fatal(err)
	}
	return string(b)
}

func TestUpdateLoopInstalls(t *testing.T) {
	testfs.UnixOnly(t, "self-update over the sync folder is Linux only")
	f := newUpdFix(t, "")
	f.publish("v2", "TRACKD-v2")
	u := f.newLoop()
	u.tick()
	if f.body("trackd") != "TRACKD-v2" || f.body("tock") != "TOCK-v2" {
		t.Fatalf("binaries: trackd=%q tock=%q", f.body("trackd"), f.body("tock"))
	}
	if f.restarts != 1 {
		t.Fatalf("restarts = %d; want 1", f.restarts)
	}
	if got := f.state(update.KeyApplied); !strings.HasPrefix(got, "v2 ") || len(got) != len("v2 ")+64 {
		t.Fatalf("update_applied = %q", got)
	}
	if got := f.state(update.KeyError); got != "" {
		t.Fatalf("update_error = %q", got)
	}
	u.tick() // done: nothing more
	if f.restarts != 1 {
		t.Fatalf("restarts after a second tick = %d", f.restarts)
	}
}

func TestUpdateLoopNeverAppliesTheSameManifestTwice(t *testing.T) {
	testfs.UnixOnly(t, "self-update over the sync folder is Linux only")
	f := newUpdFix(t, "")
	f.publish("v2", "TRACKD-v2")
	f.newLoop().tick()
	// the install happened but the new process still says v1
	again := f.newLoop()
	again.tick()
	if f.restarts != 1 {
		t.Fatalf("restarts = %d; want 1", f.restarts)
	}
	if got := f.state(update.KeyError); !strings.Contains(got, "still runs") {
		t.Fatalf("update_error = %q; want a still runs error", got)
	}
}

func TestUpdateLoopOff(t *testing.T) {
	for name, cfg := range map[string]string{
		"update = false": "[sync]\ndir = \"SYNC\"\nupdate = false\n",
		"update not set": "[sync]\ndir = \"SYNC\"\n", // (d) opt-in, even with a pinned key
		"no sync dir":    "# nothing\n",
	} {
		t.Run(name, func(t *testing.T) {
			f := newUpdFix(t, cfg)
			f.publish("v2", "TRACKD-v2")
			f.newLoop().tick()
			if f.restarts != 0 || f.body("trackd") != "OLD-TRACKD" {
				t.Fatalf("installed with updates off (restarts %d)", f.restarts)
			}
			if m, err := f.st.States(); err != nil || len(m) != 0 {
				t.Fatalf("state = %v, %v; want none", m, err)
			}
		})
	}
}

// (a) A key and a signed build in the sync folder are not enough: with no
// key pinned on this machine, nothing is installed.
func TestUpdateLoopNoTrustedKey(t *testing.T) {
	testfs.UnixOnly(t, "self-update over the sync folder is Linux only")
	f := newUpdFix(t, "")
	f.noTrust = true
	f.publish("v2", "TRACKD-v2")
	f.newLoop().tick()
	if f.restarts != 0 || f.body("trackd") != "OLD-TRACKD" || f.body("tock") != "OLD-TOCK" {
		t.Fatal("installed without a trusted key")
	}
	if !strings.Contains(f.logb.String(), "no trusted key") {
		t.Fatalf("log lacks no trusted key:\n%s", f.logb.String())
	}
	if _, err := os.Stat(f.pinned); !os.IsNotExist(err) {
		t.Fatal("pinned the key from the sync folder")
	}
}

func TestUpdateLoopKeyChanged(t *testing.T) {
	testfs.UnixOnly(t, "self-update over the sync folder is Linux only")
	f := newUpdFix(t, "")
	f.publish("v2", "TRACKD-v2")
	other, err := update.LoadOrCreateKey(filepath.Join(f.dir, "other", "update.key"))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(f.pinned, update.EncodePub(other.Public().(ed25519.PublicKey)), 0o644); err != nil {
		t.Fatal(err)
	}
	u := f.newLoop()
	u.tick()
	u.tick()
	if got := f.state(update.KeyError); !strings.Contains(got, "key changed") {
		t.Fatalf("update_error = %q", got)
	}
	if f.restarts != 0 || f.body("trackd") != "OLD-TRACKD" || f.body("tock") != "OLD-TOCK" {
		t.Fatal("installed under a changed key")
	}
	if n := strings.Count(f.logb.String(), "key changed"); n != 1 {
		t.Fatalf("warned %d times; want once\n%s", n, f.logb.String())
	}
}

func TestUpdateLoopHashMismatchRetries(t *testing.T) {
	testfs.UnixOnly(t, "self-update over the sync folder is Linux only")
	f := newUpdFix(t, "")
	f.publish("v2", "TRACKD-v2")
	// the file arrives different from what the manifest says
	if err := os.WriteFile(filepath.Join(f.syncBin, "linux-amd64", "trackd"), []byte("TAMPERED"), 0o600); err != nil {
		t.Fatal(err)
	}
	u := f.newLoop()
	u.tick()
	if f.restarts != 0 || f.body("trackd") != "OLD-TRACKD" || f.body("tock") != "OLD-TOCK" {
		t.Fatal("installed a mismatching file")
	}
	if got := f.state(update.KeyApplied); got != "" {
		t.Fatalf("update_applied = %q; want none", got)
	}
	if got := f.state(update.KeyError); !strings.Contains(got, "does not match the manifest") {
		t.Fatalf("update_error = %q", got)
	}
	// published again, correctly: the next tick takes it
	f.publish("v2", "TRACKD-v2")
	u.tick()
	if f.restarts != 1 || f.body("trackd") != "TRACKD-v2" {
		t.Fatalf("not retried: restarts %d, trackd %q", f.restarts, f.body("trackd"))
	}
	if got := f.state(update.KeyError); got != "" {
		t.Fatalf("update_error = %q after success", got)
	}
}

func TestRunDaemonReturnsExitUpdated(t *testing.T) {
	testfs.UnixOnly(t, "self-update over the sync folder is Linux only")
	if runtime.GOARCH != "amd64" && runtime.GOARCH != "arm64" {
		t.Skip("no published build for " + runtime.GOARCH)
	}
	f := newUpdFix(t, "")
	f.arch = runtime.GOARCH
	f.publish("v2", "TRACKD-v2")
	old := version
	version = "v1"
	defer func() { version = old }()

	data := t.TempDir()
	var buf bytes.Buffer
	rc := runDaemon(daemonOpts{
		DataDir: data, StateDir: data,
		NewSource: func() (collect.Source, error) { return samplesSource(), nil },
		Stderr:    &buf,
		ConfigDir: filepath.Dir(f.cfgPath),
		Exe:       filepath.Join(f.binDir, "trackd"),
	})
	if rc != exitUpdated {
		t.Fatalf("rc = %d; want exitUpdated\n%s", rc, buf.String())
	}
	if f.body("trackd") != "TRACKD-v2" {
		t.Fatalf("trackd = %q", f.body("trackd"))
	}
	if strings.Contains(buf.String(), "TRACKD-v2") {
		t.Fatalf("log holds file contents:\n%s", buf.String())
	}
}
