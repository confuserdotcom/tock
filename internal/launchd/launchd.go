// Package launchd installs and removes the LaunchAgents of trackd and the
// tock menu bar app.
package launchd

import (
	"bytes"
	"encoding/xml"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"text/template"
)

// launchctlAlreadyLoaded is bootstrap's exit status when the label is still
// loaded (RESEARCH Q6: rc 5 after a racing bootout).
const launchctlAlreadyLoaded = 5

// Agent is one LaunchAgent: the label, the program it runs and the file
// its output goes to. Missing is the error text when Program is absent.
type Agent struct {
	Label, Program, LogPath string
	Missing                 string
}

// KeepAlive {SuccessfulExit=false}: launchd restarts the program after exit
// 1 or a crash, never after exit 0 (D-17). ThrottleInterval bounds a restart storm.
var plistTmpl = template.Must(template.New("plist").Funcs(template.FuncMap{
	"xml": func(s string) (string, error) {
		var b bytes.Buffer
		err := xml.EscapeText(&b, []byte(s))
		return b.String(), err
	},
}).Parse(`<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
	<key>Label</key>
	<string>{{xml .Label}}</string>
	<key>ProgramArguments</key>
	<array>
		<string>{{xml .Program}}</string>
	</array>
	<key>RunAtLoad</key>
	<true/>
	<key>KeepAlive</key>
	<dict>
		<key>SuccessfulExit</key>
		<false/>
	</dict>
	<key>ProcessType</key>
	<string>Interactive</string>
	<key>LimitLoadToSessionType</key>
	<string>Aqua</string>
	<key>ThrottleInterval</key>
	<integer>10</integer>
	<key>StandardOutPath</key>
	<string>{{xml .LogPath}}</string>
	<key>StandardErrorPath</key>
	<string>{{xml .LogPath}}</string>
</dict>
</plist>
`))

// Render is the agent's plist.
func Render(c Agent) ([]byte, error) {
	var b bytes.Buffer
	if err := plistTmpl.Execute(&b, c); err != nil {
		return nil, err
	}
	return b.Bytes(), nil
}

// PlistPath and Run are variables so tests never touch the real
// LaunchAgents dir or launchd.
var PlistPath = func(label string) string {
	h, err := os.UserHomeDir()
	if err != nil {
		h = os.Getenv("HOME")
	}
	return filepath.Join(h, "Library", "LaunchAgents", label+".plist")
}

// Run runs launchctl with args.
var Run = func(args ...string) error {
	out, err := exec.Command("launchctl", args...).CombinedOutput()
	if err != nil {
		return &launchctlError{args: args, out: strings.TrimSpace(string(out)), err: err}
	}
	return nil
}

type launchctlError struct {
	args []string
	out  string
	err  error
}

func (e *launchctlError) Error() string {
	return fmt.Sprintf("launchctl %s: %v: %s", strings.Join(e.args, " "), e.err, e.out)
}

func (e *launchctlError) Unwrap() error { return e.err }

func launchctlExitCode(err error) int {
	var ee *exec.ExitError
	if errors.As(err, &ee) {
		return ee.ExitCode()
	}
	return -1
}

func guiDomain() string { return fmt.Sprintf("gui/%d", os.Getuid()) }

// Install writes the agent's plist and (re)loads it.
func Install(c Agent) error {
	if _, err := os.Stat(c.Program); err != nil {
		return errors.New(c.Missing)
	}
	if err := os.MkdirAll(filepath.Dir(c.LogPath), 0o700); err != nil {
		return err
	}
	f, err := os.OpenFile(c.LogPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return err
	}
	f.Close()
	if err := os.Chmod(c.LogPath, 0o600); err != nil {
		return err
	}

	b, err := Render(c)
	if err != nil {
		return err
	}
	path := PlistPath(c.Label)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(path, b, 0o644); err != nil {
		return err
	}
	if err := os.Chmod(path, 0o644); err != nil {
		return err
	}

	target := guiDomain() + "/" + c.Label
	_ = Run("bootout", target) // rc 3: not loaded
	err = Run("bootstrap", guiDomain(), path)
	if launchctlExitCode(err) == launchctlAlreadyLoaded {
		_ = Run("bootout", target)
		err = Run("bootstrap", guiDomain(), path)
	}
	if err != nil {
		return err
	}
	fmt.Printf("installed %s; log: %s\n", c.Label, c.LogPath)
	return nil
}

// Uninstall unloads the agent and removes its plist. Data and the log are
// kept.
func Uninstall(label string) error {
	_ = Run("bootout", guiDomain()+"/"+label)
	if err := os.Remove(PlistPath(label)); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	fmt.Printf("uninstalled %s (data kept)\n", label)
	return nil
}
