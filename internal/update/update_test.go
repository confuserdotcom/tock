package update

import (
	"bytes"
	"crypto/ed25519"
	"crypto/sha256"
	"debug/buildinfo"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"tock/internal/testfs"
)

var t0 = time.Date(2026, 10, 20, 12, 0, 0, 0, time.UTC)

// src makes a dir holding fake binaries named trackd and tock.
func src(t *testing.T, trackd, tock string) string {
	t.Helper()
	d := t.TempDir()
	for name, body := range map[string]string{"trackd": trackd, "tock": tock} {
		if err := os.WriteFile(filepath.Join(d, name), []byte(body), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	return d
}

func newKey(t *testing.T) ed25519.PrivateKey {
	t.Helper()
	k, err := LoadOrCreateKey(filepath.Join(t.TempDir(), "k", "update.key"))
	if err != nil {
		t.Fatal(err)
	}
	return k
}

func sum(s string) string {
	h := sha256.Sum256([]byte(s))
	return hex.EncodeToString(h[:])
}

func read(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func TestKeyCreateAndReload(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "data")
	path := filepath.Join(dir, "update.key")
	k1, err := LoadOrCreateKey(path)
	if err != nil {
		t.Fatal(err)
	}
	fi, _ := os.Stat(path)
	if !testfs.Perm(fi, 0o600) {
		t.Fatalf("key mode = %v; want 0600", fi.Mode().Perm())
	}
	di, _ := os.Stat(dir)
	if !testfs.Perm(di, 0o700) {
		t.Fatalf("dir mode = %v; want 0700", di.Mode().Perm())
	}
	k2, err := LoadOrCreateKey(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(k1, k2) {
		t.Fatal("second call returned another key")
	}
}

func TestKeyRefusesLooseMode(t *testing.T) {
	path := filepath.Join(t.TempDir(), "update.key")
	if _, err := LoadOrCreateKey(path); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(path, 0o640); err != nil {
		t.Fatal(err)
	}
	_, err := LoadOrCreateKey(path)
	if err == nil || !strings.Contains(err.Error(), "chmod 600") {
		t.Fatalf("err = %v; want a chmod 600 refusal", err)
	}
}

func TestPublishLayoutAndVerify(t *testing.T) {
	bin, key := filepath.Join(t.TempDir(), "bin"), newKey(t)
	s := src(t, "TRACKD1", "TOCK1")
	if err := Publish(bin, "amd64", s, "v1", t0, key); err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(bin, "linux-amd64")
	for _, n := range []string{"trackd", "tock", "manifest.sig", "manifest.json"} {
		if _, err := os.Stat(filepath.Join(dir, n)); err != nil {
			t.Fatalf("missing %s: %v", n, err)
		}
	}
	if _, err := os.Stat(filepath.Join(bin, "update.pub")); err != nil {
		t.Fatal(err)
	}
	ents, _ := os.ReadDir(dir)
	for _, e := range ents {
		if strings.HasSuffix(e.Name(), ".tmp") {
			t.Fatalf("left %s behind", e.Name())
		}
	}
	mb := []byte(read(t, filepath.Join(dir, "manifest.json")))
	m, err := Verify(mb, []byte(read(t, filepath.Join(dir, "manifest.sig"))), key.Public().(ed25519.PublicKey))
	if err != nil {
		t.Fatal(err)
	}
	if m.Version != "v1" || !m.BuiltAt.Equal(t0) {
		t.Fatalf("manifest = %+v", m)
	}
	if m.Files["trackd"] != sum("TRACKD1") || m.Files["tock"] != sum("TOCK1") {
		t.Fatalf("sums = %v", m.Files)
	}
	if fi, _ := os.Stat(filepath.Join(dir, "trackd")); !testfs.Perm(fi, 0o600) {
		t.Fatalf("published file mode = %v; want 0600", fi.Mode().Perm())
	}
}

func TestPublishRefusesBadInput(t *testing.T) {
	bin, key, s := t.TempDir(), newKey(t), src(t, "a", "b")
	if err := Publish(bin, "386", s, "v1", t0, key); err == nil {
		t.Fatal("accepted arch 386")
	}
	if err := Publish(bin, "amd64", s, "", t0, key); err == nil {
		t.Fatal("accepted an empty version")
	}
	if err := Publish(bin, "amd64", s, "v1", time.Time{}, key); err == nil {
		t.Fatal("accepted a zero built_at")
	}
}

// signed returns manifest bytes for m and their signature under key.
func signed(t *testing.T, key ed25519.PrivateKey, m Manifest) (mb, sig []byte) {
	t.Helper()
	mb, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	return mb, Sign(key, mb)
}

func goodManifest() Manifest {
	return Manifest{Version: "v1", BuiltAt: t0, Files: map[string]string{"trackd": sum("a"), "tock": sum("b")}}
}

func TestVerifyRejects(t *testing.T) {
	key, other := newKey(t), newKey(t)
	pub := key.Public().(ed25519.PublicKey)
	mb, sig := signed(t, key, goodManifest())
	if _, err := Verify(mb, sig, pub); err != nil {
		t.Fatalf("good manifest: %v", err)
	}
	tampered := bytes.Replace(mb, []byte("v1"), []byte("v2"), 1)
	if _, err := Verify(tampered, sig, pub); err == nil || !strings.Contains(err.Error(), "manifest.sig") {
		t.Fatalf("tampered: err = %v", err)
	}
	mb2, sig2 := signed(t, other, goodManifest())
	if _, err := Verify(mb2, sig2, pub); err == nil {
		t.Fatal("accepted another key's signature")
	}
	bad := map[string]func(m *Manifest){
		"path name":  func(m *Manifest) { m.Files["../x"] = sum("c") },
		"other name": func(m *Manifest) { delete(m.Files, "tock"); m.Files["sh"] = sum("c") },
		"missing":    func(m *Manifest) { delete(m.Files, "tock") },
		"short sum":  func(m *Manifest) { m.Files["tock"] = "abc" },
		"upper sum":  func(m *Manifest) { m.Files["tock"] = strings.ToUpper(sum("b")) },
		"no version": func(m *Manifest) { m.Version = "" },
		"no built":   func(m *Manifest) { m.BuiltAt = time.Time{} },
	}
	for name, f := range bad {
		m := goodManifest()
		f(&m)
		mb, sig := signed(t, key, m)
		if _, err := Verify(mb, sig, pub); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
}

// (a) No key is ever taken from the sync folder by itself.
func TestPinnedNeverTrustsOnFirstUse(t *testing.T) {
	d := t.TempDir()
	pinned, offered := filepath.Join(d, "cfg", "update.pub"), filepath.Join(d, "update.pub")
	k := newKey(t)
	if err := os.WriteFile(offered, EncodePub(k.Public().(ed25519.PublicKey)), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Pinned(pinned, offered); !errors.Is(err, ErrNoTrustedKey) {
		t.Fatalf("err = %v; want ErrNoTrustedKey", err)
	}
	if _, err := os.Stat(pinned); !os.IsNotExist(err) {
		t.Fatal("a key was pinned from the sync folder")
	}
}

// (b) Trust pins only the key with the given fingerprint.
func TestTrust(t *testing.T) {
	d := t.TempDir()
	pinned, offered := filepath.Join(d, "cfg", "update.pub"), filepath.Join(d, "update.pub")
	k1, k2 := newKey(t), newKey(t)
	p1, p2 := k1.Public().(ed25519.PublicKey), k2.Public().(ed25519.PublicKey)
	if err := os.WriteFile(offered, EncodePub(p1), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, fp := range []string{Fingerprint(p2), "", "abc", Fingerprint(p1)[:63]} {
		if _, err := Trust(pinned, offered, fp); !errors.Is(err, ErrFingerprint) {
			t.Fatalf("fp %q: err = %v; want ErrFingerprint", fp, err)
		}
	}
	if _, err := os.Stat(pinned); !os.IsNotExist(err) {
		t.Fatal("pinned with a wrong fingerprint")
	}
	// Typed with capitals and colons, it still matches.
	fp := strings.ToUpper(Fingerprint(p1)[:2]) + ":" + Fingerprint(p1)[2:]
	if got, err := Trust(pinned, offered, fp); err != nil || !got.Equal(p1) {
		t.Fatalf("right fingerprint: %v %v", got, err)
	}
	if got, err := Pinned(pinned, offered); err != nil || !got.Equal(p1) {
		t.Fatalf("Pinned after Trust: %v %v", got, err)
	}
	// A different key in the sync folder is refused, and pinned stays.
	if err := os.WriteFile(offered, EncodePub(p2), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Pinned(pinned, offered); !errors.Is(err, ErrKeyChanged) {
		t.Fatalf("err = %v; want ErrKeyChanged", err)
	}
	if read(t, pinned) != string(EncodePub(p1)) {
		t.Fatal("pinned key changed")
	}
	// The offered key missing is fine: the pinned one is what counts.
	os.Remove(offered)
	if got, err := Pinned(pinned, offered); err != nil || !got.Equal(p1) {
		t.Fatalf("offered missing: %v %v", got, err)
	}
}

func TestTrustNeedsAKey(t *testing.T) {
	d := t.TempDir()
	if _, err := Trust(filepath.Join(d, "p"), filepath.Join(d, "o"), strings.Repeat("a", 64)); err == nil {
		t.Fatal("pinned nothing from nothing")
	}
	os.WriteFile(filepath.Join(d, "o"), []byte("junk\n"), 0o644)
	if _, err := Trust(filepath.Join(d, "p"), filepath.Join(d, "o"), strings.Repeat("a", 64)); err == nil {
		t.Fatal("pinned a junk key")
	}
	if _, err := os.Stat(filepath.Join(d, "p")); err == nil {
		t.Fatal("wrote a pinned file for junk")
	}
}

// published sets up a bin dir holding version v and returns it with a
// pinned key path.
func published(t *testing.T, v string, at time.Time) (bin, pinned string, key ed25519.PrivateKey) {
	t.Helper()
	bin, pinned, key = filepath.Join(t.TempDir(), "bin"), filepath.Join(t.TempDir(), "update.pub"), newKey(t)
	if err := Publish(bin, "amd64", src(t, "TRACKD-"+v, "TOCK-"+v), v, at, key); err != nil {
		t.Fatal(err)
	}
	if _, err := Trust(pinned, filepath.Join(bin, PubFile), Fingerprint(key.Public().(ed25519.PublicKey))); err != nil {
		t.Fatal(err)
	}
	return bin, pinned, key
}

func TestCheck(t *testing.T) {
	bin, pinned, _ := published(t, "v2", t0)
	// nothing published for this arch
	if _, ok, err := Check(bin, "arm64", pinned, Running{Version: "v1"}, ""); ok || err != nil {
		t.Fatalf("arm64: ok=%v err=%v", ok, err)
	}
	// same version
	if _, ok, err := Check(bin, "amd64", pinned, Running{Version: "v2", BuiltAt: t0.Add(-time.Hour)}, ""); ok || err != nil {
		t.Fatalf("same version: ok=%v err=%v", ok, err)
	}
	// not newer
	for _, at := range []time.Time{t0, t0.Add(time.Hour)} {
		if _, ok, err := Check(bin, "amd64", pinned, Running{Version: "v1", BuiltAt: at}, ""); ok || err != nil {
			t.Fatalf("running built %v: ok=%v err=%v", at, ok, err)
		}
	}
	// newer, and unknown build time
	for _, run := range []Running{{Version: "v1", BuiltAt: t0.Add(-time.Hour)}, {Version: "dev"}} {
		p, ok, err := Check(bin, "amd64", pinned, run, "")
		if !ok || err != nil {
			t.Fatalf("%+v: ok=%v err=%v", run, ok, err)
		}
		if p.Version != "v2" || p.Dir != filepath.Join(bin, "linux-amd64") || p.Sum == "" || p.Files["trackd"] != sum("TRACKD-v2") {
			t.Fatalf("plan = %+v", p)
		}
		if p.Mark() != "v2 "+p.Sum {
			t.Fatalf("mark = %q", p.Mark())
		}
	}
}

func TestCheckRefusesTheSameManifestTwice(t *testing.T) {
	bin, pinned, _ := published(t, "v2", t0)
	p, ok, err := Check(bin, "amd64", pinned, Running{Version: "v1"}, "")
	if !ok || err != nil {
		t.Fatal(ok, err)
	}
	_, ok, err = Check(bin, "amd64", pinned, Running{Version: "v1"}, p.Mark())
	if ok || err == nil || !strings.Contains(err.Error(), "still runs") {
		t.Fatalf("ok=%v err=%v; want a still runs error", ok, err)
	}
}

func TestCheckBadSignature(t *testing.T) {
	bin, pinned, key := published(t, "v2", t0)
	dir := filepath.Join(bin, "linux-amd64")
	// the manifest changed after signing (as when a sync is half done)
	mb := read(t, filepath.Join(dir, "manifest.json"))
	os.WriteFile(filepath.Join(dir, "manifest.json"), []byte(strings.Replace(mb, "v2", "v9", 1)), 0o600)
	if _, ok, err := Check(bin, "amd64", pinned, Running{Version: "v1"}, ""); ok || err == nil {
		t.Fatalf("tampered: ok=%v err=%v", ok, err)
	}
	// a missing signature is an error too: it may still be syncing
	os.WriteFile(filepath.Join(dir, "manifest.json"), []byte(mb), 0o600)
	os.Remove(filepath.Join(dir, "manifest.sig"))
	if _, ok, err := Check(bin, "amd64", pinned, Running{Version: "v1"}, ""); ok || err == nil {
		t.Fatalf("no sig: ok=%v err=%v", ok, err)
	}
	// signed again, it passes
	os.WriteFile(filepath.Join(dir, "manifest.sig"), Sign(key, []byte(mb)), 0o600)
	if _, ok, err := Check(bin, "amd64", pinned, Running{Version: "v1"}, ""); !ok || err != nil {
		t.Fatalf("resigned: ok=%v err=%v", ok, err)
	}
}

func TestCheckKeyChanged(t *testing.T) {
	bin, pinned, _ := published(t, "v2", t0)
	if _, ok, err := Check(bin, "amd64", pinned, Running{Version: "v1"}, ""); !ok || err != nil {
		t.Fatal(ok, err)
	}
	// someone publishes under another key
	if err := Publish(bin, "amd64", src(t, "evil", "evil"), "v3", t0.Add(time.Hour), newKey(t)); err != nil {
		t.Fatal(err)
	}
	if _, ok, err := Check(bin, "amd64", pinned, Running{Version: "v1"}, ""); ok || !errors.Is(err, ErrKeyChanged) {
		t.Fatalf("ok=%v err=%v; want ErrKeyChanged", ok, err)
	}
}

func TestCheckRefusesOlderSignedBuild(t *testing.T) {
	bin, pinned, _ := published(t, "v2", t0)
	run := Running{Version: "v3", BuiltAt: t0.Add(24 * time.Hour)}
	if _, ok, err := Check(bin, "amd64", pinned, run, ""); ok || err != nil {
		t.Fatalf("downgrade: ok=%v err=%v", ok, err)
	}
}

func plan(t *testing.T, v string) (Plan, string) {
	t.Helper()
	bin, pinned, _ := published(t, v, t0)
	p, ok, err := Check(bin, "amd64", pinned, Running{Version: "old"}, "")
	if !ok || err != nil {
		t.Fatal(ok, err)
	}
	return p, bin
}

func installed(t *testing.T) string {
	t.Helper()
	d := t.TempDir()
	os.WriteFile(filepath.Join(d, "trackd"), []byte("OLD-TRACKD"), 0o755)
	os.WriteFile(filepath.Join(d, "tock"), []byte("OLD-TOCK"), 0o755)
	return d
}

func noNew(t *testing.T, dir string) {
	t.Helper()
	ents, _ := os.ReadDir(dir)
	for _, e := range ents {
		if strings.HasSuffix(e.Name(), ".new") {
			t.Fatalf("left %s behind", e.Name())
		}
	}
}

func TestApplyInstalls(t *testing.T) {
	old := setUmask(0o077) // trackd runs like this
	defer setUmask(old)
	p, _ := plan(t, "v2")
	dir := installed(t)
	calls := 0
	err := Apply(p, dir, func() error {
		calls++
		// both copies are staged, nothing is replaced yet
		for _, n := range []string{"trackd.new", "tock.new"} {
			if _, err := os.Stat(filepath.Join(dir, n)); err != nil {
				t.Errorf("mark before %s was staged", n)
			}
		}
		if read(t, filepath.Join(dir, "trackd")) != "OLD-TRACKD" || read(t, filepath.Join(dir, "tock")) != "OLD-TOCK" {
			t.Error("mark after a rename")
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if calls != 1 {
		t.Fatalf("mark called %d times", calls)
	}
	if read(t, filepath.Join(dir, "trackd")) != "TRACKD-v2" || read(t, filepath.Join(dir, "tock")) != "TOCK-v2" {
		t.Fatal("new contents not installed")
	}
	for _, n := range Names {
		if fi, _ := os.Stat(filepath.Join(dir, n)); !testfs.Perm(fi, 0o755) {
			t.Fatalf("%s mode = %v; want 0755", n, fi.Mode().Perm())
		}
	}
	noNew(t, dir)
}

func TestApplyHashMismatch(t *testing.T) {
	p, bin := plan(t, "v2")
	os.WriteFile(filepath.Join(bin, "linux-amd64", "trackd"), []byte("TAMPERED"), 0o600)
	dir := installed(t)
	err := Apply(p, dir, func() error { t.Error("mark called"); return nil })
	if err == nil || !strings.Contains(err.Error(), "does not match the manifest") {
		t.Fatalf("err = %v", err)
	}
	if read(t, filepath.Join(dir, "trackd")) != "OLD-TRACKD" || read(t, filepath.Join(dir, "tock")) != "OLD-TOCK" {
		t.Fatal("binDir changed")
	}
	noNew(t, dir)
}

func TestApplyMarkFails(t *testing.T) {
	p, _ := plan(t, "v2")
	dir := installed(t)
	err := Apply(p, dir, func() error { return errors.New("db is locked") })
	if err == nil || !strings.Contains(err.Error(), "db is locked") {
		t.Fatalf("err = %v", err)
	}
	if read(t, filepath.Join(dir, "trackd")) != "OLD-TRACKD" || read(t, filepath.Join(dir, "tock")) != "OLD-TOCK" {
		t.Fatal("renamed after a failed mark")
	}
	noNew(t, dir)
}

func TestPublishedSkipsBadManifests(t *testing.T) {
	bin := filepath.Join(t.TempDir(), "bin")
	key := newKey(t)
	if _, ok := Published(bin); ok {
		t.Fatal("found a build in nothing")
	}
	Publish(bin, "amd64", src(t, "a", "b"), "v1", t0, key)
	Publish(bin, "arm64", src(t, "a", "b"), "v2", t0.Add(time.Hour), key)
	os.MkdirAll(filepath.Join(bin, "linux-riscv64"), 0o700)
	os.WriteFile(filepath.Join(bin, "linux-riscv64", "manifest.json"), []byte("{"), 0o600)
	m, ok := Published(bin)
	if !ok || m.Version != "v2" {
		t.Fatalf("published = %+v %v", m, ok)
	}
}

func TestBootstrapCommand(t *testing.T) {
	m := Manifest{Files: map[string]string{"trackd": sum("T"), "tock": sum("K")}}
	got := BootstrapCommand("/home/a/Sync/tock", "/home/a", "amd64", m, "ff00")
	for _, want := range []string{"cp ~/Sync/tock/bin/linux-amd64/trackd trackd.new", sum("T") + " trackd.new", sum("K") + " tock.new",
		"sha256sum -c", "./tock update --sync ~/Sync/tock trust ff00", "./trackd install"} {
		if !strings.Contains(got, want) {
			t.Errorf("lacks %q:\n%s", want, got)
		}
	}
	if got := BootstrapCommand("/mnt/s", "/home/a", "arm64", m, "ff00"); !strings.Contains(got, "/mnt/s/bin/linux-arm64/trackd") {
		t.Fatalf("got %s", got)
	}
}

// (e) The printed command installs nothing when a binary in the sync
// folder was altered, and installs both when they match.
func TestBootstrapCommandRuns(t *testing.T) {
	if _, err := exec.LookPath("sha256sum"); err != nil {
		t.Skip("no sha256sum")
	}
	home := t.TempDir()
	syncDir := filepath.Join(home, "Sync")
	key := newKey(t)
	trackd := "#!/bin/sh\necho trackd \"$@\" >> \"$HOME/ran\"\n"
	tock := "#!/bin/sh\necho tock \"$@\" >> \"$HOME/ran\"\n"
	if err := Publish(filepath.Join(syncDir, BinDir), "amd64", src(t, trackd, tock), "v2", t0, key); err != nil {
		t.Fatal(err)
	}
	cmd, err := Bootstrap(syncDir, home, "amd64", key.Public().(ed25519.PublicKey))
	if err != nil {
		t.Fatal(err)
	}
	run := func() error {
		c := exec.Command("sh", "-c", cmd)
		c.Env = append(os.Environ(), "HOME="+home)
		out, err := c.CombinedOutput()
		if err != nil {
			t.Logf("%s", out)
		}
		return err
	}
	bin := filepath.Join(syncDir, BinDir, "linux-amd64", "trackd")
	if err := os.WriteFile(bin, []byte("#!/bin/sh\necho evil\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	if run() == nil {
		t.Fatal("altered build: command succeeded")
	}
	for _, n := range []string{"trackd", "tock", "trackd.new", "tock.new"} {
		if _, err := os.Stat(filepath.Join(home, ".local", "bin", n)); err == nil {
			t.Fatalf("altered build: %s left in ~/.local/bin", n)
		}
	}
	if err := os.WriteFile(bin, []byte(trackd), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := run(); err != nil {
		t.Fatalf("matching build: %v", err)
	}
	ran := read(t, filepath.Join(home, "ran"))
	if !strings.Contains(ran, " trust "+Fingerprint(key.Public().(ed25519.PublicKey))) || !strings.Contains(ran, "trackd install") {
		t.Fatalf("ran:\n%s", ran)
	}
}

// Bootstrap refuses a manifest the Mac's key did not sign.
func TestBootstrapChecksTheManifest(t *testing.T) {
	syncDir := t.TempDir()
	key, other := newKey(t), newKey(t)
	if err := Publish(filepath.Join(syncDir, BinDir), "amd64", src(t, "a", "b"), "v2", t0, other); err != nil {
		t.Fatal(err)
	}
	if _, err := Bootstrap(syncDir, "", "amd64", key.Public().(ed25519.PublicKey)); err == nil {
		t.Fatal("printed a command for a manifest signed by another key")
	}
}

// (c) A rename that fails part way puts every binary back as it was.
func TestApplyRollsBackAFailedRename(t *testing.T) {
	p, _ := plan(t, "v2")
	dir := installed(t)
	old := rename
	t.Cleanup(func() { rename = old })
	rename = func(from, to string) error {
		if to == filepath.Join(dir, Names[1]) && strings.HasSuffix(from, ".new") {
			return errors.New("injected")
		}
		return os.Rename(from, to)
	}
	if err := Apply(p, dir, func() error { return nil }); err == nil || !strings.Contains(err.Error(), "injected") {
		t.Fatalf("err = %v", err)
	}
	if read(t, filepath.Join(dir, "trackd")) != "OLD-TRACKD" || read(t, filepath.Join(dir, "tock")) != "OLD-TOCK" {
		t.Fatal("binaries not put back")
	}
	ents, _ := os.ReadDir(dir)
	for _, e := range ents {
		if strings.HasSuffix(e.Name(), ".new") || strings.HasSuffix(e.Name(), ".old") {
			t.Fatalf("left %s behind", e.Name())
		}
	}
}

func TestParseRunning(t *testing.T) {
	r := ParseRunning("v1", "2026-10-20T12:00:00Z")
	if r.Version != "v1" || !r.BuiltAt.Equal(t0) {
		t.Fatalf("%+v", r)
	}
	for _, s := range []string{"", "yesterday"} {
		if r := ParseRunning("v1", s); !r.BuiltAt.IsZero() {
			t.Fatalf("%q gave %v", s, r.BuiltAt)
		}
	}
}

// TestPublishedTree checks a tree that make publish-linux made:
// TOCK_PUBLISHED is its bin dir. The installed trackd must carry the
// manifest's version in its build info.
func TestPublishedTree(t *testing.T) {
	bin := os.Getenv("TOCK_PUBLISHED")
	if bin == "" {
		t.Skip("TOCK_PUBLISHED is not set")
	}
	pinned := filepath.Join(t.TempDir(), "update.pub")
	p, ok, err := Check(bin, "amd64", pinned, Running{Version: "old"}, "")
	if !ok || err != nil {
		t.Fatalf("check: ok=%v err=%v", ok, err)
	}
	dir := installed(t)
	if err := Apply(p, dir, func() error { return nil }); err != nil {
		t.Fatal(err)
	}
	bi, err := buildinfo.ReadFile(filepath.Join(dir, "trackd"))
	if err != nil {
		t.Fatal(err)
	}
	var flags string
	for _, s := range bi.Settings {
		if s.Key == "-ldflags" {
			flags = s.Value
		}
	}
	if !strings.Contains(flags, "main.version="+p.Version) {
		t.Fatalf("-ldflags = %q; want main.version=%s", flags, p.Version)
	}
}
