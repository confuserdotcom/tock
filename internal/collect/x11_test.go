package collect

import (
	"bytes"
	"encoding/binary"
	"io"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"tock/internal/testfs"
)

func TestXDisplay(t *testing.T) {
	for _, tt := range []struct{ in, sock, num string }{
		{":0", "/tmp/.X11-unix/X0", "0"},
		{":1.0", "/tmp/.X11-unix/X1", "1"},
		{"unix:2", "/tmp/.X11-unix/X2", "2"},
	} {
		if s, n, err := xDisplay(tt.in); err != nil || s != tt.sock || n != tt.num {
			t.Errorf("%q = %q %q %v", tt.in, s, n, err)
		}
	}
	for _, bad := range []string{"remote:0", "0", ":x"} {
		if _, _, err := xDisplay(bad); err == nil {
			t.Errorf("%q accepted", bad)
		}
	}
}

// xauthEntry is one Xauthority record.
func xauthEntry(family uint16, addr, num, name string, data []byte) []byte {
	var b bytes.Buffer
	binary.Write(&b, binary.BigEndian, family)
	for _, s := range [][]byte{[]byte(addr), []byte(num), []byte(name), data} {
		binary.Write(&b, binary.BigEndian, uint16(len(s)))
		b.Write(s)
	}
	return b.Bytes()
}

func TestXCookie(t *testing.T) {
	testfs.UnixOnly(t, "X11 runs on Linux")
	file := append(append(append(
		xauthEntry(256, "other", "0", "MIT-MAGIC-COOKIE-1", []byte("nope")),
		xauthEntry(65535, "", "", "MIT-MAGIC-COOKIE-1", []byte("wild"))...),
		xauthEntry(256, "box", "1", "MIT-MAGIC-COOKIE-1", []byte("one"))...),
		xauthEntry(256, "box", "0", "MIT-MAGIC-COOKIE-1", []byte("zero"))...)
	if c := xCookie(file, "box", "0"); string(c) != "zero" {
		t.Fatalf("local = %q", c)
	}
	if c := xCookie(file, "elsewhere", "0"); string(c) != "wild" {
		t.Fatalf("wild = %q", c)
	}
	if c := xCookie(file[:5], "box", "0"); c != nil {
		t.Fatalf("truncated = %q", c)
	}
	env := func(k string) string { return map[string]string{"XAUTHORITY": "/a", "XDG_RUNTIME_DIR": "/r"}[k] }
	if fs := xAuthFiles(env, "/h"); len(fs) < 3 || fs[0] != "/a" || fs[1] != "/h/.Xauthority" {
		t.Fatalf("files = %v", fs)
	}
}

// fakeX is an X server with one root window and the few requests tock sends.
type fakeX struct {
	sock   string
	cookie string
	mu     sync.Mutex
	atoms  map[string]uint32
	props  map[[2]uint32][]byte // window, atom → value (format 8, or 32 for the active window)
	idle   uint32
	notify chan uint32 // atoms to send PropertyNotify for
}

const xRoot = 0x2a

func newFakeX(t *testing.T, cookie string) *fakeX {
	t.Helper()
	dir, err := os.MkdirTemp("", "x")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })
	f := &fakeX{sock: filepath.Join(dir, "X0"), cookie: cookie, atoms: map[string]uint32{}, props: map[[2]uint32][]byte{},
		notify: make(chan uint32, 4)}
	l, err := net.Listen("unix", f.sock)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { l.Close() })
	go func() {
		for {
			c, err := l.Accept()
			if err != nil {
				return
			}
			go f.serve(c)
		}
	}()
	return f
}

func (f *fakeX) atom(name string) uint32 {
	f.mu.Lock()
	defer f.mu.Unlock()
	if a, ok := f.atoms[name]; ok {
		return a
	}
	a := uint32(100 + len(f.atoms))
	f.atoms[name] = a
	return a
}

func (f *fakeX) set(window uint32, prop string, v []byte) {
	a := f.atom(prop)
	f.mu.Lock()
	f.props[[2]uint32{window, a}] = v
	f.mu.Unlock()
}

func (f *fakeX) setActive(w uint32) {
	v := make([]byte, 4)
	binary.LittleEndian.PutUint32(v, w)
	f.set(xRoot, "_NET_ACTIVE_WINDOW", v)
}

func reply(extra []byte, fill func(m []byte)) []byte {
	m := make([]byte, 32, 32+len(extra)+3)
	m[0] = 1
	binary.LittleEndian.PutUint32(m[4:], uint32((len(extra)+pad4(len(extra)))/4))
	if fill != nil {
		fill(m)
	}
	m = append(m, extra...)
	return append(m, make([]byte, pad4(len(extra)))...)
}

func (f *fakeX) serve(c net.Conn) {
	defer c.Close()
	var h [12]byte
	if _, err := io.ReadFull(c, h[:]); err != nil || h[0] != 'l' {
		return
	}
	nl, dl := int(binary.LittleEndian.Uint16(h[6:])), int(binary.LittleEndian.Uint16(h[8:]))
	rest := make([]byte, nl+pad4(nl)+dl+pad4(dl))
	io.ReadFull(c, rest)
	got := string(rest[nl+pad4(nl) : nl+pad4(nl)+dl])
	if got != f.cookie {
		reason := "No protocol specified"
		m := make([]byte, 8)
		m[1] = byte(len(reason))
		binary.LittleEndian.PutUint16(m[6:], uint16((len(reason)+pad4(len(reason)))/4))
		c.Write(append(append(m, reason...), make([]byte, pad4(len(reason)))...))
		return
	}
	// Setup reply: 32 bytes of fixed info, vendor "fake" (4), one format
	// (8), then the screen starting with the root window.
	body := make([]byte, 32+4+8+40)
	binary.LittleEndian.PutUint16(body[16:], 4)
	body[20], body[21] = 1, 1
	copy(body[32:], "fake")
	binary.LittleEndian.PutUint32(body[44:], xRoot)
	m := make([]byte, 8)
	m[0] = 1
	binary.LittleEndian.PutUint16(m[6:], uint16(len(body)/4))
	c.Write(append(m, body...))

	var wmu sync.Mutex
	write := func(b []byte) { wmu.Lock(); c.Write(b); wmu.Unlock() }
	for {
		var rh [4]byte
		if _, err := io.ReadFull(c, rh[:]); err != nil {
			return
		}
		req := make([]byte, int(binary.LittleEndian.Uint16(rh[2:]))*4-4)
		if _, err := io.ReadFull(c, req); err != nil {
			return
		}
		switch rh[0] {
		case xOpInternAtom:
			n := int(binary.LittleEndian.Uint16(req[0:]))
			a := f.atom(string(req[4 : 4+n]))
			write(reply(nil, func(m []byte) { binary.LittleEndian.PutUint32(m[8:], a) }))
		case xOpGetProperty:
			w, a := binary.LittleEndian.Uint32(req[0:]), binary.LittleEndian.Uint32(req[4:])
			f.mu.Lock()
			v, ok := f.props[[2]uint32{w, a}]
			f.mu.Unlock()
			if !ok {
				write(reply(nil, nil))
				continue
			}
			format, n := 8, len(v)
			if a == f.atom("_NET_ACTIVE_WINDOW") {
				format, n = 32, len(v)/4
			}
			write(reply(v, func(m []byte) {
				m[1] = byte(format)
				binary.LittleEndian.PutUint32(m[8:], 1)
				binary.LittleEndian.PutUint32(m[16:], uint32(n))
			}))
		case xOpQueryExtension:
			write(reply(nil, func(m []byte) { m[8], m[9] = 1, 140 }))
		case 140: // MIT-SCREEN-SAVER
			f.mu.Lock()
			idle := f.idle
			f.mu.Unlock()
			write(reply(nil, func(m []byte) { binary.LittleEndian.PutUint32(m[16:], idle) }))
		case xOpChangeWindowAttributes:
			go func() {
				for a := range f.notify {
					ev := make([]byte, 32)
					ev[0] = xPropertyNotify
					binary.LittleEndian.PutUint32(ev[4:], xRoot)
					binary.LittleEndian.PutUint32(ev[8:], a)
					write(ev)
				}
				c.Close()
			}()
		default:
			e := make([]byte, 32)
			e[1], e[10] = 1, rh[0]
			write(e)
		}
	}
}

func TestX11Source(t *testing.T) {
	f := newFakeX(t, "secret")
	f.setActive(0x500)
	f.set(0x500, "WM_CLASS", []byte("kitty\x00kitty\x00"))
	f.set(0x500, "_NET_WM_NAME", []byte("~/tock — vim"))
	f.set(0x600, "WM_CLASS", []byte("Navigator\x00firefox\x00"))
	f.set(0x600, "WM_NAME", []byte("Docs"))
	f.idle = 4500

	if _, err := xDial(f.sock, []byte("wrong")); err == nil || !strings.Contains(err.Error(), "No protocol specified") {
		t.Fatalf("bad cookie = %v", err)
	}
	b := &x11Backend{sock: f.sock, cookie: []byte("secret")}
	if err := b.connect(); err != nil {
		t.Fatal(err)
	}
	if b.req.root != xRoot || b.ss != 140 {
		t.Fatalf("root %x ss %d", b.req.root, b.ss)
	}
	src := newWinSource(b, LinuxOptions{PollInterval: time.Hour, Proc: t.TempDir(), Now: func() int64 { return 9000 }})
	runErr := make(chan error, 1)
	go func() { runErr <- src.Run() }()
	s := next(t, src.Inputs())
	if s.BundleID != "kitty" || *s.Title != "~/tock — vim" || s.IdleS != 4.5 || s.Activation {
		t.Fatalf("poll = %+v", s)
	}
	// Focus moves: the root's _NET_ACTIVE_WINDOW changes and says so.
	f.setActive(0x600)
	time.Sleep(50 * time.Millisecond) // the listener connection selects first
	f.notify <- f.atom("_NET_ACTIVE_WINDOW")
	if s = next(t, src.Inputs()); s.BundleID != "firefox" || *s.Title != "Docs" || !s.Activation {
		t.Fatalf("event = %+v", s)
	}
	f.notify <- f.atom("WM_NAME") // another property: nothing
	f.setActive(0)                // focus moving through None: no activation
	f.notify <- f.atom("_NET_ACTIVE_WINDOW")
	select {
	case in := <-src.Inputs():
		t.Fatalf("sample for a cleared active window: %+v", in.Sample)
	case <-time.After(200 * time.Millisecond):
	}
	if class, _, err := b.active(); err != nil || class != "" {
		t.Fatalf("a poll still sees the desktop: %q %v", class, err)
	}
	src.Stop()
	if err := <-runErr; err != nil {
		t.Fatalf("Run after Stop = %v", err)
	}
}

func TestX11SourceServerGone(t *testing.T) {
	f := newFakeX(t, "")
	f.setActive(0)
	b := &x11Backend{sock: f.sock}
	if err := b.connect(); err != nil {
		t.Fatal(err)
	}
	src := newWinSource(b, LinuxOptions{PollInterval: time.Hour, Proc: t.TempDir()})
	runErr := make(chan error, 1)
	go func() { runErr <- src.Run() }()
	next(t, src.Inputs())
	time.Sleep(50 * time.Millisecond)
	close(f.notify) // the server drops the listener
	select {
	case err := <-runErr:
		if err == nil || !strings.Contains(err.Error(), "x11: events") {
			t.Fatalf("Run = %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("Run did not return")
	}
}

func TestWMClass(t *testing.T) {
	if c := wmClass([]byte("Navigator\x00firefox\x00")); c != "firefox" {
		t.Fatal(c)
	}
	if c := wmClass([]byte("solo")); c != "solo" {
		t.Fatal(c)
	}
}
