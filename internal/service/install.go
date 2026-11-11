package service

import (
	"crypto/sha256"
	"io"
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

// dmgTock is the tock inside the app the .dmg puts in Applications; its
// setup copies it to ~/.local/bin.
const dmgTock = "/Applications/tock.app/Contents/Resources/libexec/tock"

// InstallMethod tells how the tock at self was installed. fromDMG reports
// whether self is a copy of the .dmg app's tock (sameBytes in production).
func InstallMethod(self string, fromDMG func(self string) bool) Method {
	if r, err := filepath.EvalSymlinks(self); err == nil {
		self = r
	}
	s := filepath.ToSlash(self)
	if strings.Contains(s, "/Cellar/") || strings.Contains(s, "/linuxbrew/") {
		return Brew
	}
	// The .dmg's tock copies itself to ~/.local/bin, like make install.
	// The app being in Applications is not enough (a source install may
	// have tried the .dmg once): the copy must be the app's own tock.
	if strings.HasPrefix(s, "/Applications/tock.app/") || fromDMG(self) {
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
	return InstallMethod(self, func(s string) bool { return sameBytes(s, dmgTock) })
}

// sameBytes reports whether files a and b hold the same bytes. Different
// sizes answer without reading either.
func sameBytes(a, b string) bool {
	fa, err := os.Stat(a)
	if err != nil {
		return false
	}
	fb, err := os.Stat(b)
	if err != nil || fa.Size() != fb.Size() {
		return false
	}
	ha, errA := fileSum(a)
	hb, errB := fileSum(b)
	return errA == nil && errB == nil && ha == hb
}

func fileSum(p string) ([32]byte, error) {
	f, err := os.Open(p)
	if err != nil {
		return [32]byte{}, err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return [32]byte{}, err
	}
	var s [32]byte
	copy(s[:], h.Sum(nil))
	return s, nil
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
