package main

import (
	"crypto/sha256"
	"embed"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"tock/internal/fronttab"
)

// extFiles is the tock browser extension, which `trackd browser install`
// writes out for the browser's "Load unpacked".
//
//go:embed browserext
var extFiles embed.FS

// browserHostCaller reports whether args are a browser starting trackd as
// its native-messaging host, and whether the caller is the tock extension.
// Chromium passes the extension's origin; Firefox passes the host
// manifest's path and the extension's id.
func browserHostCaller(args []string) (host, ok bool) {
	switch {
	case len(args) > 1 && strings.HasPrefix(args[1], "chrome-extension://"):
		return true, args[1] == "chrome-extension://"+fronttab.ChromeExtensionID+"/"
	case len(args) > 2 && strings.HasSuffix(args[1], ".json"):
		return true, args[2] == fronttab.FirefoxExtensionID
	}
	return false, false
}

// runBrowserHost is trackd as the browser's native-messaging host. stdout
// belongs to the browser: nothing is ever written to it.
func runBrowserHost(args []string, stateDir string) int {
	if _, ok := browserHostCaller(args); !ok {
		fmt.Fprintln(os.Stderr, "trackd: browser host: not the tock extension")
		return 1
	}
	path := filepath.Join(stateDir, fronttab.File)
	if err := fronttab.RunHost(os.Stdin, path, func() int64 { return time.Now().UnixMilli() }); err != nil {
		fmt.Fprintf(os.Stderr, "trackd: %v\n", err)
		return 1
	}
	return 0
}

// hostDirs are where each browser looks for native-messaging host
// manifests, relative to home, under the browser's own dir.
var hostDirs = []struct {
	browser, dir, hosts string
	firefox             bool
}{
	{"Chromium", ".config/chromium", "NativeMessagingHosts", false},
	{"Chrome", ".config/google-chrome", "NativeMessagingHosts", false},
	{"Brave", ".config/BraveSoftware/Brave-Browser", "NativeMessagingHosts", false},
	{"Firefox", ".mozilla", "native-messaging-hosts", true},
}

// installBrowser is trackd browser install: it writes the extension under
// dataDir and a host manifest pointing at exe for every installed browser.
func installBrowser(home, dataDir, exe string, out io.Writer) error {
	extDir := filepath.Join(dataDir, "browser-extension")
	var found []string
	for _, h := range hostDirs {
		base := filepath.Join(home, h.dir)
		if fi, err := os.Stat(base); err != nil || !fi.IsDir() {
			continue
		}
		m := map[string]any{"name": fronttab.HostName, "description": "tock: the tab in front",
			"path": exe, "type": "stdio"}
		if h.firefox {
			m["allowed_extensions"] = []string{fronttab.FirefoxExtensionID}
		} else {
			m["allowed_origins"] = []string{"chrome-extension://" + fronttab.ChromeExtensionID + "/"}
		}
		b, _ := json.MarshalIndent(m, "", "  ")
		dir := filepath.Join(base, h.hosts)
		if err := os.MkdirAll(dir, 0o700); err != nil {
			return fmt.Errorf("browser install: %w", err)
		}
		if err := os.WriteFile(filepath.Join(dir, fronttab.HostName+".json"), append(b, '\n'), 0o600); err != nil {
			return fmt.Errorf("browser install: %w", err)
		}
		found = append(found, h.browser)
	}
	if len(found) == 0 {
		return errors.New("browser install: no Chromium, Chrome, Brave or Firefox profile found")
	}
	if err := writeExtension(extDir); err != nil {
		return err
	}
	fmt.Fprintf(out, "host installed for %s\n\n", strings.Join(found, ", "))
	fmt.Fprintf(out, "Chromium, Chrome, Brave: open chrome://extensions, turn on Developer mode,\n"+
		"click Load unpacked and pick\n  %s\nthen in the extension's Details turn on \"Allow in Incognito\" so private\n"+
		"windows are recognised (their titles are then never recorded).\n", extDir)
	fmt.Fprintf(out, "Firefox: about:debugging → This Firefox → Load Temporary Add-on → %s\n"+
		"(Firefox drops unsigned add-ons at restart, so this lasts one session.)\n",
		filepath.Join(extDir, "manifest.json"))
	return nil
}

// writeExtension writes the embedded extension files to dir.
func writeExtension(dir string) error {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("browser install: %w", err)
	}
	return fs.WalkDir(extFiles, "browserext", func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		b, err := extFiles.ReadFile(p)
		if err != nil {
			return err
		}
		if err := os.WriteFile(filepath.Join(dir, filepath.Base(p)), b, 0o600); err != nil {
			return fmt.Errorf("browser install: %w", err)
		}
		return nil
	})
}

// extensionID is the Chromium extension id a manifest key gives: the first
// 32 hex digits of the key's SHA-256, written with a–p for 0–f.
func extensionID(key string) (string, error) {
	der, err := base64.StdEncoding.DecodeString(key)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(der)
	h := hex.EncodeToString(sum[:16])
	b := make([]byte, len(h))
	for i := range h {
		v := h[i] - '0'
		if h[i] >= 'a' {
			v = h[i] - 'a' + 10
		}
		b[i] = 'a' + v
	}
	return string(b), nil
}

// runBrowserCmd is trackd browser install. macOS needs no extension: tock
// asks the browser itself.
func runBrowserCmd(args []string, out io.Writer) error {
	if len(args) != 1 || args[0] != "install" {
		return errors.New("usage: trackd browser install")
	}
	if runtime.GOOS == "darwin" {
		fmt.Fprintln(out, "not needed on macOS: tock asks the browser for the tab in front")
		return nil
	}
	exe, err := os.Executable()
	if err != nil {
		return fmt.Errorf("browser install: %w", err)
	}
	if p, err := filepath.EvalSymlinks(exe); err == nil {
		exe = p
	}
	return installBrowser(home(), dataDir(), exe, out)
}
