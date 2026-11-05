package main

import (
	"crypto/x509"
	"encoding/pem"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// fakeMac is the keychain, codesign and launchctl that tock setup talks to.
type fakeMac struct {
	cert, trusted, declineTrust bool
	certPEM                     string
	loaded                      map[string]bool
	signed                      map[string]bool
	calls                       []string
}

func (f *fakeMac) run(name string, args ...string) (string, error) {
	call := name + " " + strings.Join(args, " ")
	f.calls = append(f.calls, call)
	no := errors.New("exit status 1")
	switch {
	case name == "security" && args[0] == "find-certificate":
		if !f.cert {
			return "", no
		}
		if slicesContains(args, "-p") {
			return f.certPEM, nil
		}
		return "keychain: login", nil
	case name == "security" && args[0] == "import":
		b, err := os.ReadFile(args[1])
		if err != nil {
			return "", err
		}
		if slicesContains(args, "cert") {
			f.cert, f.certPEM = true, string(b)
		}
		return "1 imported.", nil
	case name == "security" && args[0] == "find-identity":
		if f.trusted {
			return `  1) ABCD "tock-local-codesign"` + "\n     1 valid identities found", nil
		}
		return "     0 valid identities found", nil
	case name == "security" && args[0] == "add-trusted-cert":
		if f.declineTrust {
			return "SecTrustSettingsSetTrustSettings: The authorization was canceled by the user.", no
		}
		f.trusted = true
		return "", nil
	case name == "launchctl" && args[0] == "bootout":
		l := args[1][strings.LastIndex(args[1], "/")+1:]
		if !f.loaded[l] {
			return "", no
		}
		f.loaded[l] = false
		return "", nil
	case name == "launchctl" && args[0] == "print":
		if f.loaded[args[1][strings.LastIndex(args[1], "/")+1:]] {
			return "state = running", nil
		}
		return "", no
	case name == "codesign" && args[0] == "--force":
		f.signed[args[len(args)-1]] = true
		return "", nil
	case name == "codesign" && args[0] == "--verify":
		if !f.signed[args[len(args)-1]] {
			return "not signed", no
		}
		return "", nil
	case name == "codesign" && args[0] == "-dvv":
		if f.signed[args[1]] {
			return "Authority=tock-local-codesign", nil
		}
		return "", no
	case strings.HasSuffix(name, "lsregister"):
		return "", nil
	case strings.HasSuffix(name, "/trackd") && args[0] == "install":
		f.loaded["com.tock.trackd"] = true
		return "", nil
	case strings.HasSuffix(name, "/tockbar") && args[0] == "install":
		f.loaded["com.tock.bar"] = true
		return "", nil
	}
	return "", errors.New("unexpected command: " + call)
}

func slicesContains(s []string, v string) bool {
	for _, x := range s {
		if x == v {
			return true
		}
	}
	return false
}

func (f *fakeMac) ran(prefix string) int {
	n := 0
	for _, c := range f.calls {
		if strings.HasPrefix(c, prefix) {
			n++
		}
	}
	return n
}

// setupRig points tock setup at a fake payload, home and system.
func setupRig(t *testing.T, te *tenv, goos string) (*fakeMac, string) {
	t.Helper()
	from := filepath.Join(t.TempDir(), "libexec", "tock")
	names := macPayload()
	if goos == "linux" {
		names = []string{"trackd"}
	}
	for _, n := range names {
		if err := os.MkdirAll(from, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(from, n), []byte(n), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	home := t.TempDir()
	f := &fakeMac{loaded: map[string]bool{}, signed: map[string]bool{}}
	oldRun, oldOS, oldHome, oldNow := setupRun, setupGOOS, setupHome, setupNow
	setupRun, setupGOOS = f.run, goos
	setupHome = func() (string, error) { return home, nil }
	setupNow = func() time.Time { return time.Date(2026, 11, 5, 12, 0, 0, 0, time.UTC) }
	t.Cleanup(func() { setupRun, setupGOOS, setupHome, setupNow = oldRun, oldOS, oldHome, oldNow })
	return f, from
}

func TestSetupFreshMac(t *testing.T) {
	te := newTestEnv(t, testRules)
	f, from := setupRig(t, te, "darwin")
	if err := os.Remove(te.rulesPath()); err != nil {
		t.Fatal(err)
	}
	code, out := te.run(t, "", "setup", "--from", from, "--no-doctor")
	if code != 0 {
		t.Fatalf("code %d: %s %s\ncalls %q", code, out, te.errb, f.calls)
	}
	for _, want := range []string{"security add-trusted-cert", "codesign --force --sign tock-local-codesign --identifier com.tock.trackd",
		"codesign --force --sign tock-local-codesign --identifier com.tock.bar"} {
		if f.ran(want) != 1 {
			t.Errorf("%q ran %d times\n%q", want, f.ran(want), f.calls)
		}
	}
	if f.ran("security import") != 2 || !f.loaded["com.tock.trackd"] || !f.loaded["com.tock.bar"] {
		t.Fatalf("imports %d loaded %v", f.ran("security import"), f.loaded)
	}
	// the apps are in place, with the right modes
	for p, mode := range map[string]os.FileMode{
		"trackd.app/Contents/MacOS/trackd":             0o755,
		"trackd.app/Contents/Info.plist":               0o644,
		"tock.app/Contents/MacOS/tockbar":              0o755,
		"tock.app/Contents/Resources/MenuGlyph@2x.png": 0o644,
	} {
		fi, err := os.Stat(filepath.Join(te.DataDir, p))
		if err != nil || fi.Mode().Perm() != mode {
			t.Errorf("%s: %v %v", p, fi, err)
		}
	}
	if b, _ := os.ReadFile(filepath.Join(te.DataDir, "trackd.app/Contents/Info.plist")); string(b) != "trackd.plist" {
		t.Errorf("trackd Info.plist = %q", b)
	}
	if exists(filepath.Join(te.DataDir, "trackd.app.new")) {
		t.Error("left the staging folder behind")
	}
	if !exists(te.rulesPath()) {
		t.Error("no starter rules")
	}
	if b, err := os.ReadFile(filepath.Join(te.DataDir, "codesign", "tock-local-codesign.pem")); err != nil || !strings.Contains(string(b), "BEGIN CERTIFICATE") {
		t.Errorf("certificate not kept for trust: %v", err)
	}
	if !strings.Contains(out, "password") {
		t.Errorf("did not explain the password prompt:\n%s", out)
	}
}

func TestSetupAgainKeepsTheCertAndRestartsTheApps(t *testing.T) {
	te := newTestEnv(t, testRules)
	f, from := setupRig(t, te, "darwin")
	if code, _ := te.run(t, "", "setup", "--from", from, "--no-doctor"); code != 0 {
		t.Fatalf("first setup failed: %s", te.errb)
	}
	f.calls = nil
	if code, _ := te.run(t, "", "setup", "--from", from, "--no-doctor"); code != 0 {
		t.Fatalf("second setup failed: %s", te.errb)
	}
	if f.ran("security import") != 0 || f.ran("security add-trusted-cert") != 0 {
		t.Fatalf("made or trusted the cert again: %q", f.calls)
	}
	// each agent was stopped before its app was replaced
	for _, l := range []string{"com.tock.trackd", "com.tock.bar"} {
		stop, sign := -1, -1
		for i, c := range f.calls {
			if strings.HasPrefix(c, "launchctl bootout") && strings.HasSuffix(c, l) {
				stop = i
			}
			if strings.HasPrefix(c, "codesign --force") && strings.Contains(c, l) {
				sign = i
			}
		}
		if stop < 0 || sign < stop {
			t.Errorf("%s: bootout at %d, signed at %d", l, stop, sign)
		}
	}
}

func TestSetupTrustDeclined(t *testing.T) {
	te := newTestEnv(t, testRules)
	f, from := setupRig(t, te, "darwin")
	f.declineTrust = true
	code, _ := te.run(t, "", "setup", "--from", from, "--no-doctor")
	if code != 1 || !strings.Contains(te.errb.String(), "run tock setup again") {
		t.Fatalf("code %d err %q", code, te.errb)
	}
	if f.ran("codesign --force") != 0 || exists(filepath.Join(te.DataDir, "tock.app")) {
		t.Fatal("installed the apps without a trusted certificate")
	}
}

func TestSetupMissingPayload(t *testing.T) {
	te := newTestEnv(t, testRules)
	_, from := setupRig(t, te, "darwin")
	if err := os.Remove(filepath.Join(from, "tockbar")); err != nil {
		t.Fatal(err)
	}
	code, _ := te.run(t, "", "setup", "--from", from, "--no-doctor")
	if code != 1 || !strings.Contains(te.errb.String(), "tockbar is missing") || !strings.Contains(te.errb.String(), "brew reinstall tock") {
		t.Fatalf("code %d err %q", code, te.errb)
	}
}

func TestSetupFindsItsFilesNextToTock(t *testing.T) {
	te := newTestEnv(t, testRules)
	_, from := setupRig(t, te, "darwin")
	bin := filepath.Join(filepath.Dir(filepath.Dir(from)), "bin")
	old := selfPath
	selfPath = func() (string, error) { return filepath.Join(bin, "tock"), nil }
	t.Cleanup(func() { selfPath = old })
	if code, _ := te.run(t, "", "setup", "--no-doctor"); code != 0 {
		t.Fatalf("setup with the default folder: %s", te.errb)
	}
}

func TestSetupCheck(t *testing.T) {
	te := newTestEnv(t, testRules)
	_, from := setupRig(t, te, "darwin")
	code, out := te.run(t, "", "setup", "--check", "--from", from)
	if code != 1 || strings.Count(out, "missing") != 6 || !strings.Contains(out, "ok       files to install") {
		t.Fatalf("check before setup: code %d\n%s", code, out)
	}
	if c, _ := te.run(t, "", "setup", "--from", from, "--no-doctor"); c != 0 {
		t.Fatal(te.errb)
	}
	code, out = te.run(t, "", "setup", "--check", "--from", from)
	if code != 0 || strings.Contains(out, "missing") || !strings.Contains(out, "tock is set up.") {
		t.Fatalf("check after setup: code %d\n%s", code, out)
	}
}

func TestSetupLinux(t *testing.T) {
	te := newTestEnv(t, testRules)
	f, from := setupRig(t, te, "linux")
	home, _ := setupHome()
	var installed bool
	inner := setupRun
	setupRun = func(name string, args ...string) (string, error) {
		if name == filepath.Join(home, ".local", "bin", "trackd") && args[0] == "install" {
			installed = true
			return "", nil
		}
		return inner(name, args...)
	}
	if code, _ := te.run(t, "", "setup", "--from", from, "--no-doctor"); code != 0 || !installed {
		t.Fatalf("code %d installed %v err %q calls %q", code, installed, te.errb, f.calls)
	}
	if fi, err := os.Stat(filepath.Join(home, ".local", "bin", "trackd")); err != nil || fi.Mode().Perm() != 0o755 {
		t.Fatalf("trackd not installed: %v %v", fi, err)
	}
}

func TestNewSigningCert(t *testing.T) {
	now := time.Date(2026, 11, 5, 12, 0, 0, 0, time.UTC)
	key, crt, err := newSigningCert(now, nil)
	if err != nil {
		t.Fatal(err)
	}
	kb, _ := pem.Decode(key)
	if kb == nil || kb.Type != "RSA PRIVATE KEY" {
		t.Fatalf("key PEM %v", kb)
	}
	if _, err := x509.ParsePKCS1PrivateKey(kb.Bytes); err != nil {
		t.Fatal(err)
	}
	cb, _ := pem.Decode(crt)
	c, err := x509.ParseCertificate(cb.Bytes)
	if err != nil {
		t.Fatal(err)
	}
	if c.Subject.CommonName != signName || c.IsCA || len(c.ExtKeyUsage) != 1 || c.ExtKeyUsage[0] != x509.ExtKeyUsageCodeSigning ||
		c.KeyUsage != x509.KeyUsageDigitalSignature || c.NotAfter.Year() != 2036 {
		t.Fatalf("cert = %+v", c)
	}
	if err := c.CheckSignature(c.SignatureAlgorithm, c.RawTBSCertificate, c.Signature); err != nil {
		t.Fatalf("not self-signed: %v", err)
	}
}

func TestSetupCLICopiesTock(t *testing.T) {
	te := newTestEnv(t, testRules)
	_, from := setupRig(t, te, "darwin")
	self := filepath.Join(from, "tock")
	if err := os.WriteFile(self, []byte("tock binary"), 0o755); err != nil {
		t.Fatal(err)
	}
	old := selfPath
	selfPath = func() (string, error) { return self, nil }
	t.Cleanup(func() { selfPath = old })
	home, _ := setupHome()
	if code, out := te.run(t, "", "setup", "--from", from, "--no-doctor", "--cli"); code != 0 {
		t.Fatalf("code %d: %s %s", code, out, te.errb)
	}
	dst := filepath.Join(home, ".local", "bin", "tock")
	fi, err := os.Stat(dst)
	if err != nil || fi.Mode().Perm() != 0o755 {
		t.Fatalf("%s: %v %v", dst, fi, err)
	}
	if b, _ := os.ReadFile(dst); string(b) != "tock binary" {
		t.Fatalf("copied %q", b)
	}
	// without --cli nothing goes there
	if err := os.Remove(dst); err != nil {
		t.Fatal(err)
	}
	if code, _ := te.run(t, "", "setup", "--from", from, "--no-doctor"); code != 0 {
		t.Fatalf("second setup failed: %s", te.errb)
	}
	if exists(dst) {
		t.Fatal("setup without --cli copied tock")
	}
}
