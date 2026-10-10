package service

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// Unit is one systemd user service: its name (without .service), what it
// runs and where its output goes. Missing is the error text when Program
// is absent.
type Unit struct {
	Name, Description, Program, LogPath string
	Missing                             string
}

// RenderUnit is the unit file. It starts with the graphical session and is
// restarted after a failure, never after exit 0 (as KeepAlive
// {SuccessfulExit=false} on macOS).
func RenderUnit(u Unit) (string, error) {
	for _, v := range []string{u.Description, u.Program, u.LogPath} {
		if strings.ContainsAny(v, "\n\r\"") {
			return "", fmt.Errorf("unit %s: invalid value %q", u.Name, v)
		}
	}
	return fmt.Sprintf(`[Unit]
Description=%s
PartOf=graphical-session.target
After=graphical-session.target

[Service]
ExecStart="%s"
Restart=on-failure
RestartSec=10
StandardOutput=append:%s
StandardError=append:%s

[Install]
WantedBy=graphical-session.target
`, u.Description, u.Program, u.LogPath, u.LogPath), nil
}

// UnitPath and Systemctl are variables so tests never touch the real user
// unit dir or systemd.
var UnitPath = func(name string) string {
	dir := os.Getenv("XDG_CONFIG_HOME")
	if dir == "" {
		h, err := os.UserHomeDir()
		if err != nil {
			h = os.Getenv("HOME")
		}
		dir = filepath.Join(h, ".config")
	}
	return filepath.Join(dir, "systemd", "user", name+".service")
}

// Systemctl runs systemctl --user with args.
var Systemctl = func(args ...string) error {
	out, err := exec.Command("systemctl", append([]string{"--user"}, args...)...).CombinedOutput()
	if err != nil {
		return fmt.Errorf("systemctl --user %s: %w: %s", strings.Join(args, " "), err, strings.TrimSpace(string(out)))
	}
	return nil
}

// InstallUnit writes the unit, then enables and (re)starts it.
func InstallUnit(u Unit) error {
	if _, err := os.Stat(u.Program); err != nil {
		return errors.New(u.Missing)
	}
	text, err := RenderUnit(u)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(u.LogPath), 0o700); err != nil {
		return err
	}
	f, err := os.OpenFile(u.LogPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return err
	}
	f.Close()
	path := UnitPath(u.Name)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(path, []byte(text), 0o644); err != nil {
		return err
	}
	for _, args := range [][]string{{"daemon-reload"}, {"enable", u.Name + ".service"}, {"restart", u.Name + ".service"}} {
		if err := Systemctl(args...); err != nil {
			return err
		}
	}
	fmt.Printf("installed %s.service; log: %s\n", u.Name, u.LogPath)
	return nil
}

// UninstallUnit stops and disables the unit and removes its file. Data and
// the log are kept.
func UninstallUnit(name string) error {
	_ = Systemctl("disable", "--now", name+".service") // not installed: fine
	if err := os.Remove(UnitPath(name)); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	_ = Systemctl("daemon-reload")
	fmt.Printf("uninstalled %s.service (data kept)\n", name)
	return nil
}
