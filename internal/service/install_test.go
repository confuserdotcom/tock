package service

import "testing"

func TestInstallMethod(t *testing.T) {
	app := "/Applications/tock.app/Contents/MacOS/tockapp"
	cases := []struct {
		name, self string
		apps       bool
		want       Method
	}{
		{"brew arm", "/opt/homebrew/Cellar/tock/6.0.0/bin/tock", false, Brew},
		{"brew intel", "/usr/local/Cellar/tock/6.0.0/bin/tock", true, Brew},
		{"linuxbrew", "/home/linuxbrew/.linuxbrew/Cellar/tock/6.0.0/bin/tock", false, Brew},
		{"dmg", "/Users/me/.local/bin/tock", true, DMG},
		{"dmg libexec", "/Applications/tock.app/Contents/Resources/libexec/tock", true, DMG},
		{"source", "/Users/me/.local/bin/tock", false, Source},
		{"go run", "/tmp/go-build123/exe/tock", false, Source},
	}
	for _, c := range cases {
		exists := func(p string) bool { return c.apps && p == app }
		if got := InstallMethod(c.self, exists); got != c.want {
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
