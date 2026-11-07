package config

import (
	"strings"
	"testing"
)

func TestSetBool(t *testing.T) {
	for _, tc := range []struct {
		name, src, table, key string
		v                     bool
		want                  string
	}{
		{"replace keeps comment", "[ocr]\nenabled = false   # read text\n", "ocr", "enabled", true,
			"[ocr]\nenabled = true   # read text\n"},
		{"replace indented", "[focus]\n  notify = true\n", "focus", "notify", false,
			"[focus]\n  notify = false\n"},
		{"same value is a no-op", "[ocr]\nenabled = true\n", "ocr", "enabled", true,
			"[ocr]\nenabled = true\n"},
		{"only inside its table", "[plan]\nnotify = true\n\n[focus]\nnotify = true\n", "focus", "notify", false,
			"[plan]\nnotify = true\n\n[focus]\nnotify = false\n"},
		{"add after last key", "[focus]\n# focus\nnotify = true\n\n[plan]\nfile = \"x\"\n", "focus", "block", true,
			"[focus]\n# focus\nnotify = true\nblock = true\n\n[plan]\nfile = \"x\"\n"},
		{"add to empty table", "[ocr]\n\n[plan]\nnotify = true\n", "ocr", "enabled", true,
			"[ocr]\nenabled = true\n\n[plan]\nnotify = true\n"},
		{"header with comment", "[ocr] # screen\nenabled = false\n", "ocr", "enabled", true,
			"[ocr] # screen\nenabled = true\n"},
		{"append table", "# tock\n[plan]\nnotify = true\n", "ocr", "enabled", true,
			"# tock\n[plan]\nnotify = true\n\n[ocr]\nenabled = true\n"},
		{"append without final newline", "[plan]\nnotify = true", "ocr", "enabled", false,
			"[plan]\nnotify = true\n\n[ocr]\nenabled = false\n"},
		{"empty file", "", "ocr", "enabled", true, "[ocr]\nenabled = true\n"},
		{"commented key is not the key", "[ocr]\n# enabled = true\n", "ocr", "enabled", true,
			"[ocr]\nenabled = true\n# enabled = true\n"},
		{"prefix key is not the key", "[focus]\nnotify_x = 1\n", "focus", "notify", true,
			"[focus]\nnotify_x = 1\nnotify = true\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := SetBool([]byte(tc.src), tc.table, tc.key, tc.v)
			if err != nil || string(got) != tc.want {
				t.Fatalf("SetBool = %q, %v\nwant %q", got, err, tc.want)
			}
		})
	}
}

func TestSetBoolRefuses(t *testing.T) {
	for name, src := range map[string]string{
		"inline table": "ocr = { enabled = false }\n",
		"dotted key":   "ocr.enabled = false\n",
		"broken file":  "[ocr\nenabled = \n",
	} {
		t.Run(name, func(t *testing.T) {
			if got, err := SetBool([]byte(src), "ocr", "enabled", true); err == nil {
				t.Fatalf("SetBool = %q, want an error", got)
			}
		})
	}
}

// Every feature's key, set both ways in a real-looking file, reads back
// through Parse.
func TestFeaturesRoundTrip(t *testing.T) {
	src := "# tock\n[calendar]\nenabled = true   # asks once\n\n[routine]\nmidday = \"off\"\n"
	seen := map[string]bool{}
	for _, f := range Features {
		if seen[f.ID] || f.ID == "" || f.Name == "" || f.Group == "" {
			t.Fatalf("feature %+v: empty or duplicate id/name/group", f)
		}
		seen[f.ID] = true
		for _, v := range []bool{true, false} {
			b, err := SetBool([]byte(src), f.Table, f.Key, v)
			if err != nil {
				t.Fatalf("%s: %v", f.ID, err)
			}
			c, err := Parse(File, b)
			if err != nil {
				t.Fatalf("%s=%v: %v\n%s", f.ID, v, err, b)
			}
			if f.On(c) != v {
				t.Fatalf("%s=%v reads back %v\n%s", f.ID, v, f.On(c), b)
			}
			if !strings.Contains(string(b), "midday = \"off\"") || !strings.Contains(string(b), "# asks once") {
				t.Fatalf("%s: lost other lines\n%s", f.ID, b)
			}
		}
	}
	if FeatureByID("ocr") == nil || FeatureByID("nope") != nil {
		t.Fatal("FeatureByID")
	}
}

// The defaults the list shows come from Default().
func TestFeaturesDefaults(t *testing.T) {
	d := Default()
	for id, want := range map[string]bool{"ocr": false, "break_screen": false, "claude": false, "notify": true} {
		if got := FeatureByID(id).On(d); got != want {
			t.Errorf("%s default = %v, want %v", id, got, want)
		}
	}
}

func TestSetList(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{"no table", "[focus]\nnotify = true\n",
			"[focus]\nnotify = true\n\n[privacy]\nhide_apps = [\"a\", \"b\"]\n"},
		{"no key", "[privacy]\nkeep_titles = 30 # a month\n\n[focus]\n",
			"[privacy]\nkeep_titles = 30 # a month\nhide_apps = [\"a\", \"b\"]\n\n[focus]\n"},
		{"one line", "[privacy]\nhide_apps = [\"x\", \"y]\"] # mine\nkeep_titles = 30\n",
			"[privacy]\nhide_apps = [\"a\", \"b\"] # mine\nkeep_titles = 30\n"},
		{"several lines", "[privacy]\n  hide_apps = [\n    \"x\", # one\n    \"y]\",\n  ]\nkeep_titles = 30\n",
			"[privacy]\n  hide_apps = [\"a\", \"b\"]\nkeep_titles = 30\n"},
	} {
		got, err := SetList([]byte(tc.src), "privacy", "hide_apps", []string{"a", "b"})
		if err != nil {
			t.Errorf("%s: %v", tc.name, err)
			continue
		}
		if string(got) != tc.want {
			t.Errorf("%s:\ngot  %q\nwant %q", tc.name, got, tc.want)
		}
	}
	got, err := SetList([]byte("[privacy]\nhide_apps = [\"a\"]\n"), "privacy", "hide_apps", nil)
	if err != nil || string(got) != "[privacy]\nhide_apps = []\n" {
		t.Errorf("empty list: %q, %v", got, err)
	}
}
