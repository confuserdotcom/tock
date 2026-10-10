package main

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

const agentLabel = "com.tock.trackd"

// launchctlAlreadyLoaded is bootstrap's exit status when the label is still
// loaded (RESEARCH Q6: rc 5 after a racing bootout).
const launchctlAlreadyLoaded = 5

type agentConfig struct {
	Label, Program, LogPath string
}

// KeepAlive {SuccessfulExit=false}: launchd restarts trackd after exit 1 or a
// crash, never after exit 0 (D-17). ThrottleInterval bounds a restart storm.
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

func renderPlist(c agentConfig) ([]byte, error) {
	var b bytes.Buffer
	if err := plistTmpl.Execute(&b, c); err != nil {
		return nil, err
	}
	return b.Bytes(), nil
}

func plistPath() string {
	return filepath.Join(home(), "Library", "LaunchAgents", agentLabel+".plist")
}

// plistPathFn and runLaunchctl are variables so tests never touch the real
// LaunchAgents dir or launchd.
var plistPathFn = plistPath

var runLaunchctl = func(args ...string) error {
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

// installAgent writes the plist for the bundle under dataDir and (re)loads it.
func installAgent(dataDir, stateDir string) error {
	c := agentConfig{
		Label:   agentLabel,
		Program: filepath.Join(dataDir, "trackd.app", "Contents", "MacOS", "trackd"),
		LogPath: filepath.Join(stateDir, "trackd.log"),
	}
	if _, err := os.Stat(c.Program); err != nil {
		return fmt.Errorf("bundle not found at %s; run make bundle", c.Program)
	}
	if err := os.MkdirAll(stateDir, 0o700); err != nil {
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

	b, err := renderPlist(c)
	if err != nil {
		return err
	}
	path := plistPathFn()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(path, b, 0o644); err != nil {
		return err
	}
	if err := os.Chmod(path, 0o644); err != nil {
		return err
	}

	target := guiDomain() + "/" + agentLabel
	_ = runLaunchctl("bootout", target) // rc 3: not loaded
	err = runLaunchctl("bootstrap", guiDomain(), path)
	if launchctlExitCode(err) == launchctlAlreadyLoaded {
		_ = runLaunchctl("bootout", target)
		err = runLaunchctl("bootstrap", guiDomain(), path)
	}
	if err != nil {
		return err
	}
	fmt.Printf("installed %s; log: %s\n", agentLabel, c.LogPath)
	return nil
}

// uninstallAgent unloads the agent and removes its plist. tock.db and the log
// are kept.
func uninstallAgent() error {
	_ = runLaunchctl("bootout", guiDomain()+"/"+agentLabel)
	if err := os.Remove(plistPathFn()); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	fmt.Printf("uninstalled %s (data kept)\n", agentLabel)
	return nil
}
