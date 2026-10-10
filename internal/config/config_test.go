package config

import (
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"

	"tock/internal/stats"
)

func write(t *testing.T, body string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), File)
	if err := os.WriteFile(p, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestLoadMissingIsDefault(t *testing.T) {
	c, err := Load(filepath.Join(t.TempDir(), File))
	if err != nil || !reflect.DeepEqual(c, Default()) || c.Score != stats.DefaultWeights {
		t.Fatalf("Load(missing) = %+v, %v", c, err)
	}
}

func TestLoadValid(t *testing.T) {
	c, err := Load(write(t, "[score]\ndepth = 0.6\nblocks = 0.2\ncalm = 0.2\n"))
	if err != nil {
		t.Fatal(err)
	}
	if c.Score != (stats.Weights{Depth: 0.6, Blocks: 0.2, Calm: 0.2}) {
		t.Fatalf("weights = %+v", c.Score)
	}
}

func TestLoadPartialKeepsDefaults(t *testing.T) {
	// depth 0.5 is the default; moving 0.1 from blocks to calm needs both keys.
	c, err := Load(write(t, "[score]\nblocks = 0.2\ncalm = 0.3\n"))
	if err != nil {
		t.Fatal(err)
	}
	if c.Score != (stats.Weights{Depth: 0.5, Blocks: 0.2, Calm: 0.3}) {
		t.Fatalf("weights = %+v", c.Score)
	}
}

func TestLoadStretchWeight(t *testing.T) {
	c, err := Load(write(t, "[score]\ndepth = 0.4\nstretch = 0.1\n"))
	if err != nil {
		t.Fatal(err)
	}
	if c.Score != (stats.Weights{Depth: 0.4, Blocks: 0.3, Calm: 0.2, Stretch: 0.1}) {
		t.Fatalf("weights = %+v", c.Score)
	}
}

func TestLoadAway(t *testing.T) {
	c, err := Load(write(t, "[away]\nmin = 15\nmax = 180\n"))
	if err != nil {
		t.Fatal(err)
	}
	if c.Away != (Away{MinMS: 15 * 60_000, MaxMS: 180 * 60_000}) {
		t.Fatalf("away = %+v", c.Away)
	}
	if Default().Away != (Away{MinMS: 10 * 60_000, MaxMS: 240 * 60_000}) {
		t.Fatalf("default away = %+v", Default().Away)
	}
}

func TestLoadEmptyFile(t *testing.T) {
	if c, err := Load(write(t, "")); err != nil || !reflect.DeepEqual(c, Default()) {
		t.Fatalf("Load(empty) = %+v, %v", c, err)
	}
}

func TestLoadErrors(t *testing.T) {
	tests := []struct{ body, want string }{
		{"[score]\ndepht = 0.5\n", "config.toml: unknown key score.depht"},
		{"[sessions]\nlength = 25\n", "config.toml: unknown key sessions"},
		{"[score]\ndepth = 0.6\n", "config.toml: [score] weights sum to 1.10, want 1"},
		{"[score]\ndepth = 1.5\nblocks = -0.3\ncalm = -0.2\n", "config.toml: [score] depth = 1.5, want 0 to 1"},
		{"[score\n", "config.toml: "},
		{"[away]\nmin = 4\n", "config.toml: [away] min = 4, want at least 5"},
		{"[away]\nmin = 30\nmax = 30\n", "config.toml: [away] max = 30, want more than min (30) and at most 720"},
		{"[away]\nmax = 721\n", "config.toml: [away] max = 721"},
		{"[score]\nstretch = 0.1\n", "config.toml: [score] weights sum to 1.10, want 1"},
	}
	for _, tt := range tests {
		c, err := Load(write(t, tt.body))
		if err == nil || !strings.HasPrefix(err.Error(), tt.want) {
			t.Errorf("Load(%q) error = %v, want prefix %q", tt.body, err, tt.want)
		}
		if !reflect.DeepEqual(c, Default()) {
			t.Errorf("Load(%q) config = %+v, want Default()", tt.body, c)
		}
	}
}

func TestFocusDefaults(t *testing.T) {
	f := Default().Focus
	want := Focus{
		Presets:      []int{25, 50, 90},
		BreakAfterMS: 50 * 60_000, BreakRepeatMS: 10 * 60_000,
		DistractGraceMS: 60_000, DistractRepeatMS: 120_000,
		AutoMinMS: 25 * 60_000, RecapAt: 21 * 60, Notify: true,
		MeetingApps: []string{"us.zoom.xos", "com.apple.FaceTime", "com.microsoft.teams2",
			"com.microsoft.teams", "Cisco-Systems.Spark", "com.cisco.webexmeetingsapp"},
		MeetingHosts: []string{"meet.google.com", "teams.microsoft.com", "app.zoom.us", "whereby.com"},
	}
	if !reflect.DeepEqual(f, want) {
		t.Fatalf("DefaultFocus = %+v\nwant %+v", f, want)
	}
	f.Presets[0] = 1
	if Default().Focus.Presets[0] != 25 {
		t.Fatal("Default must return fresh slices")
	}
}

func TestFocusValid(t *testing.T) {
	c, err := Load(write(t, `[focus]
presets = [30, 45]
break_after = 40
break_repeat = 5
distract_grace = 30
distract_repeat = 90
auto_min = 20
recap = "08:30"
notify = false
meeting_apps = ["a.b"]
meeting_hosts = []
`))
	if err != nil {
		t.Fatal(err)
	}
	want := Focus{
		Presets: []int{30, 45}, BreakAfterMS: 40 * 60_000, BreakRepeatMS: 5 * 60_000,
		DistractGraceMS: 30_000, DistractRepeatMS: 90_000, AutoMinMS: 20 * 60_000,
		RecapAt: 8*60 + 30, Notify: false, MeetingApps: []string{"a.b"}, MeetingHosts: []string{},
	}
	if !reflect.DeepEqual(c.Focus, want) {
		t.Fatalf("focus = %+v\nwant %+v", c.Focus, want)
	}
	if c.Score != stats.DefaultWeights {
		t.Fatalf("score = %+v", c.Score)
	}
}

func TestFocusRecapOff(t *testing.T) {
	c, err := Load(write(t, "[focus]\nrecap = \"\"\n"))
	if err != nil || c.Focus.RecapAt != -1 {
		t.Fatalf("recap off: %+v, %v", c.Focus, err)
	}
}

func TestFocusErrors(t *testing.T) {
	tests := []struct{ body, want string }{
		{"[focus]\npresets = []\n", "config.toml: [focus] presets is empty, want at least one length"},
		{"[focus]\npresets = [25, 0]\n", "config.toml: [focus] presets has 0, want 1 to 240 minutes"},
		{"[focus]\npresets = [241]\n", "config.toml: [focus] presets has 241, want 1 to 240 minutes"},
		{"[focus]\nbreak_after = 0\n", "config.toml: [focus] break_after = 0, want 1 to 480"},
		{"[focus]\nbreak_repeat = 241\n", "config.toml: [focus] break_repeat = 241, want 1 to 240"},
		{"[focus]\ndistract_grace = 3601\n", "config.toml: [focus] distract_grace = 3601, want 1 to 3600"},
		{"[focus]\ndistract_repeat = -1\n", "config.toml: [focus] distract_repeat = -1, want 1 to 3600"},
		{"[focus]\nauto_min = 4\n", "config.toml: [focus] auto_min = 4, want 5 to 240"},
		{"[focus]\nrecap = \"25:00\"\n", `config.toml: [focus] recap = "25:00", want "HH:MM" or ""`},
		{"[focus]\nrecap = \"9:00\"\n", `config.toml: [focus] recap = "9:00", want "HH:MM" or ""`},
		{"[focus]\nx = 1\n", "config.toml: unknown key focus.x"},
	}
	for _, tt := range tests {
		c, err := Load(write(t, tt.body))
		if err == nil || err.Error() != tt.want {
			t.Errorf("Load(%q) error = %v, want %q", tt.body, err, tt.want)
		}
		if !reflect.DeepEqual(c, Default()) {
			t.Errorf("Load(%q) config = %+v, want Default()", tt.body, c)
		}
	}
}

func TestLLMDefaults(t *testing.T) {
	l := Default().LLM
	if !l.Enabled || l.Model != "claude-haiku-5-5" || l.MinMS != 120_000 ||
		len(l.DenyApps) != 13 || len(l.DenyHosts) != 2 || l.DenyTitles != nil {
		t.Fatalf("defaults = %+v", l)
	}
	for _, b := range []string{"com.apple.mail", "com.apple.MobileSMS", "com.1password.1password", "com.tinyspeck.slackmacgap"} {
		if !slices.Contains(l.DenyApps, b) {
			t.Errorf("default deny_apps lacks %s", b)
		}
	}
}

func TestLLMAdditions(t *testing.T) {
	c, err := Load(write(t, "[llm]\nenabled = false\nmodel = \"m\"\nmin_minutes = 5\n"+
		"deny_apps = [\"x.y\"]\ndeny_hosts = [\"intranet\"]\ndeny_titles = [\"salary\"]\n"))
	if err != nil {
		t.Fatal(err)
	}
	l, d := c.LLM, DefaultLLM()
	if l.Enabled || l.Model != "m" || l.MinMS != 300_000 {
		t.Fatalf("scalars = %+v", l)
	}
	if !reflect.DeepEqual(l.DenyApps, append(d.DenyApps, "x.y")) ||
		!reflect.DeepEqual(l.DenyHosts, append(d.DenyHosts, "intranet")) ||
		!reflect.DeepEqual(l.DenyTitles, []string{"salary"}) {
		t.Fatalf("lists = %+v", l)
	}
	if len(Default().LLM.DenyApps) != 13 {
		t.Fatal("additions leaked into the defaults")
	}
}

func TestLLMErrors(t *testing.T) {
	tests := []struct{ body, want string }{
		{"[llm]\nmodel = \"\"\n", "config.toml: [llm] model is empty"},
		{"[llm]\nmin_minutes = 0\n", "config.toml: [llm] min_minutes = 0, want 1 to 600"},
		{"[llm]\nmin_minutes = 601\n", "config.toml: [llm] min_minutes = 601, want 1 to 600"},
		{"[llm]\ndeny_hosts = [\"(\"]\n", `config.toml: [llm] deny_hosts has "("`},
		{"[llm]\ndeny_titles = [\"[\"]\n", `config.toml: [llm] deny_titles has "["`},
		{"[llm]\napi_key = \"x\"\n", "config.toml: unknown key llm.api_key"},
	}
	for _, tt := range tests {
		c, err := Load(write(t, tt.body))
		if err == nil || !strings.HasPrefix(err.Error(), tt.want) {
			t.Errorf("Load(%q) error = %v, want prefix %q", tt.body, err, tt.want)
		}
		if !reflect.DeepEqual(c, Default()) {
			t.Errorf("Load(%q) config = %+v, want Default()", tt.body, c)
		}
	}
}

func TestShell(t *testing.T) {
	c, err := Load(write(t, ""))
	if err != nil || !slices.Contains(c.Shell.Terminals, "com.github.wez.wezterm") || len(c.Shell.Terminals) != 7 {
		t.Fatalf("default terminals = %v, %v", c.Shell.Terminals, err)
	}
	c, err = Load(write(t, "[shell]\nterminals = [\"x.term\"]\n"))
	if err != nil || !reflect.DeepEqual(c.Shell.Terminals, []string{"x.term"}) {
		t.Fatalf("override = %v, %v", c.Shell.Terminals, err)
	}
	if _, err := Load(write(t, "[shell]\nnope = 1\n")); err == nil {
		t.Fatal("unknown [shell] key must fail")
	}
}
