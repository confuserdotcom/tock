// Package fronttab carries the front browser tab from the tock extension
// to trackd on Linux: the native-messaging host and the file it writes.
package fronttab

import (
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"tock/internal/span"
)

// On Linux no browser exposes its front tab, so a small tock extension
// sends it over native messaging to trackd, which Chromium or Firefox start
// as the host (stdin, never the network). The host writes the front tab to
// File in trackd's state dir; the Hyprland collector reads it.

// File is the front-tab file, in trackd's state dir.
const File = "browser.json"

// The extension's ids: Chromium derives ChromeExtensionID from the key in
// the extension's manifest; Firefox uses FirefoxExtensionID as written.
// The host answers only these.
const (
	ChromeExtensionID  = "pgofmjadkeojgckojgonelbfoiihonlo"
	FirefoxExtensionID = "browser@tock.local"
	HostName           = "dev.tock.browser"
)

const (
	// MaxMsg bounds one message; a front tab is far smaller.
	MaxMsg = 64 << 10
	// SkewMS is how much earlier than the window's activation a
	// message may be and still describe that window: the extension and
	// Hyprland see the same focus change at about the same time.
	SkewMS = 2000
)

// State is the front tab as the extension last saw it. URL is
// span.URLKey's form (no scheme, query or fragment); an incognito tab has
// Private set and neither URL nor Title. Focused is false when no window
// the extension may see has focus: an incognito window it is not allowed
// in, devtools, or another app.
type State struct {
	AtMS    int64  `json:"at_ms"`
	Focused bool   `json:"focused"`
	Private bool   `json:"private,omitempty"`
	URL     string `json:"url,omitempty"`
	Title   string `json:"title,omitempty"`
}

// message is one extension message.
type message struct {
	Focused   bool   `json:"focused"`
	URL       string `json:"url"`
	Title     string `json:"title"`
	Incognito bool   `json:"incognito"`
}

// RunHost reads native-messaging frames (a little-endian uint32
// length, then JSON) from r until EOF and writes each as the state at path.
// A malformed frame ends the host with an error; the extension reconnects.
func RunHost(r io.Reader, path string, now func() int64) error {
	var n uint32
	for {
		if err := binary.Read(r, binary.LittleEndian, &n); err != nil {
			if errors.Is(err, io.EOF) {
				return nil
			}
			return fmt.Errorf("browser host: read length: %w", err)
		}
		if n == 0 || n > MaxMsg {
			return fmt.Errorf("browser host: message of %d bytes", n)
		}
		b := make([]byte, n)
		if _, err := io.ReadFull(r, b); err != nil {
			return fmt.Errorf("browser host: read message: %w", err)
		}
		var m message
		if err := json.Unmarshal(b, &m); err != nil {
			return fmt.Errorf("browser host: message: %w", err)
		}
		st := State{AtMS: now(), Focused: m.Focused}
		switch {
		case m.Incognito:
			st.Private = true
		case m.Focused:
			st.Title = strings.ToValidUTF8(m.Title, "�")
			if k, ok := span.URLKey(m.URL); ok {
				st.URL = k
			}
		}
		if err := writeState(path, st); err != nil {
			return err
		}
	}
}

// writeState replaces path atomically, readable by the user only.
func writeState(path string, st State) error {
	b, err := json.Marshal(st)
	if err != nil {
		return fmt.Errorf("browser host: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("browser host: %w", err)
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".browser-*")
	if err != nil {
		return fmt.Errorf("browser host: %w", err)
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.Write(b); err != nil {
		tmp.Close()
		return fmt.Errorf("browser host: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("browser host: %w", err)
	}
	if err := os.Rename(tmp.Name(), path); err != nil {
		return fmt.Errorf("browser host: %w", err)
	}
	return nil
}

// Read reads the front-tab file; ok is false when there is
// none or it does not parse.
func Read(path string) (State, bool) {
	b, err := os.ReadFile(path)
	if err != nil || len(b) > MaxMsg {
		return State{}, false
	}
	var st State
	if json.Unmarshal(b, &st) != nil {
		return State{}, false
	}
	return st, true
}
