package collect

import (
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"tock/internal/span"
)

func TestParseActiveWindowEvent(t *testing.T) {
	for _, tt := range []struct {
		line, class, title string
		ok                 bool
	}{
		{"activewindow>>kitty,~/tock: go test, ok", "kitty", "~/tock: go test, ok", true},
		{"activewindow>>,", "", "", true},
		{"activewindowv2>>55d0", "", "", false},
		{"workspace>>2", "", "", false},
	} {
		c, ti, ok := parseActiveWindowEvent(tt.line)
		if c != tt.class || ti != tt.title || ok != tt.ok {
			t.Errorf("%q = %q %q %v", tt.line, c, ti, ok)
		}
	}
	if c, ti, err := parseActiveWindow([]byte(`{"class":"firefox","title":"Docs — Mozilla Firefox","pid":7}`)); err != nil || c != "firefox" || ti != "Docs — Mozilla Firefox" {
		t.Fatalf("json = %q %q %v", c, ti, err)
	}
	if c, _, err := parseActiveWindow([]byte(`{}`)); err != nil || c != "" {
		t.Fatalf("empty = %q %v", c, err)
	}
}

func TestIdleFileAndLock(t *testing.T) {
	dir := t.TempDir()
	idle := filepath.Join(dir, "state", IdleFile)
	now := int64(1_000_000)
	if idleSeconds(idle, now) != 0 {
		t.Fatal("no file means active")
	}
	if err := WriteIdle(idle, now, 300*time.Second); err != nil {
		t.Fatal(err)
	}
	if got := idleSeconds(idle, now+60_000); got != 360 {
		t.Fatalf("idle = %v, want 360", got)
	}
	if fi, _ := os.Stat(idle); fi.Mode().Perm() != 0o600 {
		t.Fatalf("mode %v", fi.Mode())
	}
	if err := ClearIdle(idle); err != nil || idleSeconds(idle, now) != 0 {
		t.Fatal("clear")
	}
	if err := ClearIdle(idle); err != nil {
		t.Fatal("clearing twice is fine")
	}

	proc := filepath.Join(dir, "proc")
	for pid, comm := range map[string]string{"12": "kitty\n", "self": "x\n", "40": "Hyprland\n"} {
		os.MkdirAll(filepath.Join(proc, pid), 0o755)
		os.WriteFile(filepath.Join(proc, pid, "comm"), []byte(comm), 0o644)
	}
	if lockerRunning(proc) {
		t.Fatal("no locker yet")
	}
	os.MkdirAll(filepath.Join(proc, "99"), 0o755)
	os.WriteFile(filepath.Join(proc, "99", "comm"), []byte("hyprlock\n"), 0o644)
	if !lockerRunning(proc) {
		t.Fatal("hyprlock means locked")
	}
}

func TestHyprDir(t *testing.T) {
	rt := t.TempDir()
	if d, err := HyprDir(rt, "sig1"); err != nil || d != filepath.Join(rt, "hypr", "sig1") {
		t.Fatalf("named = %q %v", d, err)
	}
	if _, err := HyprDir(rt, ""); err == nil {
		t.Fatal("no instance must fail")
	}
	for i, name := range []string{"old", "new"} {
		d := filepath.Join(rt, "hypr", name)
		os.MkdirAll(d, 0o700)
		p := filepath.Join(d, ".socket2.sock")
		os.WriteFile(p, nil, 0o600)
		mt := time.Now().Add(time.Duration(i-5) * time.Minute)
		os.Chtimes(p, mt, mt)
	}
	os.MkdirAll(filepath.Join(rt, "hypr", "nosocket"), 0o700)
	if d, err := HyprDir(rt, ""); err != nil || filepath.Base(d) != "new" {
		t.Fatalf("newest = %q %v", d, err)
	}
	if _, err := HyprDir("", ""); err == nil {
		t.Fatal("no runtime dir must fail")
	}
}

// fakeHypr serves .socket.sock (one JSON reply per request) and
// .socket2.sock (the event stream, written by the test).
type fakeHypr struct {
	dir    string
	events chan string
	window string
}

func newFakeHypr(t *testing.T) *fakeHypr {
	t.Helper()
	dir, err := os.MkdirTemp("/tmp", "hy") // unix socket paths are short on macOS
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })
	f := &fakeHypr{dir: dir, events: make(chan string, 8), window: `{"class":"kitty","title":"~/tock"}`}
	req, err := net.Listen("unix", filepath.Join(dir, ".socket.sock"))
	if err != nil {
		t.Fatal(err)
	}
	ev, err := net.Listen("unix", filepath.Join(dir, ".socket2.sock"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { req.Close(); ev.Close() })
	go func() {
		for {
			c, err := req.Accept()
			if err != nil {
				return
			}
			buf := make([]byte, 64)
			n, _ := c.Read(buf)
			if string(buf[:n]) == "j/activewindow" {
				c.Write([]byte(f.window))
			}
			c.Close()
		}
	}()
	go func() {
		c, err := ev.Accept()
		if err != nil {
			return
		}
		defer c.Close()
		for line := range f.events {
			c.Write([]byte(line + "\n"))
		}
	}()
	return f
}

func next(t *testing.T, ch <-chan span.Input) *span.Sample {
	t.Helper()
	select {
	case in, ok := <-ch:
		if !ok {
			t.Fatal("channel closed")
		}
		return in.Sample
	case <-time.After(3 * time.Second):
		t.Fatal("no sample")
	}
	return nil
}

func TestHyprlandSource(t *testing.T) {
	f := newFakeHypr(t)
	src, err := NewHyprland(HyprOptions{Dir: f.dir, PollInterval: time.Hour, Proc: t.TempDir(),
		IdleFile: filepath.Join(t.TempDir(), IdleFile), Now: func() int64 { return 5000 }})
	if err != nil {
		t.Fatal(err)
	}
	runErr := make(chan error, 1)
	go func() { runErr <- src.Run() }()
	s := next(t, src.Inputs())
	if s.BundleID != "kitty" || s.App != "kitty" || s.Title == nil || *s.Title != "~/tock" || s.Activation || !s.AXTrusted || s.TS != 5000 {
		t.Fatalf("poll sample = %+v", s)
	}
	f.events <- "workspace>>2"
	f.events <- "activewindow>>firefox,Docs, notes — Mozilla Firefox"
	s = next(t, src.Inputs())
	if s.BundleID != "firefox" || *s.Title != "Docs, notes — Mozilla Firefox" || !s.Activation {
		t.Fatalf("event sample = %+v", s)
	}
	f.events <- "activewindow>>,"
	if s = next(t, src.Inputs()); s.BundleID != DesktopBundleID || s.App != DesktopApp || s.Title != nil {
		t.Fatalf("desktop sample = %+v", s)
	}
	src.Stop()
	if err := <-runErr; err != nil {
		t.Fatalf("Run after Stop = %v", err)
	}
	if _, ok := <-src.Inputs(); ok {
		t.Fatal("Inputs must close")
	}
}

func TestHyprlandSourceSocketLost(t *testing.T) {
	f := newFakeHypr(t)
	src, err := NewHyprland(HyprOptions{Dir: f.dir, PollInterval: time.Hour, Proc: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	runErr := make(chan error, 1)
	go func() { runErr <- src.Run() }()
	next(t, src.Inputs())
	close(f.events) // Hyprland exits
	select {
	case err := <-runErr:
		if err == nil || !strings.Contains(err.Error(), "closed") {
			t.Fatalf("Run = %v, want the socket error", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("Run did not return")
	}
	if _, err := NewHyprland(HyprOptions{Dir: t.TempDir()}); err != nil {
		t.Fatal("NewHyprland with a dir does not dial")
	}
	s2, _ := NewHyprland(HyprOptions{Dir: t.TempDir()})
	if err := s2.Run(); err == nil {
		t.Fatal("no socket must fail Run")
	}
}
