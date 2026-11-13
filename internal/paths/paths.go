// Package paths says where tock keeps its files, the same for every binary.
//
// macOS and Linux: ~/.local/share/tock (data), ~/.config/tock (config),
// ~/.local/state/tock (state). Windows: %LOCALAPPDATA%\tock\data and
// \state, %APPDATA%\tock for config. TOCK_DATA_DIR, TOCK_CONFIG_DIR and
// TOCK_STATE_DIR override each one everywhere.
package paths

import (
	"os"
	"path"
	"runtime"
	"strings"
)

// Layout is the three directories tock uses.
type Layout struct {
	Data, Config, State string
}

// For computes the layout for goos from the environment and the home dir.
// It reads nothing itself, so every platform's layout is testable anywhere.
func For(goos string, getenv func(string) string, home string) Layout {
	var l Layout
	if goos == "windows" {
		local := firstNonEmpty(getenv("LOCALAPPDATA"), join(goos, home, "AppData", "Local"))
		roaming := firstNonEmpty(getenv("APPDATA"), join(goos, home, "AppData", "Roaming"))
		l = Layout{
			Data:   join(goos, local, "tock", "data"),
			Config: join(goos, roaming, "tock"),
			State:  join(goos, local, "tock", "state"),
		}
	} else {
		l = Layout{
			Data:   join(goos, home, ".local", "share", "tock"),
			Config: join(goos, home, ".config", "tock"),
			State:  join(goos, home, ".local", "state", "tock"),
		}
	}
	if d := getenv("TOCK_DATA_DIR"); d != "" {
		l.Data = d
	}
	if d := getenv("TOCK_CONFIG_DIR"); d != "" {
		l.Config = d
	}
	if d := getenv("TOCK_STATE_DIR"); d != "" {
		l.State = d
	}
	return l
}

// Default is the layout on this machine.
func Default() Layout { return For(runtimeGOOS, os.Getenv, Home()) }

// Home is the user's home directory, "" only when nothing says.
func Home() string {
	if h, err := os.UserHomeDir(); err == nil {
		return h
	}
	return os.Getenv("HOME")
}

// Expand turns a leading ~ into home and, on Windows, a leading %VAR%
// (any case) into its value. A ~\ prefix counts only on Windows, where \ is
// the separator; an unknown %VAR% is left as written.
func Expand(goos string, getenv func(string) string, home, p string) string {
	if p == "~" {
		return home
	}
	if rest, ok := strings.CutPrefix(p, "~/"); ok {
		return join(goos, home, rest)
	}
	if goos != "windows" {
		return p
	}
	if rest, ok := strings.CutPrefix(p, `~\`); ok {
		return join(goos, home, rest)
	}
	if strings.HasPrefix(p, "%") {
		if end := strings.Index(p[1:], "%"); end > 0 {
			name := strings.ToUpper(p[1 : end+1])
			if v := getenv(name); v != "" {
				return join(goos, v, strings.TrimLeft(p[end+2:], `\/`))
			}
		}
	}
	return p
}

// join is filepath.Join for goos rather than for the running OS.
func join(goos string, parts ...string) string {
	if goos != "windows" {
		return path.Join(parts...)
	}
	var b strings.Builder
	for i, s := range parts {
		s = strings.ReplaceAll(s, "/", `\`)
		if i > 0 {
			s = strings.TrimLeft(s, `\`)
		}
		if i < len(parts)-1 {
			s = strings.TrimRight(s, `\`)
		}
		if s == "" {
			continue
		}
		if b.Len() > 0 {
			b.WriteByte('\\')
		}
		b.WriteString(s)
	}
	return b.String()
}

func firstNonEmpty(a, b string) string {
	if a != "" {
		return a
	}
	return b
}

// runtimeGOOS is runtime.GOOS, a variable for tests.
var runtimeGOOS = runtime.GOOS

// Standard is the layout on this machine without the TOCK_*_DIR overrides:
// where the background services (which never get the overrides) keep
// their files.
func Standard() Layout {
	return For(runtimeGOOS, func(k string) string {
		if strings.HasPrefix(k, "TOCK_") {
			return ""
		}
		return os.Getenv(k)
	}, Home())
}
