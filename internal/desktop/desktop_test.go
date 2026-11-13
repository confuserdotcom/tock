package desktop

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDetect(t *testing.T) {
	env := func(m map[string]string) func(string) string { return func(k string) string { return m[k] } }
	empty := t.TempDir()
	for _, tt := range []struct {
		name string
		vars map[string]string
		want string
	}{
		{"forced", map[string]string{"TOCK_DESKTOP": "X11", "SWAYSOCK": "/s"}, X11},
		{"i3 by hand", map[string]string{"TOCK_DESKTOP": "i3"}, Sway},
		{"hyprland", map[string]string{"HYPRLAND_INSTANCE_SIGNATURE": "abc", "DISPLAY": ":0"}, Hyprland},
		{"sway", map[string]string{"SWAYSOCK": "/s", "DISPLAY": ":0", "WAYLAND_DISPLAY": "wayland-1"}, Sway},
		{"x11", map[string]string{"DISPLAY": ":0"}, X11},
		{"xwayland only", map[string]string{"DISPLAY": ":0", "WAYLAND_DISPLAY": "wayland-1"}, X11},
	} {
		got, err := Detect(Env{Getenv: env(tt.vars), RuntimeDir: empty, X11Dir: empty})
		if err != nil || got != tt.want {
			t.Errorf("%s = %q %v", tt.name, got, err)
		}
	}
	if _, err := Detect(Env{Getenv: env(map[string]string{"TOCK_DESKTOP": "gnome"})}); err == nil {
		t.Error("unknown TOCK_DESKTOP accepted")
	}
	if _, err := Detect(Env{Getenv: env(nil), RuntimeDir: empty, X11Dir: empty}); err == nil {
		t.Error("nothing found yet no error")
	}

	// A service without the session's variables finds it on disk.
	rt := t.TempDir()
	os.WriteFile(filepath.Join(rt, "sway-ipc.1000.5.sock"), nil, 0o600)
	if d, _ := Detect(Env{Getenv: env(nil), RuntimeDir: rt, X11Dir: empty}); d != Sway {
		t.Errorf("sway on disk = %q", d)
	}
	os.MkdirAll(filepath.Join(rt, "hypr", "sig"), 0o700)
	os.WriteFile(filepath.Join(rt, "hypr", "sig", ".socket2.sock"), nil, 0o600)
	if d, _ := Detect(Env{Getenv: env(nil), RuntimeDir: rt, X11Dir: empty}); d != Hyprland {
		t.Errorf("hyprland on disk = %q", d)
	}
	xd := t.TempDir()
	os.WriteFile(filepath.Join(xd, "X0"), nil, 0o600)
	if d, _ := Detect(Env{Getenv: env(nil), RuntimeDir: empty, X11Dir: xd}); d != X11 {
		t.Errorf("x socket on disk = %q", d)
	}
}

func TestIdleHook(t *testing.T) {
	x := Env{Getenv: func(k string) string { return map[string]string{"DISPLAY": ":0"}[k] }}
	w := Env{Getenv: func(k string) string { return map[string]string{"DISPLAY": ":0", "WAYLAND_DISPLAY": "wayland-1"}[k] }}
	for _, tt := range []struct {
		e    Env
		d    string
		want string
	}{{w, Hyprland, "hypridle"}, {w, Sway, "swayidle"}, {x, Sway, ""}, {x, X11, ""}} {
		if got := IdleHook(tt.e, tt.d); got != tt.want {
			t.Errorf("%s = %q", tt.d, got)
		}
	}
}
