package main

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// plistJSON renders c and converts it to JSON with plutil, failing on any
// plist that plutil rejects.
func plistJSON(t *testing.T, c agentConfig) map[string]any {
	t.Helper()
	b, err := renderPlist(c)
	if err != nil {
		t.Fatal(err)
	}
	p := filepath.Join(t.TempDir(), "agent.plist")
	if err := os.WriteFile(p, b, 0o600); err != nil {
		t.Fatal(err)
	}
	if out, err := exec.Command("plutil", "-lint", p).CombinedOutput(); err != nil {
		t.Fatalf("plutil -lint: %v\n%s\n%s", err, out, b)
	}
	out, err := exec.Command("plutil", "-convert", "json", "-o", "-", p).Output()
	if err != nil {
		t.Fatalf("plutil -convert json: %v", err)
	}
	var m map[string]any
	if err := json.Unmarshal(out, &m); err != nil {
		t.Fatal(err)
	}
	return m
}

var testAgent = agentConfig{
	Label:   "com.tock.trackd",
	Program: "/Users/x/.local/share/tock/trackd.app/Contents/MacOS/trackd",
	LogPath: "/Users/x/.local/state/tock/trackd.log",
}

func TestRenderPlistKeys(t *testing.T) {
	m := plistJSON(t, testAgent)
	want := map[string]any{
		"Label":                  "com.tock.trackd",
		"RunAtLoad":              true,
		"ProcessType":            "Interactive",
		"LimitLoadToSessionType": "Aqua",
		"ThrottleInterval":       float64(10),
		"StandardOutPath":        testAgent.LogPath,
		"StandardErrorPath":      testAgent.LogPath,
	}
	for k, v := range want {
		if m[k] != v {
			t.Errorf("%s = %#v; want %#v", k, m[k], v)
		}
	}
	args, ok := m["ProgramArguments"].([]any)
	if !ok || len(args) != 1 || args[0] != testAgent.Program {
		t.Errorf("ProgramArguments = %#v", m["ProgramArguments"])
	}
	if len(m) != len(want)+2 {
		t.Errorf("plist has %d keys; want %d: %v", len(m), len(want)+2, m)
	}
}

func TestRenderPlistEscapesXML(t *testing.T) {
	c := testAgent
	c.Program = "/Users/a&b/<x>/trackd"
	b, err := renderPlist(c)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(b), "/Users/a&amp;b/&lt;x&gt;/trackd") {
		t.Fatalf("program not escaped:\n%s", b)
	}
	args := plistJSON(t, c)["ProgramArguments"].([]any)
	if args[0] != c.Program {
		t.Fatalf("round trip = %q; want %q", args[0], c.Program)
	}
}

func TestRenderPlistKeepAliveIsDictNotBool(t *testing.T) {
	ka, ok := plistJSON(t, testAgent)["KeepAlive"].(map[string]any)
	if !ok {
		t.Fatalf("KeepAlive is not a dict")
	}
	if v, ok := ka["SuccessfulExit"].(bool); !ok || v {
		t.Fatalf("KeepAlive.SuccessfulExit = %#v; want false", ka["SuccessfulExit"])
	}
	if len(ka) != 1 {
		t.Fatalf("KeepAlive = %v; want only SuccessfulExit", ka)
	}
}

func TestInstallRequiresBundle(t *testing.T) {
	dir := t.TempDir()
	plist := filepath.Join(dir, "LaunchAgents", "com.tock.trackd.plist")
	oldPath, oldRun := plistPathFn, runLaunchctl
	t.Cleanup(func() { plistPathFn, runLaunchctl = oldPath, oldRun })
	plistPathFn = func() string { return plist }
	ran := false
	runLaunchctl = func(args ...string) error { ran = true; return nil }

	err := installAgent(filepath.Join(dir, "data"), filepath.Join(dir, "state"))
	if err == nil || !strings.Contains(err.Error(), "run make bundle") {
		t.Fatalf("err = %v; want bundle-not-found", err)
	}
	if _, err := os.Stat(plist); !os.IsNotExist(err) {
		t.Fatalf("plist written without a bundle (stat err %v)", err)
	}
	if ran {
		t.Fatal("launchctl called without a bundle")
	}
}
