package main

import (
	"strings"
	"testing"
)

func TestVersion(t *testing.T) {
	old := version
	version = "v5.1.0"
	defer func() { version = old }()
	for _, arg := range []string{"version", "--version"} {
		te := newTestEnv(t, testRules)
		code, out := te.run(t, "", arg)
		if code != 0 || out != "tock v5.1.0\n" {
			t.Errorf("%s: code %d, out %q", arg, code, out)
		}
	}
	te := newTestEnv(t, testRules)
	if _, out := te.run(t, "", "help"); !strings.Contains(out, "tock version") {
		t.Errorf("help lacks tock version:\n%s", out)
	}
}
