package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"tock/internal/fronttab"
)

func TestBrowserHostCaller(t *testing.T) {
	cases := []struct {
		args []string
		host bool
		ok   bool
	}{
		{[]string{"trackd", "chrome-extension://" + fronttab.ChromeExtensionID + "/"}, true, true},
		{[]string{"trackd", "chrome-extension://aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa/"}, true, false},
		{[]string{"trackd", "/home/u/.mozilla/native-messaging-hosts/dev.tock.browser.json", fronttab.FirefoxExtensionID}, true, true},
		{[]string{"trackd", "/x.json", "evil@x"}, true, false},
		{[]string{"trackd", "install"}, false, false},
		{[]string{"trackd"}, false, false},
	}
	for _, c := range cases {
		host, ok := browserHostCaller(c.args)
		if host != c.host || ok != c.ok {
			t.Errorf("%q: host %v ok %v", c.args, host, ok)
		}
	}
}

func TestInstallBrowser(t *testing.T) {
	home := t.TempDir()
	data := filepath.Join(home, "data")
	var out bytes.Buffer
	if err := installBrowser(home, data, "/opt/trackd", &out); err == nil {
		t.Fatal("no browser profile must be an error")
	}
	os.MkdirAll(filepath.Join(home, ".config", "chromium"), 0o700)
	os.MkdirAll(filepath.Join(home, ".mozilla"), 0o700)
	out.Reset()
	if err := installBrowser(home, data, "/opt/trackd", &out); err != nil {
		t.Fatal(err)
	}
	var cm map[string]any
	b, err := os.ReadFile(filepath.Join(home, ".config", "chromium", "NativeMessagingHosts", fronttab.HostName+".json"))
	if err != nil || json.Unmarshal(b, &cm) != nil {
		t.Fatalf("chromium manifest: %s %v", b, err)
	}
	if cm["path"] != "/opt/trackd" || cm["type"] != "stdio" ||
		cm["allowed_origins"].([]any)[0] != "chrome-extension://"+fronttab.ChromeExtensionID+"/" {
		t.Fatalf("chromium manifest = %v", cm)
	}
	var fm map[string]any
	b, _ = os.ReadFile(filepath.Join(home, ".mozilla", "native-messaging-hosts", fronttab.HostName+".json"))
	if json.Unmarshal(b, &fm) != nil || fm["allowed_extensions"].([]any)[0] != fronttab.FirefoxExtensionID {
		t.Fatalf("firefox manifest = %s", b)
	}
	if _, err := os.Stat(filepath.Join(home, ".config", "google-chrome", "NativeMessagingHosts")); err == nil {
		t.Fatal("wrote for a browser that is not installed")
	}
	for _, f := range []string{"manifest.json", "background.js"} {
		if _, err := os.Stat(filepath.Join(data, "browser-extension", f)); err != nil {
			t.Fatalf("extension file %s: %v", f, err)
		}
	}
	if s := out.String(); !strings.Contains(s, "Load unpacked") || !strings.Contains(s, filepath.Join(data, "browser-extension")) {
		t.Fatalf("instructions:\n%s", s)
	}
}

// The embedded manifest's key must give the id the host allows.
func TestExtensionKeyMatchesID(t *testing.T) {
	b, err := extFiles.ReadFile("browserext/manifest.json")
	if err != nil {
		t.Fatal(err)
	}
	var m struct{ Key string }
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatal(err)
	}
	if id, err := extensionID(m.Key); err != nil || id != fronttab.ChromeExtensionID {
		t.Fatalf("id %q err %v, want %s", id, err, fronttab.ChromeExtensionID)
	}
}
