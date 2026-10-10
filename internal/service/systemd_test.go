package service

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRenderUnit(t *testing.T) {
	u := Unit{Name: "trackd", Description: "tock activity tracker", Program: "/home/x/.local/bin/trackd",
		LogPath: "/home/x/.local/state/tock/trackd.log"}
	got, err := RenderUnit(u)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"Description=tock activity tracker\n",
		`ExecStart="/home/x/.local/bin/trackd"` + "\n",
		"Restart=on-failure\n", "RestartSec=10\n",
		"StandardOutput=append:/home/x/.local/state/tock/trackd.log\n",
		"WantedBy=graphical-session.target\n",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in:\n%s", want, got)
		}
	}
	u.Program = "/x\nExecStartPre=/bin/evil"
	if _, err := RenderUnit(u); err == nil {
		t.Fatal("a newline in a value must be refused")
	}
}

func TestInstallUnit(t *testing.T) {
	dir := t.TempDir()
	oldPath, oldCtl := UnitPath, Systemctl
	t.Cleanup(func() { UnitPath, Systemctl = oldPath, oldCtl })
	unit := filepath.Join(dir, "systemd", "user", "trackd.service")
	UnitPath = func(string) string { return unit }
	var calls []string
	Systemctl = func(args ...string) error { calls = append(calls, strings.Join(args, " ")); return nil }

	u := Unit{Name: "trackd", Description: "d", Program: filepath.Join(dir, "nope"), LogPath: filepath.Join(dir, "s", "t.log"),
		Missing: "trackd not found; run make install-linux"}
	if err := InstallUnit(u); err == nil || !strings.Contains(err.Error(), "install-linux") || len(calls) != 0 {
		t.Fatalf("missing program: %v, calls %v", err, calls)
	}
	u.Program = filepath.Join(dir, "trackd")
	os.WriteFile(u.Program, nil, 0o755)
	if err := InstallUnit(u); err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(calls, ","); got != "daemon-reload,enable trackd.service,restart trackd.service" {
		t.Fatalf("systemctl calls = %s", got)
	}
	if _, err := os.Stat(unit); err != nil {
		t.Fatal(err)
	}
	calls = nil
	if err := UninstallUnit("trackd"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(unit); !os.IsNotExist(err) || calls[0] != "disable --now trackd.service" {
		t.Fatalf("uninstall: %v %v", err, calls)
	}
}
