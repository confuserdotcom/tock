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

func TestLoadCalendar(t *testing.T) {
	if c := Default().Calendar; c.Enabled || c.Calendars != nil {
		t.Fatalf("default calendar = %+v", c)
	}
	c, err := Load(write(t, "[calendar]\nenabled = true\ncalendars = [\"Work\", \"Uni\"]\n"))
	if err != nil || !c.Calendar.Enabled || !reflect.DeepEqual(c.Calendar.Calendars, []string{"Work", "Uni"}) {
		t.Fatalf("calendar = %+v, %v", c.Calendar, err)
	}
	if _, err := Load(write(t, "[calendar]\ncalendars = [\" \"]\n")); err == nil ||
		err.Error() != "config.toml: [calendar] calendars has an empty name" {
		t.Fatalf("empty name err = %v", err)
	}
	if _, err := Load(write(t, "[calendar]\nsync = 5\n")); err == nil {
		t.Fatal("unknown [calendar] key must fail")
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
		BlockGraceMS: 10_000,
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
		RecapAt: 8*60 + 30, Notify: false, MeetingApps: []string{"a.b"}, MeetingHosts: []string{}, BlockGraceMS: 10_000,
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
	if err != nil || !slices.Contains(c.Shell.Terminals, "com.github.wez.wezterm") || len(c.Shell.Terminals) != 11 || !slices.Contains(c.Shell.Terminals, "kitty") {
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

func TestSync(t *testing.T) {
	c, err := Load(write(t, ""))
	if err != nil || c.Sync.Dir != "" || c.Sync.MachineName() == "" {
		t.Fatalf("default sync = %+v, %v", c.Sync, err)
	}
	if !machineRE.MatchString(c.Sync.MachineName()) {
		t.Fatalf("host name gives an invalid machine %q", c.Sync.MachineName())
	}
	home, _ := os.UserHomeDir()
	c, err = Load(write(t, "[sync]\ndir = \"~/Sync/tock\"\nmachine = \"ryuk\"\n"))
	if err != nil || c.Sync.Dir != filepath.Join(home, "Sync", "tock") || c.Sync.MachineName() != "ryuk" {
		t.Fatalf("sync = %+v, %v", c.Sync, err)
	}
	for _, bad := range []string{"[sync]\ndir = \"rel/dir\"\n", "[sync]\nmachine = \"Ryuk\"\n", "[sync]\nmachine = \"a/b\"\n"} {
		if _, err := Load(write(t, bad)); err == nil || !strings.Contains(err.Error(), "[sync]") {
			t.Errorf("%q: %v", bad, err)
		}
	}
}

func TestLoadFocusGuard(t *testing.T) {
	d := Default().Focus
	if d.Block || d.BlockGraceMS != 10_000 || d.FocusOn != "" || d.FocusOff != "" {
		t.Fatalf("default guard = %+v", d)
	}
	c, err := Load(write(t, "[focus]\nblock = true\nblock_apps = [\"com.valvesoftware.steam\"]\n"+
		"block_hosts = [\"YouTube.com\"]\nblock_grace = 30\nfocus_on = \"Work\"\nfocus_off = \"Off\"\n"))
	if err != nil {
		t.Fatal(err)
	}
	f := c.Focus
	if !f.Block || f.BlockGraceMS != 30_000 || f.FocusOn != "Work" || f.FocusOff != "Off" ||
		!reflect.DeepEqual(f.BlockApps, []string{"com.valvesoftware.steam"}) ||
		!reflect.DeepEqual(f.BlockHosts, []string{"youtube.com"}) {
		t.Fatalf("guard = %+v", f)
	}
	for _, bad := range []string{"[focus]\nblock_grace = 0\n", "[focus]\nblock_grace = 700\n",
		"[focus]\nblock_hosts = [\"\"]\n", "[focus]\nblock_hosts = [\"a b\"]\n"} {
		if _, err := Load(write(t, bad)); err == nil {
			t.Errorf("Load(%q) = nil error", bad)
		}
	}
}

func TestLoadSlack(t *testing.T) {
	d := Default().Slack
	if d.Enabled || d.Status != "In a focus session" || d.Emoji != ":dart:" || !d.DND {
		t.Fatalf("default slack = %+v", d)
	}
	c, err := Load(write(t, "[slack]\nenabled = true\nstatus = \"Deep work\"\nemoji = \":brain:\"\ndnd = false\n"))
	if err != nil || c.Slack != (Slack{Enabled: true, Status: "Deep work", Emoji: ":brain:"}) {
		t.Fatalf("slack = %+v, %v", c.Slack, err)
	}
	for _, bad := range []string{"[slack]\nstatus = \"\"\n", "[slack]\nemoji = \"dart\"\n",
		"[slack]\nstatus = \"" + strings.Repeat("x", 101) + "\"\n"} {
		if _, err := Load(write(t, bad)); err == nil {
			t.Errorf("Load(%q) = nil error", bad)
		}
	}
}

func TestHours(t *testing.T) {
	c, err := Load(write(t, "[hours]\nstart = \"09:00\"\nend = \"17:30\"\n"))
	if err != nil {
		t.Fatal(err)
	}
	want := stats.Hours{Set: true, StartMin: 9 * 60, EndMin: 17*60 + 30,
		Days: [7]bool{false, true, true, true, true, true, false}}
	if c.Hours != want {
		t.Fatalf("hours = %+v, want %+v", c.Hours, want)
	}
	if c.Focus.RecapAt != 17*60+30 {
		t.Errorf("recap at %d, want the end of hours", c.Focus.RecapAt)
	}
	c, err = Load(write(t, "[hours]\ndays = [\"Sat\", \"sun\"]\nstart = \"10:00\"\nend = \"14:00\"\n[focus]\nrecap = \"20:00\"\n"))
	if err != nil {
		t.Fatal(err)
	}
	if c.Hours.Days != [7]bool{true, false, false, false, false, false, true} || c.Focus.RecapAt != 20*60 {
		t.Errorf("days %v recap %d", c.Hours.Days, c.Focus.RecapAt)
	}
	if c, _ := Load(write(t, "")); c.Hours.Set {
		t.Error("hours set without [hours]")
	}
}

func TestHoursErrors(t *testing.T) {
	for body, want := range map[string]string{
		"[hours]\nstart = \"09:00\"\n":                                             "needs both start and end",
		"[hours]\nstart = \"9am\"\nend = \"17:00\"\n":                              `start = "9am"`,
		"[hours]\nstart = \"\"\nend = \"17:00\"\n":                                 `start = ""`,
		"[hours]\nstart = \"18:00\"\nend = \"09:00\"\n":                            "is not before end",
		"[hours]\ndays = []\nstart = \"09:00\"\nend = \"17:00\"\n":                 "days is empty",
		"[hours]\ndays = [\"mo\"]\nstart = \"09:00\"\nend = \"17:00\"\n":           `days has "mo"`,
		"[hours]\ndays = [\"mon\", \"MON\"]\nstart = \"09:00\"\nend = \"17:00\"\n": "twice",
	} {
		c, err := Load(write(t, body))
		if err == nil || !strings.Contains(err.Error(), want) || c.Hours.Set {
			t.Errorf("%q: err %v, want %q", body, err, want)
		}
	}
}

func TestInvoice(t *testing.T) {
	c, err := Load(write(t, "[invoice]\nname = \" Ada Lovelace \"\naddress = [\"1 Analytical Way\", \"London\"]\ncurrency = \"GBP\"\nnote = \"Due in 30 days.\"\n"))
	if err != nil {
		t.Fatal(err)
	}
	want := Invoice{Name: "Ada Lovelace", Address: []string{"1 Analytical Way", "London"}, Currency: "GBP", Note: "Due in 30 days."}
	if !reflect.DeepEqual(c.Invoice, want) {
		t.Fatalf("invoice %+v", c.Invoice)
	}
	if _, err := Load(write(t, "[invoice]\ncurrency = \"EUROS-AND-CENTS\"\n")); err == nil || !strings.Contains(err.Error(), "currency") {
		t.Errorf("long currency: %v", err)
	}
}

func TestPrivacy(t *testing.T) {
	if c, err := Load(write(t, "[privacy]\nkeep_titles = 30\n")); err != nil || c.Privacy.KeepTitles != 30 {
		t.Fatalf("keep_titles: %+v %v", c.Privacy, err)
	}
	for _, v := range []string{"0", "-1", "3651"} {
		if _, err := Load(write(t, "[privacy]\nkeep_titles = "+v+"\n")); err == nil || !strings.Contains(err.Error(), "keep_titles") {
			t.Errorf("keep_titles = %s: %v", v, err)
		}
	}
}
