package config

import (
	"os"
	"reflect"
	"testing"
)

// AC1: a group off turns its members off in the effective config; the file
// values stay in File.
func TestGroupsEffective(t *testing.T) {
	c, err := Parse(File, []byte("[focus]\nenabled = false\nwake_screen = true\nblock = true\n\n[slack]\nenabled = true\n\n[plan]\nauto_start = true\n"))
	if err != nil {
		t.Fatal(err)
	}
	if c.Focus.Enabled || c.Focus.WakeScreen || c.Focus.Block || c.Focus.Hotkey || c.Recommend.Enabled || c.Slack.Enabled || c.Plan.AutoStart {
		t.Fatalf("focus off left members on: %+v %+v %+v", c.Focus, c.Recommend, c.Slack)
	}
	f := c.Raw()
	if !f.Focus.WakeScreen || !f.Focus.Block || !f.Slack.Enabled || !f.Plan.AutoStart || f.Focus.Enabled {
		t.Fatalf("file values lost: %+v", f.Focus)
	}
	// Notices and the routine are not Focus members.
	if !c.Focus.Notify || !c.Routine.Start || c.Focus.RecapAt < 0 {
		t.Fatalf("focus off turned off more than its members: %+v %+v", c.Focus, c.Routine)
	}

	c, err = Parse(File, []byte("[routine]\nenabled = false\n\n[plan]\nnotify = true\n"))
	if err != nil {
		t.Fatal(err)
	}
	if c.Routine.Start || c.Routine.Midday || c.Routine.End || c.Routine.Overwork || c.Plan.Notify || c.Focus.RecapAt != -1 {
		t.Fatalf("routine off: %+v %+v", c.Routine, c.Plan)
	}
	if !c.Focus.Enabled || !c.Raw().Plan.Notify || !c.Raw().Routine.Start {
		t.Fatalf("routine off: file %+v", c.Raw().Routine)
	}

	c, err = Parse(File, []byte("[billing]\nenabled = false\n"))
	if err != nil || c.Billing.Enabled || c.Raw().Billing.Enabled {
		t.Fatalf("billing off: %+v %v", c.Billing, err)
	}
}

// AC11 / G6 at the config level: all groups on is the old effective config.
func TestGroupsDefaultOn(t *testing.T) {
	d := Default()
	if !d.Focus.Enabled || !d.Routine.Enabled || !d.Billing.Enabled {
		t.Fatalf("groups default off: %+v", d)
	}
	c, err := Parse(File, nil)
	if err != nil || !reflect.DeepEqual(c, d) {
		t.Fatalf("empty file is not Default: %v", err)
	}
	if !reflect.DeepEqual(d.Raw(), d.withoutFile()) {
		t.Fatal("Default's file values differ from its effective values")
	}
}

// G3: every member names a group feature that exists, and groups come
// before their members.
func TestFeatureGroups(t *testing.T) {
	seen := map[string]bool{}
	for _, f := range Features {
		if f.Needs != "" {
			g := FeatureByID(f.Needs)
			if g == nil || !g.IsGroup || !seen[f.Needs] {
				t.Fatalf("%s needs %q: not a group listed before it", f.ID, f.Needs)
			}
		}
		seen[f.ID] = true
	}
	for _, id := range []string{"focus", "routine", "billing"} {
		if f := FeatureByID(id); f == nil || !f.IsGroup {
			t.Fatalf("no group %s", id)
		}
	}
	for id, need := range map[string]string{"recommend": "focus", "wake_screen": "focus", "slack": "focus", "recap": "routine", "plan": "routine"} {
		if f := FeatureByID(id); f.Needs != need {
			t.Fatalf("%s needs %q, want %q", id, f.Needs, need)
		}
	}
	c, _ := Parse(File, []byte("[focus]\nenabled = false\nwake_screen = true\n"))
	if s := FeatureByID("wake_screen").State(c); s != "on (waits for Focus)" {
		t.Fatalf("state %q", s)
	}
	if s := FeatureByID("hotkey").State(Default()); s != "on" {
		t.Fatalf("state %q", s)
	}
}

// AC2: a group off then on gives back the same bytes.
func TestSaveGroupRoundTrip(t *testing.T) {
	for _, src := range []string{
		"",
		"# mine\n[score]\ndepth = 0.4\nblocks = 0.3\ncalm = 0.2\nstretch = 0.1\n",
		"[focus]\nnotify = false # quiet\n\n[slack]\nenabled = true\n",
		"[routine]\nstart = false\n",
	} {
		path := t.TempDir() + "/" + File
		if src != "" {
			writeFile(t, path, src)
		}
		for _, id := range []string{"focus", "routine", "billing"} {
			if err := SaveFeatures(path, map[string]bool{id: false}); err != nil {
				t.Fatal(err)
			}
			c, err := Load(path)
			if err != nil || FeatureByID(id).On(c) {
				t.Fatalf("%s off: %v", id, err)
			}
			if err := SaveFeatures(path, map[string]bool{id: true}); err != nil {
				t.Fatal(err)
			}
			if got := readFile(t, path); got != src {
				t.Fatalf("%s off/on changed the file:\n%q\nwant\n%q", id, got, src)
			}
		}
	}
}

func TestUnsetKey(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{"absent", "[focus]\nnotify = true\n", "[focus]\nnotify = true\n"},
		{"one of two", "[focus]\nenabled = false # x\nnotify = true\n", "[focus]\nnotify = true\n"},
		{"last table", "[a]\nx = 1\n\n[focus]\nenabled = false\n", "[a]\nx = 1\n"},
		{"only table", "[focus]\nenabled = false\n", ""},
		{"keeps a commented table", "[focus]\n# mine\nenabled = false\n", "[focus]\n# mine\n"},
	} {
		got, err := UnsetKey([]byte(tc.src), "focus", "enabled")
		if err != nil || string(got) != tc.want {
			t.Errorf("%s: %q %v, want %q", tc.name, got, err, tc.want)
		}
	}
	if _, err := UnsetKey([]byte("focus.enabled = false\n"), "focus", "enabled"); err == nil {
		t.Error("dotted key: no error")
	}
}

func writeFile(t *testing.T, path, s string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(s), 0o600); err != nil {
		t.Fatal(err)
	}
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil && !os.IsNotExist(err) {
		t.Fatal(err)
	}
	return string(b)
}

// AC12: a config.toml from before the groups loads with the same effective
// values: without a group key, nothing is turned off.
func TestGroupsOldConfig(t *testing.T) {
	old := `[focus]
presets = [50, 25]
notify = true
break_screen = true
wake_screen = true
block = true
block_hosts = ["youtube.com"]
urge_surf = true

[routine]
start = true
midday = "13:30"
overwork = true

[hours]
start = "09:00"
end = "18:00"

[recommend]
enabled = true

[plan]
notify = true
auto_start = true

[slack]
enabled = true

[invoice]
name = "Ada"
currency = "EUR"
`
	c, err := Parse(File, []byte(old))
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(c.withoutFile(), c.Raw()) {
		t.Fatalf("an old config changed under the groups:\n%+v\n%+v", c.withoutFile(), c.Raw())
	}
}
