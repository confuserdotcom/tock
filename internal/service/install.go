package service

import (
	"os"
	"path/filepath"
	"strings"
)

// Method is how tock was installed. Fix hints depend on it: a Homebrew or
// .dmg user has no checkout to run make in.
type Method int

const (
	Source Method = iota // make install, make install-linux, or the Linux tarball
	Brew
	DMG
)

func (m Method) String() string {
	switch m {
	case Brew:
		return "homebrew"
	case DMG:
		return "dmg"
	}
	return "source"
}

// dmgApp is the app the .dmg puts in Applications.
const dmgApp = "/Applications/tock.app/Contents/MacOS/tockapp"

// InstallMethod tells how the tock or trackd at self was installed. exists
// reports whether a file exists (os.Stat in production).
func InstallMethod(self string, exists func(string) bool) Method {
	if r, err := filepath.EvalSymlinks(self); err == nil {
		self = r
	}
	s := filepath.ToSlash(self)
	if strings.Contains(s, "/Cellar/") || strings.Contains(s, "/linuxbrew/") {
		return Brew
	}
	// The .dmg's tock copies itself to ~/.local/bin, like make install, so
	// the app in Applications is what tells them apart.
	if strings.HasPrefix(s, "/Applications/tock.app/") || exists(dmgApp) {
		return DMG
	}
	return Source
}

// Installed is InstallMethod for the running program.
func Installed() Method {
	self, err := os.Executable()
	if err != nil {
		return Source
	}
	return InstallMethod(self, func(p string) bool { _, err := os.Stat(p); return err == nil })
}

// Reinstall is what to run to (re)install and start trackd.
func (m Method) Reinstall(linux bool) string {
	switch m {
	case Brew:
		return "tock setup"
	case DMG:
		return "open tock from Applications"
	}
	if linux {
		return "trackd install"
	}
	return "make install"
}

// Update is how to get a newer tock.
func (m Method) Update(linux bool) string {
	switch m {
	case Brew:
		return "brew upgrade tock && tock setup"
	case DMG:
		return "download the new .dmg and drag tock over the old one"
	}
	if linux {
		return "download the new release tarball, or git pull && make install-linux"
	}
	return "git pull && make install"
}
