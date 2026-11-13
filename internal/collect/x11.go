package collect

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

// A minimal X11 client: enough of the core protocol to read the active
// window's class and title, hear when it changes, and ask the
// MIT-SCREEN-SAVER extension how long input has been idle. Requests go
// out little-endian; the server answers in the order we asked for.

const (
	xOpChangeWindowAttributes = 2
	xOpInternAtom             = 16
	xOpGetProperty            = 20
	xOpQueryExtension         = 98

	xCWEventMask        = 0x800
	xPropertyChangeMask = 0x400000
	xPropertyNotify     = 28
	xAnyPropertyType    = 0
	xSSQueryInfo        = 1
)

// xDisplay is the unix socket and display number DISPLAY names: ":0",
// ":1.0", "unix:0". A TCP display (host:0) is not supported.
func xDisplay(display string) (sock, num string, err error) {
	host, rest, ok := strings.Cut(display, ":")
	if !ok || (host != "" && host != "unix") {
		return "", "", fmt.Errorf("x11: display %q: only local displays are supported", display)
	}
	num, _, _ = strings.Cut(rest, ".")
	if _, err := strconv.Atoi(num); err != nil {
		return "", "", fmt.Errorf("x11: display %q", display)
	}
	return "/tmp/.X11-unix/X" + num, num, nil
}

// xCookie finds the MIT-MAGIC-COOKIE-1 for display num in an Xauthority
// file: a local entry for this host, or a wildcard; none means connect
// without one.
func xCookie(auth []byte, hostname, num string) []byte {
	r := bytes.NewReader(auth)
	str := func() ([]byte, bool) {
		var n uint16
		if binary.Read(r, binary.BigEndian, &n) != nil {
			return nil, false
		}
		b := make([]byte, n)
		_, err := io.ReadFull(r, b)
		return b, err == nil
	}
	var wild []byte
	for {
		var family uint16
		if binary.Read(r, binary.BigEndian, &family) != nil {
			return wild
		}
		addr, ok1 := str()
		number, ok2 := str()
		name, ok3 := str()
		data, ok4 := str()
		if !ok1 || !ok2 || !ok3 || !ok4 {
			return wild
		}
		if string(name) != "MIT-MAGIC-COOKIE-1" || (len(number) > 0 && string(number) != num) {
			continue
		}
		switch {
		case family == 256 && string(addr) == hostname: // FamilyLocal
			return data
		case family == 65535 && wild == nil: // FamilyWild
			wild = data
		}
	}
}

// xAuthFiles are where the cookie may be, in order.
func xAuthFiles(env func(string) string, home string) []string {
	var out []string
	if p := env("XAUTHORITY"); p != "" {
		out = append(out, p)
	}
	out = append(out, filepath.Join(home, ".Xauthority"))
	if d := env("XDG_RUNTIME_DIR"); d != "" {
		out = append(out, filepath.Join(d, "gdm", "Xauthority"))
		if ms, _ := filepath.Glob(filepath.Join(d, "xauth_*")); len(ms) > 0 {
			out = append(out, ms...)
		}
	}
	return out
}

func pad4(n int) int { return (4 - n%4) % 4 }

// xConn is one connection to the X server.
type xConn struct {
	c    net.Conn
	root uint32
}

// xDial connects to sock and sets up with cookie (nil: no auth).
func xDial(sock string, cookie []byte) (*xConn, error) {
	c, err := net.DialTimeout("unix", sock, 2*time.Second)
	if err != nil {
		return nil, fmt.Errorf("x11: connect: %w", err)
	}
	x := &xConn{c: c}
	if err := x.setup(cookie); err != nil {
		c.Close()
		return nil, err
	}
	return x, nil
}

func (x *xConn) setup(cookie []byte) error {
	name := ""
	if cookie != nil {
		name = "MIT-MAGIC-COOKIE-1"
	}
	req := make([]byte, 12, 12+len(name)+len(cookie)+8)
	req[0] = 'l'
	binary.LittleEndian.PutUint16(req[2:], 11)
	binary.LittleEndian.PutUint16(req[6:], uint16(len(name)))
	binary.LittleEndian.PutUint16(req[8:], uint16(len(cookie)))
	req = append(req, name...)
	req = append(req, make([]byte, pad4(len(name)))...)
	req = append(req, cookie...)
	req = append(req, make([]byte, pad4(len(cookie)))...)
	_ = x.c.SetDeadline(time.Now().Add(2 * time.Second))
	defer x.c.SetDeadline(time.Time{})
	if _, err := x.c.Write(req); err != nil {
		return fmt.Errorf("x11: setup: %w", err)
	}
	var h [8]byte
	if _, err := io.ReadFull(x.c, h[:]); err != nil {
		return fmt.Errorf("x11: setup: %w", err)
	}
	body := make([]byte, int(binary.LittleEndian.Uint16(h[6:]))*4)
	if _, err := io.ReadFull(x.c, body); err != nil {
		return fmt.Errorf("x11: setup: %w", err)
	}
	if h[0] != 1 {
		reason := body
		if h[0] == 0 && int(h[1]) <= len(body) {
			reason = body[:h[1]]
		}
		return fmt.Errorf("x11: setup refused: %s", strings.TrimSpace(string(reason)))
	}
	if len(body) < 32 {
		return errors.New("x11: setup: short reply")
	}
	vendor := int(binary.LittleEndian.Uint16(body[16:]))
	formats := int(body[21])
	at := 32 + vendor + pad4(vendor) + 8*formats
	if body[20] == 0 || len(body) < at+4 {
		return errors.New("x11: setup: no screen")
	}
	x.root = binary.LittleEndian.Uint32(body[at:])
	return nil
}

func (x *xConn) close() { x.c.Close() }

// request sends req and reads its reply (or error), skipping events. Only
// requests with replies go through here.
func (x *xConn) request(req []byte) ([]byte, error) {
	_ = x.c.SetDeadline(time.Now().Add(2 * time.Second))
	defer x.c.SetDeadline(time.Time{})
	if _, err := x.c.Write(req); err != nil {
		return nil, err
	}
	for {
		var m [32]byte
		if _, err := io.ReadFull(x.c, m[:]); err != nil {
			return nil, err
		}
		switch m[0] {
		case 0:
			return nil, fmt.Errorf("x11: error %d on request %d", m[1], m[10])
		case 1:
			extra := make([]byte, int(binary.LittleEndian.Uint32(m[4:]))*4)
			if _, err := io.ReadFull(x.c, extra); err != nil {
				return nil, err
			}
			return append(m[:], extra...), nil
		}
	}
}

func (x *xConn) internAtom(name string) (uint32, error) {
	req := make([]byte, 8, 8+len(name)+3)
	req[0] = xOpInternAtom
	binary.LittleEndian.PutUint16(req[2:], uint16(2+(len(name)+pad4(len(name)))/4))
	binary.LittleEndian.PutUint16(req[4:], uint16(len(name)))
	req = append(req, name...)
	req = append(req, make([]byte, pad4(len(name)))...)
	r, err := x.request(req)
	if err != nil {
		return 0, fmt.Errorf("x11: atom %s: %w", name, err)
	}
	return binary.LittleEndian.Uint32(r[8:]), nil
}

// property reads up to 64 KiB of window's property; nil when unset.
func (x *xConn) property(window, prop uint32) ([]byte, error) {
	req := make([]byte, 24)
	req[0] = xOpGetProperty
	binary.LittleEndian.PutUint16(req[2:], 6)
	binary.LittleEndian.PutUint32(req[4:], window)
	binary.LittleEndian.PutUint32(req[8:], prop)
	binary.LittleEndian.PutUint32(req[12:], xAnyPropertyType)
	binary.LittleEndian.PutUint32(req[20:], 16384)
	r, err := x.request(req)
	if err != nil {
		return nil, err
	}
	format := int(r[1])
	n := int(binary.LittleEndian.Uint32(r[16:])) * max(format/8, 1)
	if binary.LittleEndian.Uint32(r[8:]) == 0 || 32+n > len(r) {
		return nil, nil
	}
	return r[32 : 32+n], nil
}

// selectProperties asks for PropertyNotify events on window.
func (x *xConn) selectProperties(window uint32) error {
	req := make([]byte, 16)
	req[0] = xOpChangeWindowAttributes
	binary.LittleEndian.PutUint16(req[2:], 4)
	binary.LittleEndian.PutUint32(req[4:], window)
	binary.LittleEndian.PutUint32(req[8:], xCWEventMask)
	binary.LittleEndian.PutUint32(req[12:], xPropertyChangeMask)
	_, err := x.c.Write(req)
	return err
}

// extension is the major opcode of extension name, 0 when absent.
func (x *xConn) extension(name string) (byte, error) {
	req := make([]byte, 8, 8+len(name)+3)
	req[0] = xOpQueryExtension
	binary.LittleEndian.PutUint16(req[2:], uint16(2+(len(name)+pad4(len(name)))/4))
	binary.LittleEndian.PutUint16(req[4:], uint16(len(name)))
	req = append(req, name...)
	req = append(req, make([]byte, pad4(len(name)))...)
	r, err := x.request(req)
	if err != nil {
		return 0, err
	}
	if r[8] == 0 {
		return 0, nil
	}
	return r[9], nil
}

// idle is the screensaver's ms since the last input.
func (x *xConn) idle(major byte) (int64, error) {
	req := make([]byte, 8)
	req[0], req[1] = major, xSSQueryInfo
	binary.LittleEndian.PutUint16(req[2:], 2)
	binary.LittleEndian.PutUint32(req[4:], x.root)
	r, err := x.request(req)
	if err != nil {
		return 0, err
	}
	return int64(binary.LittleEndian.Uint32(r[16:])), nil
}

// x11Backend is an X11 session: one connection for requests, one that
// only listens for the root window's property changes.
type x11Backend struct {
	sock   string
	cookie []byte

	mu    sync.Mutex
	req   *xConn // nil until (re)connected
	ev    *xConn
	shut  bool
	ss    byte // MIT-SCREEN-SAVER's opcode, 0 = none
	atoms map[string]uint32
}

var xAtomNames = []string{"_NET_ACTIVE_WINDOW", "_NET_WM_NAME", "WM_NAME", "WM_CLASS"}

// NewX11 returns the X11 source for display ("" reads DISPLAY). It fails
// when the display cannot be reached.
func NewX11(display string, opts LinuxOptions) (Source, error) {
	b, err := newX11Backend(display)
	if err != nil {
		return nil, err
	}
	return newWinSource(b, opts), nil
}

// newX11Backend connects to display ("" reads DISPLAY, then :0).
func newX11Backend(display string) (*x11Backend, error) {
	if display == "" {
		display = os.Getenv("DISPLAY")
	}
	if display == "" {
		display = ":0"
	}
	sock, num, err := xDisplay(display)
	if err != nil {
		return nil, err
	}
	home, _ := os.UserHomeDir()
	host, _ := os.Hostname()
	var cookie []byte
	for _, p := range xAuthFiles(os.Getenv, home) {
		if b, err := os.ReadFile(p); err == nil {
			if cookie = xCookie(b, host, num); cookie != nil {
				break
			}
		}
	}
	b := &x11Backend{sock: sock, cookie: cookie}
	if err := b.connect(); err != nil {
		return nil, err
	}
	return b, nil
}

// connect opens the request connection and reads the atoms. Caller holds
// no lock.
func (b *x11Backend) connect() error {
	x, err := xDial(b.sock, b.cookie)
	if err != nil {
		return err
	}
	atoms := map[string]uint32{}
	for _, n := range xAtomNames {
		a, err := x.internAtom(n)
		if err != nil {
			x.close()
			return err
		}
		atoms[n] = a
	}
	ss, _ := x.extension("MIT-SCREEN-SAVER")
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.req != nil {
		b.req.close()
	}
	b.req, b.atoms, b.ss = x, atoms, ss
	return nil
}

func (b *x11Backend) close() {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.shut = true
	if b.req != nil {
		b.req.close()
	}
	if b.ev != nil {
		b.ev.close()
	}
}

// do runs f on the request connection, reconnecting once after a failure.
func (b *x11Backend) do(f func(x *xConn) error) error {
	b.mu.Lock()
	x := b.req
	b.mu.Unlock()
	if x != nil {
		if err := f(x); err == nil {
			return nil
		}
	}
	if err := b.connect(); err != nil {
		return err
	}
	b.mu.Lock()
	x = b.req
	b.mu.Unlock()
	return f(x)
}

func (b *x11Backend) active() (class, title string, err error) {
	err = b.do(func(x *xConn) error {
		b.mu.Lock()
		atoms := b.atoms
		b.mu.Unlock()
		v, err := x.property(x.root, atoms["_NET_ACTIVE_WINDOW"])
		if err != nil {
			return err
		}
		class, title = "", ""
		if len(v) < 4 {
			return nil
		}
		w := binary.LittleEndian.Uint32(v)
		if w == 0 {
			return nil
		}
		cl, err := x.property(w, atoms["WM_CLASS"])
		if err != nil {
			return nil // the window went away between the two reads
		}
		class = wmClass(cl)
		t, _ := x.property(w, atoms["_NET_WM_NAME"])
		if len(t) == 0 {
			t, _ = x.property(w, atoms["WM_NAME"])
		}
		title = string(bytes.TrimRight(t, "\x00"))
		if class == "" && title != "" {
			class = "x11"
		}
		return nil
	})
	return class, title, err
}

// wmClass is WM_CLASS's class part: "instance\0Class\0".
func wmClass(v []byte) string {
	parts := strings.Split(strings.TrimRight(string(v), "\x00"), "\x00")
	return parts[len(parts)-1]
}

func (b *x11Backend) idleMS() (int64, bool) {
	b.mu.Lock()
	ss := b.ss
	b.mu.Unlock()
	if ss == 0 {
		return 0, false
	}
	var ms int64
	if err := b.do(func(x *xConn) error {
		var err error
		ms, err = x.idle(ss)
		return err
	}); err != nil {
		return 0, false
	}
	return ms, true
}

func (b *x11Backend) watch(focus func(class, title string)) error {
	x, err := xDial(b.sock, b.cookie)
	if err != nil {
		return err
	}
	b.mu.Lock()
	if b.shut {
		b.mu.Unlock()
		x.close()
		return nil
	}
	b.ev = x
	active := b.atoms["_NET_ACTIVE_WINDOW"]
	b.mu.Unlock()
	if err := x.selectProperties(x.root); err != nil {
		return fmt.Errorf("x11: listen: %w", err)
	}
	for {
		var m [32]byte
		if _, err := io.ReadFull(x.c, m[:]); err != nil {
			return fmt.Errorf("x11: events: %w", err)
		}
		if m[0] == 1 { // a reply nobody asked for: skip its body
			io.CopyN(io.Discard, x.c, int64(binary.LittleEndian.Uint32(m[4:]))*4)
			continue
		}
		if m[0]&0x7f == xPropertyNotify && binary.LittleEndian.Uint32(m[8:]) == active {
			// Window managers clear the active window for a moment while
			// focus moves; a real empty desktop shows up on the next poll.
			if class, title, err := b.active(); err == nil && class != "" {
				focus(class, title)
			}
		}
	}
}
