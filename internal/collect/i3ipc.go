package collect

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"tock/internal/desktop"
)

// i3 IPC, which Sway speaks too: "i3-ipc", the payload's length and the
// message type (both uint32, native order: little-endian everywhere tock
// runs), then the payload.
const (
	i3Magic        = "i3-ipc"
	i3RunCommand   = 0
	i3Subscribe    = 2
	i3GetTree      = 4
	i3EventWindow  = 0x80000003
	i3MaxPayload   = 64 << 20
	i3HeaderLength = len(i3Magic) + 8
)

func i3Encode(typ uint32, payload []byte) []byte {
	b := make([]byte, i3HeaderLength, i3HeaderLength+len(payload))
	copy(b, i3Magic)
	binary.LittleEndian.PutUint32(b[6:], uint32(len(payload)))
	binary.LittleEndian.PutUint32(b[10:], typ)
	return append(b, payload...)
}

func i3Read(r io.Reader) (typ uint32, payload []byte, err error) {
	var h [i3HeaderLength]byte
	if _, err := io.ReadFull(r, h[:]); err != nil {
		return 0, nil, err
	}
	if string(h[:6]) != i3Magic {
		return 0, nil, errors.New("i3 ipc: bad magic")
	}
	n := binary.LittleEndian.Uint32(h[6:])
	if n > i3MaxPayload {
		return 0, nil, fmt.Errorf("i3 ipc: payload of %d bytes", n)
	}
	payload = make([]byte, n)
	if _, err := io.ReadFull(r, payload); err != nil {
		return 0, nil, err
	}
	return binary.LittleEndian.Uint32(h[10:]), payload, nil
}

// i3Node is the part of a tree node tock reads.
type i3Node struct {
	Type      string `json:"type"`
	Name      string `json:"name"`
	Focused   bool   `json:"focused"`
	AppID     string `json:"app_id"`
	WindowPro struct {
		Class string `json:"class"`
	} `json:"window_properties"`
	Nodes         []i3Node `json:"nodes"`
	FloatingNodes []i3Node `json:"floating_nodes"`
}

// class is the window's app: Wayland's app_id, else X11's WM_CLASS class
// (Xwayland under Sway, everything under i3). A container that is not a
// window (a workspace) has none.
func (n *i3Node) class() string {
	if n.AppID != "" {
		return n.AppID
	}
	return n.WindowPro.Class
}

// focusedWindow finds the focused node of a GET_TREE reply: a window
// gives its class and title, a focused workspace (nothing open) none.
func focusedWindow(tree []byte) (class, title string, err error) {
	var root i3Node
	if err := json.Unmarshal(tree, &root); err != nil {
		return "", "", fmt.Errorf("i3 ipc: tree: %w", err)
	}
	var walk func(n *i3Node) *i3Node
	walk = func(n *i3Node) *i3Node {
		if n.Focused {
			return n
		}
		for i := range n.Nodes {
			if f := walk(&n.Nodes[i]); f != nil {
				return f
			}
		}
		for i := range n.FloatingNodes {
			if f := walk(&n.FloatingNodes[i]); f != nil {
				return f
			}
		}
		return nil
	}
	f := walk(&root)
	if f == nil || f.class() == "" {
		return "", "", nil
	}
	return f.class(), f.Name, nil
}

// windowEvent reads a window event: a focus change, or a new title on the
// focused window, reports the window; anything else ok = false.
func windowEvent(payload []byte) (class, title string, ok bool) {
	var ev struct {
		Change    string `json:"change"`
		Container i3Node `json:"container"`
	}
	if json.Unmarshal(payload, &ev) != nil {
		return "", "", false
	}
	c := &ev.Container
	switch {
	case ev.Change == "focus", ev.Change == "title" && c.Focused:
		return c.class(), c.Name, true
	case ev.Change == "close" && c.Focused:
		return "", "", true
	}
	return "", "", false
}

// I3Socket finds the IPC socket: $SWAYSOCK, $I3SOCK, else the newest
// sway-ipc.*.sock under runtimeDir, else i3's ipc-socket.* under
// runtimeDir/i3. env reads the environment.
func I3Socket(env func(string) string, runtimeDir string) (string, error) {
	for _, k := range []string{"SWAYSOCK", "I3SOCK"} {
		if p := env(k); p != "" {
			return p, nil
		}
	}
	if runtimeDir == "" {
		return "", errors.New("sway: no SWAYSOCK and no XDG_RUNTIME_DIR")
	}
	for _, pat := range []string{filepath.Join(runtimeDir, "sway-ipc.*.sock"), filepath.Join(runtimeDir, "i3", "ipc-socket.*")} {
		if p := newest(pat); p != "" {
			return p, nil
		}
	}
	return "", errors.New("sway: no IPC socket under " + runtimeDir)
}

// newest is the most recently changed file matching pattern, "" for none.
func newest(pattern string) string {
	ms, _ := filepath.Glob(pattern)
	best, bestT := "", time.Time{}
	for _, m := range ms {
		fi, err := os.Stat(m)
		if err != nil {
			continue
		}
		if best == "" || fi.ModTime().After(bestT) {
			best, bestT = m, fi.ModTime()
		}
	}
	return best
}

// i3Backend is Sway or i3 over its IPC socket.
type i3Backend struct {
	sock string
	// x is the X server's idle time under i3 (on X), nil under Sway: then
	// swayidle's idle file says.
	x    *x11Backend
	mu   sync.Mutex
	ev   net.Conn // the subscribed connection, closed by close
	shut bool
}

// NewSway returns the Sway/i3 source on the IPC socket at sock ("" finds
// it). It fails when no socket is found. On X (i3) the X server gives the
// idle time when it can be reached.
func NewSway(sock string, opts LinuxOptions) (Source, error) {
	var x *x11Backend
	if desktop.OnX(desktop.System()) {
		x, _ = newX11Backend("")
	}
	if sock == "" {
		var err error
		if sock, err = I3Socket(os.Getenv, os.Getenv("XDG_RUNTIME_DIR")); err != nil {
			return nil, err
		}
	}
	return newWinSource(&i3Backend{sock: sock, x: x}, opts), nil
}

func (b *i3Backend) idleMS() (int64, bool) {
	if b.x == nil {
		return 0, false
	}
	return b.x.idleMS()
}

func (b *i3Backend) close() {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.shut = true
	if b.x != nil {
		b.x.close()
	}
	if b.ev != nil {
		b.ev.Close()
	}
}

// request sends one message on a fresh connection and reads its reply.
func i3Request(sock string, typ uint32, payload []byte) ([]byte, error) {
	c, err := net.DialTimeout("unix", sock, time.Second)
	if err != nil {
		return nil, err
	}
	defer c.Close()
	_ = c.SetDeadline(time.Now().Add(2 * time.Second))
	if _, err := c.Write(i3Encode(typ, payload)); err != nil {
		return nil, err
	}
	for {
		t, p, err := i3Read(c)
		if err != nil {
			return nil, err
		}
		if t == typ {
			return p, nil
		}
	}
}

func (b *i3Backend) active() (string, string, error) {
	tree, err := i3Request(b.sock, i3GetTree, nil)
	if err != nil {
		return "", "", fmt.Errorf("sway: tree: %w", err)
	}
	return focusedWindow(tree)
}

func (b *i3Backend) watch(focus func(class, title string)) error {
	c, err := net.Dial("unix", b.sock)
	if err != nil {
		return fmt.Errorf("sway: ipc socket: %w", err)
	}
	b.mu.Lock()
	if b.shut {
		b.mu.Unlock()
		c.Close()
		return nil
	}
	b.ev = c
	b.mu.Unlock()
	if _, err := c.Write(i3Encode(i3Subscribe, []byte(`["window"]`))); err != nil {
		return fmt.Errorf("sway: subscribe: %w", err)
	}
	for {
		typ, p, err := i3Read(c)
		if err != nil {
			return fmt.Errorf("sway: events: %w", err)
		}
		switch typ {
		case i3Subscribe:
			if !bytes.Contains(p, []byte(`"success":true`)) && !bytes.Contains(p, []byte(`"success": true`)) {
				return fmt.Errorf("sway: subscribe: %s", p)
			}
		case i3EventWindow:
			if class, title, ok := windowEvent(p); ok {
				focus(class, title)
			}
		}
	}
}

// SwayHideCommands are the commands that move every window of class to
// the scratchpad (Sway's scratchpad show brings one back): by app_id for
// Wayland apps, by class for X11 ones.
func SwayHideCommands(class string) []string {
	q := strings.ReplaceAll(regexp.QuoteMeta(class), `"`, `\"`)
	return []string{
		`[app_id="^` + q + `$"] move scratchpad`,
		`[class="^` + q + `$"] move scratchpad`,
	}
}

// SwayHide hides every window of class in the session's Sway or i3.
func SwayHide(class string) (bool, error) {
	sock, err := I3Socket(os.Getenv, os.Getenv("XDG_RUNTIME_DIR"))
	if err != nil {
		return false, err
	}
	return swayHide(sock, class)
}

func swayHide(sock, class string) (bool, error) {
	var last error
	for _, cmd := range SwayHideCommands(class) {
		p, err := i3Request(sock, i3RunCommand, []byte(cmd))
		if err != nil {
			return false, fmt.Errorf("sway: hide: %w", err)
		}
		var rs []struct {
			Success bool   `json:"success"`
			Error   string `json:"error"`
		}
		if err := json.Unmarshal(p, &rs); err != nil {
			return false, fmt.Errorf("sway: hide: %w", err)
		}
		for _, r := range rs {
			if r.Success {
				return true, nil
			}
			if r.Error != "" {
				last = errors.New("sway: " + r.Error)
			}
		}
	}
	return false, last
}
