package config

import (
	"slices"
	"strings"
	"testing"
)

func TestLoadOCR(t *testing.T) {
	c, err := Load(write(t, ""))
	if err != nil {
		t.Fatal(err)
	}
	d := DefaultOCR()
	if c.OCR.Enabled || c.OCR.IntervalMS != 60_000 || c.OCR.MinDwellMS != 20_000 {
		t.Fatalf("defaults = %+v", c.OCR)
	}
	for _, b := range []string{"com.apple.Terminal", "com.mitchellh.ghostty", "com.apple.Preview", "net.ankiweb.launcher", "com.apple.Safari"} {
		if !slices.Contains(d.Apps, b) {
			t.Errorf("default apps miss %s", b)
		}
	}
	d.Apps[0] = "x"
	if DefaultOCR().Apps[0] == "x" {
		t.Fatal("DefaultOCR shares its slice")
	}

	c, err = Load(write(t, `[ocr]
enabled = true
interval = 15
min_dwell = 300
apps = ["a.b"]
deny_apps = ["c.d"]
deny_hosts = ["^x\\.org$"]
deny_titles = ["secret"]
`))
	if err != nil {
		t.Fatal(err)
	}
	o := c.OCR
	if !o.Enabled || o.IntervalMS != 15_000 || o.MinDwellMS != 300_000 || !slices.Equal(o.Apps, []string{"a.b"}) ||
		!slices.Equal(o.DenyApps, []string{"c.d"}) || !slices.Equal(o.DenyHosts, []string{`^x\.org$`}) || !slices.Equal(o.DenyTitles, []string{"secret"}) {
		t.Fatalf("set = %+v", o)
	}

	for _, tc := range []struct{ in, want string }{
		{"interval = 14", "[ocr] interval = 14, want 15 to 3600 seconds"},
		{"interval = 3601", "[ocr] interval = 3601, want 15 to 3600 seconds"},
		{"min_dwell = 4", "[ocr] min_dwell = 4, want 5 to 300 seconds"},
		{"min_dwell = 301", "[ocr] min_dwell = 301, want 5 to 300 seconds"},
		{`deny_titles = ["("]`, `[ocr] deny_titles has "("`},
		{`deny_hosts = ["("]`, `[ocr] deny_hosts has "("`},
		{`apps = [""]`, "[ocr] apps has an empty bundle id"},
		{"nope = 1", "unknown key ocr.nope"},
	} {
		_, err := Load(write(t, "[ocr]\n"+tc.in+"\n"))
		if err == nil || !strings.Contains(err.Error(), "config.toml: "+tc.want) {
			t.Errorf("%s: err = %v, want %q", tc.in, err, tc.want)
		}
	}
}
