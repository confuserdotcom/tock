package collect

import (
	"bytes"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"tock/internal/desktop"
)

func TestI3Framing(t *testing.T) {
	msg := i3Encode(i3Subscribe, []byte(`["window"]`))
	if string(msg[:6]) != "i3-ipc" || len(msg) != 14+10 {
		t.Fatalf("msg = %q", msg)
	}
	typ, p, err := i3Read(bytes.NewReader(msg))
	if err != nil || typ != i3Subscribe || string(p) != `["window"]` {
		t.Fatalf("read = %d %q %v", typ, p, err)
	}
	if _, _, err := i3Read(bytes.NewReader([]byte("nope-ipc\x00\x00\x00\x00\x00\x00"))); err == nil {
		t.Fatal("bad magic read")
	}
}

const swayTree = `{"type":"root","nodes":[{"type":"output","nodes":[{"type":"workspace","name":"1","nodes":[
 {"type":"con","name":"~/tock","app_id":"foot","focused":false},
 {"type":"con","name":"Docs — Mozilla Firefox","app_id":"firefox","focused":true}]}]}]}`

func TestFocusedWindow(t *testing.T) {
	c, ti, err := focusedWindow([]byte(swayTree))
	if err != nil || c != "firefox" || ti != "Docs — Mozilla Firefox" {
		t.Fatalf("= %q %q %v", c, ti, err)
	}
	// Xwayland (and i3): the class comes from window_properties; floating too.
	x := `{"nodes":[{"type":"workspace","floating_nodes":[{"type":"floating_con","name":"Slack","focused":true,"window_properties":{"class":"Slack"}}]}]}`
	if c, ti, _ := focusedWindow([]byte(x)); c != "Slack" || ti != "Slack" {
		t.Fatalf("xwayland = %q %q", c, ti)
	}
	// An empty workspace in focus is the desktop.
	if c, _, err := focusedWindow([]byte(`{"nodes":[{"type":"workspace","name":"3","focused":true}]}`)); err != nil || c != "" {
		t.Fatalf("workspace = %q %v", c, err)
	}
	if _, _, err := focusedWindow([]byte(`{`)); err == nil {
		t.Fatal("bad json")
	}
}

func TestWindowEvent(t *testing.T) {
	for _, tt := range []struct {
		in, class, title string
		ok               bool
	}{
		{`{"change":"focus","container":{"app_id":"foot","name":"vim"}}`, "foot", "vim", true},
		{`{"change":"title","container":{"app_id":"foot","name":"make","focused":true}}`, "foot", "make", true},
		{`{"change":"title","container":{"app_id":"foot","name":"bg","focused":false}}`, "", "", false},
		{`{"change":"close","container":{"app_id":"foot","focused":true}}`, "", "", true},
		{`{"change":"new","container":{"app_id":"foot"}}`, "", "", false},
		{`nope`, "", "", false},
	} {
		c, ti, ok := windowEvent([]byte(tt.in))
		if c != tt.class || ti != tt.title || ok != tt.ok {
			t.Errorf("%s = %q %q %v", tt.in, c, ti, ok)
		}
	}
}

func TestI3Socket(t *testing.T) {
	env := func(m map[string]string) func(string) string { return func(k string) string { return m[k] } }
	if p, _ := I3Socket(env(map[string]string{"SWAYSOCK": "/s", "I3SOCK": "/i"}), ""); p != "/s" {
		t.Fatalf("SWAYSOCK = %q", p)
	}
	dir := t.TempDir()
	if _, err := I3Socket(env(nil), dir); err == nil {
		t.Fatal("no socket found nothing")
	}
	old, cur := filepath.Join(dir, "sway-ipc.1000.10.sock"), filepath.Join(dir, "sway-ipc.1000.20.sock")
	os.WriteFile(old, nil, 0o600)
	os.WriteFile(cur, nil, 0o600)
	os.Chtimes(old, time.Now().Add(-time.Hour), time.Now().Add(-time.Hour))
	if p, _ := I3Socket(env(nil), dir); p != cur {
		t.Fatalf("newest = %q", p)
	}
}

// fakeSway answers GET_TREE with tree, RUN_COMMAND with run, and streams
// events to subscribers.
type fakeSway struct {
	sock   string
	tree   string
	run    func(cmd string) string
	events chan string
	cmds   chan string
}

func newFakeSway(t *testing.T) *fakeSway {
	t.Helper()
	dir, err := os.MkdirTemp("", "sw")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })
	f := &fakeSway{sock: filepath.Join(dir, "s.sock"), tree: swayTree, events: make(chan string, 8), cmds: make(chan string, 8),
		run: func(string) string { return `[{"success":true}]` }}
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
			go func() {
				defer c.Close()
				typ, p, err := i3Read(c)
				if err != nil {
					return
				}
				switch typ {
				case i3GetTree:
					c.Write(i3Encode(i3GetTree, []byte(f.tree)))
				case i3RunCommand:
					f.cmds <- string(p)
					c.Write(i3Encode(i3RunCommand, []byte(f.run(string(p)))))
				case i3Subscribe:
					c.Write(i3Encode(i3Subscribe, []byte(`{"success": true}`)))
					for ev := range f.events {
						c.Write(i3Encode(i3EventWindow, []byte(ev)))
					}
				}
			}()
		}
	}()
	return f
}

func TestSwaySource(t *testing.T) {
	f := newFakeSway(t)
	src, err := NewSway(f.sock, LinuxOptions{PollInterval: time.Hour, Proc: t.TempDir(), Now: func() int64 { return 7000 }})
	if err != nil {
		t.Fatal(err)
	}
	runErr := make(chan error, 1)
	go func() { runErr <- src.Run() }()
	s := next(t, src.Inputs())
	if s.BundleID != "firefox" || *s.Title != "Docs — Mozilla Firefox" || s.Activation || s.TS != 7000 {
		t.Fatalf("poll sample = %+v", s)
	}
	f.events <- `{"change":"new","container":{"app_id":"foot"}}`
	f.events <- `{"change":"focus","container":{"app_id":"foot","name":"~/tock"}}`
	if s = next(t, src.Inputs()); s.BundleID != "foot" || *s.Title != "~/tock" || !s.Activation {
		t.Fatalf("event sample = %+v", s)
	}
	f.events <- `{"change":"close","container":{"app_id":"foot","focused":true}}`
	if s = next(t, src.Inputs()); s.BundleID != DesktopBundleID {
		t.Fatalf("desktop = %+v", s)
	}
	src.Stop()
	if err := <-runErr; err != nil {
		t.Fatalf("Run after Stop = %v", err)
	}

	// Sway exits: Run says so.
	src2, _ := NewSway(f.sock, LinuxOptions{PollInterval: time.Hour, Proc: t.TempDir()})
	go func() { runErr <- src2.Run() }()
	next(t, src2.Inputs())
	close(f.events)
	select {
	case err := <-runErr:
		if err == nil || !strings.Contains(err.Error(), "sway: events") {
			t.Fatalf("Run = %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("Run did not return")
	}
}

func TestSwayHide(t *testing.T) {
	f := newFakeSway(t)
	f.run = func(cmd string) string {
		if strings.HasPrefix(cmd, "[app_id") {
			return `[{"success":false,"error":"No matching node."}]`
		}
		return `[{"success":true}]`
	}
	ok, err := swayHide(f.sock, `Steam "beta"`)
	if !ok || err != nil {
		t.Fatalf("hide = %v %v", ok, err)
	}
	if c := <-f.cmds; c != `[app_id="^Steam \"beta\"$"] move scratchpad` {
		t.Fatalf("first = %s", c)
	}
	if c := <-f.cmds; c != `[class="^Steam \"beta\"$"] move scratchpad` {
		t.Fatalf("second = %s", c)
	}
	f.run = func(string) string { return `[{"success":false,"error":"No matching node."}]` }
	if ok, err := swayHide(f.sock, "x"); ok || err == nil || !strings.Contains(err.Error(), "No matching") {
		t.Fatalf("no match = %v %v", ok, err)
	}
}

func TestHideWindowsX11(t *testing.T) {
	if ok, err := HideWindows(desktop.X11, "x"); ok || err != ErrUnsupported {
		t.Errorf("x11 hide = %v %v", ok, err)
	}
}
