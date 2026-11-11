package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	"tock/internal/classify"
)

// Seams for tests: commands tock setup runs, the OS, the home folder and
// the clock the certificate is dated by.
var (
	setupRun  = runCmd
	setupGOOS = runtime.GOOS
	setupHome = os.UserHomeDir
	setupNow  = time.Now
)

// setupTimeout stops a command that hangs, such as codesign waiting on a
// keychain dialog nobody answers.
const setupTimeout = 5 * time.Minute

func runCmd(name string, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), setupTimeout)
	defer cancel()
	out, err := exec.CommandContext(ctx, name, args...).CombinedOutput()
	if ctx.Err() != nil {
		return string(out), fmt.Errorf("%s timed out after %s", name, setupTimeout)
	}
	return string(out), err
}

// The macOS apps tock setup assembles. The bundle paths and ids are what
// Accessibility and notifications key on: never change them (see Makefile).
type macApp struct {
	bundle, id, label, binary, plist string
	resources                        []string
}

var macApps = []macApp{
	{bundle: "trackd.app", id: "com.tock.trackd", label: "com.tock.trackd", binary: "trackd", plist: "trackd.plist"},
	{bundle: "tock.app", id: "com.tock.bar", label: "com.tock.bar", binary: "tockbar", plist: "tockbar.plist",
		resources: []string{"AppIcon.icns", "MenuGlyph.png", "MenuGlyph@2x.png"}},
}

// cmdSetup installs tock from the files a package (Homebrew) put next to it:
// on macOS the signing certificate, the two apps and their LaunchAgents; on
// Linux trackd and its systemd unit. Then the starter rules and tock doctor.
// Running it again is safe: it keeps the certificate and replaces the apps.
func cmdSetup(env Env, args []string) error {
	fs := newFlags("setup")
	check := fs.Bool("check", false, "only report what is missing")
	from := fs.String("from", "", "folder with the files to install (default: ../libexec/tock next to tock)")
	noDoctor := fs.Bool("no-doctor", false, "skip tock doctor at the end")
	cli := fs.Bool("cli", false, "also copy this tock to ~/.local/bin (the .dmg does)")
	tpl := fs.String("template", "", "only apply rule templates: "+strings.Join(classify.TemplateNames(), ",")+" (see tock help setup)")
	past := fs.Bool("past", false, "with --template: label past activity too (default: from now on)")
	yes := fs.Bool("yes", false, "with --template: apply without asking")
	noPrompt := fs.Bool("no-prompt", false, "on a fresh install, skip the template question (applies own)")
	if err := parseFlags(fs, args); err != nil {
		return err
	}
	if fs.NArg() > 0 {
		return usagef("setup: unexpected argument %q", fs.Arg(0))
	}
	if *tpl != "" {
		if _, err := os.Stat(env.rulesPath()); errors.Is(err, os.ErrNotExist) {
			if err := cmdInit(env, nil); err != nil {
				return err
			}
		}
		return setupTemplates(env, strings.Split(*tpl, ","), *past, *yes)
	}
	dir := *from
	if dir == "" {
		self, err := selfPath()
		if err != nil {
			return fmt.Errorf("setup: %w", err)
		}
		if r, err := filepath.EvalSymlinks(self); err == nil {
			self = r
		}
		dir = filepath.Join(filepath.Dir(self), "..", "libexec", "tock")
	}
	home, err := setupHome()
	if err != nil {
		return fmt.Errorf("setup: %w", err)
	}
	s := &setup{env: env, from: filepath.Clean(dir), home: home}
	if setupGOOS == "linux" {
		if *check {
			return s.checkLinux()
		}
		err = s.linux()
	} else {
		if *check {
			return s.checkMac()
		}
		err = s.mac()
	}
	if err != nil {
		return err
	}
	if *cli {
		if err := s.copyCLI(); err != nil {
			return err
		}
	}
	if _, err := os.Stat(env.rulesPath()); errors.Is(err, os.ErrNotExist) {
		if err := cmdInit(env, nil); err != nil {
			return err
		}
		if err := setupTemplates(env, chooseTemplates(env, !*noPrompt), false, true); err != nil {
			return err
		}
	}
	if !*noDoctor {
		// like make install: a problem doctor reports does not fail setup
		_ = cmdDoctor(env, []string{"--wait", "--open"})
	}
	return nil
}

type setup struct {
	env  Env
	from string
	home string
}

func (s *setup) say(format string, a ...any) { fmt.Fprintf(s.env.Stdout, format+"\n", a...) }

func (s *setup) keychain() string {
	return filepath.Join(s.home, "Library", "Keychains", "login.keychain-db")
}

func (s *setup) certPath() string {
	return filepath.Join(s.env.DataDir, "codesign", signName+".pem")
}

// payload checks the files to install are there.
func (s *setup) payload(names ...string) error {
	for _, n := range names {
		if _, err := os.Stat(filepath.Join(s.from, n)); err != nil {
			return fmt.Errorf("setup: %s is missing from %s; reinstall tock (brew reinstall tock) or pass --from", n, s.from)
		}
	}
	return nil
}

func macPayload() []string {
	var out []string
	for _, a := range macApps {
		out = append(out, a.binary, a.plist)
		out = append(out, a.resources...)
	}
	return out
}

func (s *setup) hasCert() bool {
	_, err := setupRun("security", "find-certificate", "-c", signName, s.keychain())
	return err == nil
}

func (s *setup) trusted() bool {
	out, err := setupRun("security", "find-identity", "-v", "-p", "codesigning")
	return err == nil && strings.Contains(out, `"`+signName+`"`)
}

func guiTarget(label string) string { return "gui/" + strconv.Itoa(os.Getuid()) + "/" + label }

func loaded(label string) bool {
	_, err := setupRun("launchctl", "print", guiTarget(label))
	return err == nil
}

func (s *setup) mac() error {
	if err := s.payload(macPayload()...); err != nil {
		return err
	}
	if err := s.cert(); err != nil {
		return err
	}
	if err := s.trust(); err != nil {
		return err
	}
	for _, a := range macApps {
		if err := s.app(a); err != nil {
			return err
		}
	}
	for _, a := range macApps {
		bin := filepath.Join(s.env.DataDir, a.bundle, "Contents", "MacOS", a.binary)
		if out, err := setupRun(bin, "install"); err != nil {
			return fmt.Errorf("setup: %s install: %v: %s", a.binary, err, strings.TrimSpace(out))
		}
		s.say("started %s", a.label)
	}
	return nil
}

// cert makes the signing identity in the login keychain unless it is
// there, and keeps its public certificate next to the data for trust.
func (s *setup) cert() error {
	if !s.hasCert() {
		s.say("making the code-signing certificate %s (stays on this Mac)", signName)
		key, crt, err := newSigningCert(setupNow(), nil)
		if err != nil {
			return err
		}
		tmp, err := os.MkdirTemp("", "tock-setup-")
		if err != nil {
			return fmt.Errorf("setup: %w", err)
		}
		defer os.RemoveAll(tmp) // the private key lives only in the keychain
		kp, cp := filepath.Join(tmp, "k.pem"), filepath.Join(tmp, "c.pem")
		if err := os.WriteFile(kp, key, 0o600); err != nil {
			return fmt.Errorf("setup: %w", err)
		}
		if err := os.WriteFile(cp, crt, 0o600); err != nil {
			return fmt.Errorf("setup: %w", err)
		}
		if out, err := setupRun("security", "import", kp, "-k", s.keychain(), "-t", "priv", "-f", "openssl", "-T", "/usr/bin/codesign"); err != nil {
			return fmt.Errorf("setup: importing the key: %v: %s", err, strings.TrimSpace(out))
		}
		if out, err := setupRun("security", "import", cp, "-k", s.keychain(), "-t", "cert", "-f", "pemseq"); err != nil {
			return fmt.Errorf("setup: importing the certificate: %v: %s", err, strings.TrimSpace(out))
		}
	}
	// the public certificate, for add-trusted-cert: read back from the
	// keychain, so a certificate made by make cert works too
	out, err := setupRun("security", "find-certificate", "-c", signName, "-p", s.keychain())
	if err != nil || !strings.Contains(out, "BEGIN CERTIFICATE") {
		return fmt.Errorf("setup: reading %s from the keychain: %v", signName, err)
	}
	if err := os.MkdirAll(filepath.Dir(s.certPath()), 0o700); err != nil {
		return fmt.Errorf("setup: %w", err)
	}
	if err := os.WriteFile(s.certPath(), []byte(out), 0o644); err != nil {
		return fmt.Errorf("setup: %w", err)
	}
	return nil
}

// trust marks the certificate trusted for code signing: macOS asks for the
// user's password once.
func (s *setup) trust() error {
	if s.trusted() {
		return nil
	}
	s.say("macOS will ask for your password to trust %s for code signing.", signName)
	s.say("It signs tock's apps on this Mac, so Accessibility and Screen Recording stay granted across updates.")
	if out, err := setupRun("security", "add-trusted-cert", "-r", "trustRoot", "-p", "codeSign", "-k", s.keychain(), s.certPath()); err != nil {
		return fmt.Errorf("setup: trusting the certificate was cancelled or failed (%v: %s); run tock setup again", err, strings.TrimSpace(out))
	}
	if !s.trusted() {
		return errors.New("setup: the certificate is still not trusted for code signing; run tock setup again")
	}
	return nil
}

// app stops a's agent, puts the new app in place and signs it.
func (s *setup) app(a macApp) error {
	if _, err := setupRun("launchctl", "bootout", guiTarget(a.label)); err == nil {
		// bootout returns before the agent is gone: never replace a
		// binary under a running agent
		for i := 0; loaded(a.label); i++ {
			if i == 20 {
				return fmt.Errorf("setup: %s still running after 10 s", a.label)
			}
			s.env.Sleep(500 * time.Millisecond)
		}
	}
	final := filepath.Join(s.env.DataDir, a.bundle)
	tmp := final + ".new"
	if err := os.RemoveAll(tmp); err != nil {
		return fmt.Errorf("setup: %w", err)
	}
	c := filepath.Join(tmp, "Contents")
	files := []struct {
		from, to string
		mode     os.FileMode
	}{
		{a.binary, filepath.Join(c, "MacOS", a.binary), 0o755},
		{a.plist, filepath.Join(c, "Info.plist"), 0o644},
	}
	for _, r := range a.resources {
		files = append(files, struct {
			from, to string
			mode     os.FileMode
		}{r, filepath.Join(c, "Resources", r), 0o644})
	}
	if err := os.MkdirAll(s.env.DataDir, 0o700); err != nil {
		return fmt.Errorf("setup: %w", err)
	}
	for _, f := range files {
		if err := copyFile(filepath.Join(s.from, f.from), f.to, f.mode); err != nil {
			os.RemoveAll(tmp)
			return err
		}
	}
	if err := os.RemoveAll(final); err != nil {
		return fmt.Errorf("setup: %w", err)
	}
	if err := os.Rename(tmp, final); err != nil {
		return fmt.Errorf("setup: %w", err)
	}
	if out, err := setupRun("codesign", "--force", "--sign", signName, "--identifier", a.id, final); err != nil {
		return fmt.Errorf("setup: signing %s: %v: %s", a.bundle, err, strings.TrimSpace(out))
	}
	if out, err := setupRun("codesign", "--verify", "--strict", final); err != nil {
		return fmt.Errorf("setup: %s does not verify: %v: %s", a.bundle, err, strings.TrimSpace(out))
	}
	if a.resources != nil { // tock:// links reach the app once Launch Services knows it
		_, _ = setupRun("/System/Library/Frameworks/CoreServices.framework/Frameworks/LaunchServices.framework/Support/lsregister", "-f", final)
	}
	s.say("installed %s", final)
	return nil
}

// copyFile copies from to to with mode, making the folders (0755) it needs.
func copyFile(from, to string, mode os.FileMode) error {
	in, err := os.Open(from)
	if err != nil {
		return fmt.Errorf("setup: %w", err)
	}
	defer in.Close()
	if err := os.MkdirAll(filepath.Dir(to), 0o755); err != nil {
		return fmt.Errorf("setup: %w", err)
	}
	out, err := os.OpenFile(to, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, mode)
	if err != nil {
		return fmt.Errorf("setup: %w", err)
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return fmt.Errorf("setup: %w", err)
	}
	if err := out.Close(); err != nil {
		return fmt.Errorf("setup: %w", err)
	}
	return os.Chmod(to, mode) // umask may have taken bits off
}

// checkMac reports each part of the install; an error when any is missing.
func (s *setup) checkMac() error {
	missing := 0
	line := func(ok bool, what string) {
		mark := "ok     "
		if !ok {
			mark = "missing"
			missing++
		}
		s.say("%s  %s", mark, what)
	}
	line(s.payload(macPayload()...) == nil, "files to install in "+s.from)
	line(s.hasCert(), "certificate "+signName+" in the login keychain")
	line(s.trusted(), "certificate trusted for code signing")
	for _, a := range macApps {
		p := filepath.Join(s.env.DataDir, a.bundle)
		out, err := setupRun("codesign", "-dvv", p)
		line(err == nil && strings.Contains(out, "Authority="+signName), p+" signed with "+signName)
		line(loaded(a.label), a.label+" running")
	}
	if missing > 0 {
		return fmt.Errorf("setup: %d missing; run tock setup", missing)
	}
	s.say("tock is set up.")
	return nil
}

// cliPath is where --cli puts tock: where make install does, and where
// tockbar looks first.
func (s *setup) cliPath() string { return filepath.Join(s.home, ".local", "bin", "tock") }

// copyCLI copies the running tock to cliPath, for an install with no
// package manager to put it on PATH.
func (s *setup) copyCLI() error {
	self, err := selfPath()
	if err != nil {
		return fmt.Errorf("setup: %w", err)
	}
	if r, err := filepath.EvalSymlinks(self); err == nil {
		self = r
	}
	dst := s.cliPath()
	if self == dst {
		return nil
	}
	tmp := dst + ".new"
	if err := copyFile(self, tmp, 0o755); err != nil {
		return err
	}
	if err := os.Rename(tmp, dst); err != nil {
		os.Remove(tmp)
		return fmt.Errorf("setup: %w", err)
	}
	s.say("installed %s; add %s to PATH to run tock in a terminal", dst, filepath.Dir(dst))
	return nil
}

func (s *setup) linuxBin() string { return filepath.Join(s.home, ".local", "bin", "trackd") }

// linux puts trackd in ~/.local/bin (a path that stays put across package
// upgrades) and installs its systemd user unit.
func (s *setup) linux() error {
	if err := s.payload("trackd"); err != nil {
		return err
	}
	dst := s.linuxBin()
	tmp := dst + ".new"
	if err := copyFile(filepath.Join(s.from, "trackd"), tmp, 0o755); err != nil {
		return err
	}
	if err := os.Rename(tmp, dst); err != nil {
		return fmt.Errorf("setup: %w", err)
	}
	s.say("installed %s", dst)
	if out, err := setupRun(dst, "install"); err != nil {
		return fmt.Errorf("setup: trackd install: %v: %s", err, strings.TrimSpace(out))
	}
	return nil
}

func (s *setup) checkLinux() error {
	missing := 0
	for _, c := range []struct {
		ok   bool
		what string
	}{
		{s.payload("trackd") == nil, "files to install in " + s.from},
		{fileExists(s.linuxBin()), s.linuxBin()},
		{func() bool { _, err := setupRun("systemctl", "--user", "is-active", "trackd"); return err == nil }(), "trackd.service running"},
	} {
		mark := "ok     "
		if !c.ok {
			mark = "missing"
			missing++
		}
		s.say("%s  %s", mark, c.what)
	}
	if missing > 0 {
		return fmt.Errorf("setup: %d missing; run tock setup", missing)
	}
	s.say("tock is set up.")
	return nil
}

func fileExists(p string) bool {
	_, err := os.Stat(p)
	return err == nil
}
