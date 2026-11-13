package main

import (
	"reflect"
	"testing"

	"tock/internal/testfs"
)

func TestDecide(t *testing.T) {
	testfs.UnixOnly(t, "paths inside the macOS app bundle")
	self := "/Applications/tock.app/Contents/MacOS/tockapp"
	setup := []string{"/Applications/tock.app/Contents/Resources/libexec/tock", "setup",
		"--from", "/Applications/tock.app/Contents/Resources/libexec", "--cli"}
	for _, c := range []struct {
		name, self, installed string
		payload               bool
		kind, ok              string
		cmd                   []string
	}{
		{"first open", self, "", true, "setup", "Set Up", setup},
		{"newer than installed", self, "v6.0.0", true, "setup", "Update", setup},
		{"older than installed counts too", self, "v6.2.0", true, "setup", "Update", setup},
		{"same version", self, "v6.1.0", true, "open", "", []string{"/usr/bin/open", "tock://dashboard"}},
		{"from the disk image", "/Volumes/tock/tock.app/Contents/MacOS/tockapp", "", true, "stop", "", nil},
		{"translocated", "/private/var/folders/x/AppTranslocation/ABC/d/tock.app/Contents/MacOS/tockapp", "", true, "stop", "", nil},
		{"no payload", self, "", false, "stop", "", nil},
	} {
		p := decide(c.self, "v6.1.0", c.installed, c.payload)
		if p.kind != c.kind || p.ok != c.ok || !reflect.DeepEqual(p.cmd, c.cmd) {
			t.Errorf("%s: got %+v", c.name, p)
		}
		if p.kind != "open" && p.title == "" {
			t.Errorf("%s: no title", c.name)
		}
	}
	if p := decide(self, "v6.1.0", "v6.0.0", true); !contains(p.text, "v6.0.0") || !contains(p.text, "v6.1.0") {
		t.Errorf("update text names neither version: %q", p.text)
	}
}

func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
