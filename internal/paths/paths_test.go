package paths

import "testing"

func env(m map[string]string) func(string) string { return func(k string) string { return m[k] } }

func TestUnixLayoutIsUnchanged(t *testing.T) {
	// The layout every macOS and Linux install already has: never move it.
	for _, goos := range []string{"darwin", "linux", "freebsd"} {
		got := For(goos, env(nil), "/home/ann")
		want := Layout{Data: "/home/ann/.local/share/tock", Config: "/home/ann/.config/tock", State: "/home/ann/.local/state/tock"}
		if got != want {
			t.Errorf("%s: %+v, want %+v", goos, got, want)
		}
	}
}

func TestOverridesWin(t *testing.T) {
	e := env(map[string]string{"TOCK_DATA_DIR": "/d", "TOCK_CONFIG_DIR": "/c", "TOCK_STATE_DIR": "/s", "LOCALAPPDATA": `C:\L`, "APPDATA": `C:\R`})
	for _, goos := range []string{"darwin", "windows"} {
		if got := For(goos, e, "/home/ann"); got != (Layout{Data: "/d", Config: "/c", State: "/s"}) {
			t.Errorf("%s: %+v", goos, got)
		}
	}
}

func TestWindowsLayout(t *testing.T) {
	e := env(map[string]string{"LOCALAPPDATA": `C:\Users\José Ñ\AppData\Local`, "APPDATA": `C:\Users\José Ñ\AppData\Roaming\`})
	got := For("windows", e, `C:\Users\José Ñ`)
	want := Layout{
		Data:   `C:\Users\José Ñ\AppData\Local\tock\data`,
		Config: `C:\Users\José Ñ\AppData\Roaming\tock`,
		State:  `C:\Users\José Ñ\AppData\Local\tock\state`,
	}
	if got != want {
		t.Fatalf("%+v\nwant %+v", got, want)
	}
}

func TestWindowsWithoutAppDataVars(t *testing.T) {
	got := For("windows", env(nil), `C:\Users\ann`)
	want := Layout{Data: `C:\Users\ann\AppData\Local\tock\data`, Config: `C:\Users\ann\AppData\Roaming\tock`, State: `C:\Users\ann\AppData\Local\tock\state`}
	if got != want {
		t.Fatalf("%+v\nwant %+v", got, want)
	}
}

func TestExpand(t *testing.T) {
	e := env(map[string]string{"APPDATA": `C:\R`, "USERPROFILE": `C:\Users\ann`})
	cases := []struct{ goos, in, want string }{
		{"darwin", "~/x/y", "/home/ann/x/y"},
		{"darwin", "~", "/home/ann"},
		{"darwin", `~\x`, `~\x`}, // a backslash is a file name character on Unix
		{"darwin", "%APPDATA%/x", "%APPDATA%/x"},
		{"darwin", "/abs", "/abs"},
		{"windows", `~\x\y`, `C:\Users\ann\x\y`},
		{"windows", "~/x", `C:\Users\ann\x`},
		{"windows", `%APPDATA%\tock`, `C:\R\tock`},
		{"windows", `%appdata%\tock`, `C:\R\tock`},
		{"windows", `%NOPE%\tock`, `%NOPE%\tock`},
		{"windows", `D:\work`, `D:\work`},
	}
	for _, c := range cases {
		home := "/home/ann"
		if c.goos == "windows" {
			home = `C:\Users\ann`
		}
		if got := Expand(c.goos, e, home, c.in); got != c.want {
			t.Errorf("%s %q = %q, want %q", c.goos, c.in, got, c.want)
		}
	}
}

func TestDefaultUsesThisMachine(t *testing.T) {
	t.Setenv("TOCK_DATA_DIR", "/tmp/d")
	if Default().Data != "/tmp/d" {
		t.Fatal("Default ignores TOCK_DATA_DIR")
	}
	if Home() == "" {
		t.Fatal("no home")
	}
}

func TestStandardIgnoresOverrides(t *testing.T) {
	t.Setenv("TOCK_DATA_DIR", "/tmp/d")
	if Standard().Data == "/tmp/d" {
		t.Fatal("Standard took TOCK_DATA_DIR")
	}
	if Standard().Data != For(runtimeGOOS, func(string) string { return "" }, Home()).Data && runtimeGOOS != "windows" {
		t.Fatal("Standard is not the plain layout")
	}
}
