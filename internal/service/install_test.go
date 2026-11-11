package service

import (
	"os"
	"path/filepath"
	"testing"
)

func TestInstallMethod(t *testing.T) {
	cases := []struct {
		name, self string
		copyOfApp  bool // self holds the .dmg app's tock
		want       Method
	}{
		{"brew arm", "/opt/homebrew/Cellar/tock/6.0.0/bin/tock", false, Brew},
		{"brew intel", "/usr/local/Cellar/tock/6.0.0/bin/tock", true, Brew},
		{"linuxbrew", "/home/linuxbrew/.linuxbrew/Cellar/tock/6.0.0/bin/tock", false, Brew},
		{"dmg", "/Users/me/.local/bin/tock", true, DMG},
		{"dmg libexec", "/Applications/tock.app/Contents/Resources/libexec/tock", true, DMG},
		{"source, .dmg app also there", "/Users/me/.local/bin/tock", false, Source},
		{"go run", "/tmp/go-build123/exe/tock", false, Source},
	}
	for _, c := range cases {
		fromDMG := func(string) bool { return c.copyOfApp }
		if got := InstallMethod(c.self, fromDMG); got != c.want {
			t.Errorf("%s: InstallMethod(%q) = %v, want %v", c.name, c.self, got, c.want)
		}
	}
}

func TestReinstallHint(t *testing.T) {
	cases := []struct {
		m     Method
		linux bool
		want  string
	}{
		{Brew, false, "tock setup"},
		{Brew, true, "tock setup"},
		{DMG, false, "open tock from Applications"},
		{Source, false, "make install"},
		{Source, true, "trackd install"},
	}
	for _, c := range cases {
		if got := c.m.Reinstall(c.linux); got != c.want {
			t.Errorf("%v linux=%v: %q, want %q", c.m, c.linux, got, c.want)
		}
	}
}

func TestSameBytes(t *testing.T) {
	d := t.TempDir()
	w := func(n, s string) string {
		p := filepath.Join(d, n)
		if err := os.WriteFile(p, []byte(s), 0o600); err != nil {
			t.Fatal(err)
		}
		return p
	}
	a, b, c, e := w("a", "same"), w("b", "same"), w("c", "diff"), w("e", "longer")
	if !sameBytes(a, b) || sameBytes(a, c) || sameBytes(a, e) || sameBytes(a, filepath.Join(d, "missing")) {
		t.Fatal("sameBytes wrong")
	}
}
