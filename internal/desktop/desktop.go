// Package desktop names the Linux desktop session trackd reads. It does no
// I/O beyond looking for the session's sockets, and needs no cgo, so the
// tock CLI can ask too.
package desktop

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// The Linux desktops trackd can read.
const (
	Hyprland = "hyprland"
	Sway     = "sway" // also i3: the same IPC
	X11      = "x11"
)

// Env is what Detect looks at; tests fill it by hand.
type Env struct {
	Getenv     func(string) string
	RuntimeDir string // $XDG_RUNTIME_DIR
	X11Dir     string // where X sockets live, /tmp/.X11-unix
}

// System is the running process's Env.
func System() Env {
	return Env{Getenv: os.Getenv, RuntimeDir: os.Getenv("XDG_RUNTIME_DIR"), X11Dir: "/tmp/.X11-unix"}
}

// Detect names the session's desktop. TOCK_DESKTOP picks one by hand;
// else the session's own variables (HYPRLAND_INSTANCE_SIGNATURE,
// SWAYSOCK/I3SOCK, DISPLAY), then what is found on disk: a Hyprland
// instance, a Sway or i3 socket, an X socket. A service started before
// the desktop exported its variables still finds it that way.
func Detect(e Env) (string, error) {
	switch d := strings.ToLower(strings.TrimSpace(e.Getenv("TOCK_DESKTOP"))); d {
	case "":
	case Hyprland, Sway, X11:
		return d, nil
	case "i3":
		return Sway, nil
	default:
		return "", fmt.Errorf("desktop: TOCK_DESKTOP=%q: want hyprland, sway, i3 or x11", d)
	}
	switch {
	case e.Getenv("HYPRLAND_INSTANCE_SIGNATURE") != "":
		return Hyprland, nil
	case e.Getenv("SWAYSOCK") != "" || e.Getenv("I3SOCK") != "":
		return Sway, nil
	case e.Getenv("WAYLAND_DISPLAY") == "" && e.Getenv("DISPLAY") != "":
		return X11, nil
	}
	if e.RuntimeDir != "" {
		if exists(filepath.Join(e.RuntimeDir, "hypr", "*", ".socket2.sock")) {
			return Hyprland, nil
		}
		if exists(filepath.Join(e.RuntimeDir, "sway-ipc.*.sock")) || exists(filepath.Join(e.RuntimeDir, "i3", "ipc-socket.*")) {
			return Sway, nil
		}
	}
	if e.Getenv("DISPLAY") != "" || exists(filepath.Join(e.X11Dir, "X*")) {
		return X11, nil
	}
	return "", errors.New("desktop: no Hyprland, Sway, i3 or X11 session found (set TOCK_DESKTOP to pick one)")
}

// OnX reports whether the session runs on an X server rather than
// Wayland: i3, or Sway's Xwayland seen from a shell that only has DISPLAY.
func OnX(e Env) bool { return e.Getenv("WAYLAND_DISPLAY") == "" && e.Getenv("DISPLAY") != "" }

// IdleHook names the program that must run trackd idle for trackd to see
// input on desktop d, "" when the desktop tells it directly (X11, and i3
// on X).
func IdleHook(e Env, d string) string {
	switch {
	case d == Hyprland:
		return "hypridle"
	case d == Sway && !OnX(e):
		return "swayidle"
	}
	return ""
}

func exists(pattern string) bool {
	ms, _ := filepath.Glob(pattern)
	return len(ms) > 0
}
