package config

import (
	"slices"
	"strings"
	"testing"
)

func TestHideLists(t *testing.T) {
	c, err := Parse("config.toml", []byte(`[privacy]
hide_apps = ["com.example.diary"]
hide_sites = ["bank.example.com"]
`))
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(c.Privacy.HideApps, []string{"com.example.diary"}) || !slices.Equal(c.Privacy.HideSites, []string{"bank.example.com"}) {
		t.Fatalf("privacy = %+v", c.Privacy)
	}
	if !slices.Contains(c.LLM.DenyApps, "com.example.diary") {
		t.Errorf("deny apps miss the hidden app: %v", c.LLM.DenyApps)
	}
	if !slices.Contains(c.LLM.DenyHosts, `(^|\.)bank\.example\.com$`) {
		t.Errorf("deny hosts miss the hidden site: %v", c.LLM.DenyHosts)
	}
	if n := len(Default().LLM.DenyApps); len(c.LLM.DenyApps) != n+1 {
		t.Errorf("deny apps = %d, want the %d defaults plus one", len(c.LLM.DenyApps), n)
	}
}

func TestHideListsRejected(t *testing.T) {
	for _, src := range []string{
		`hide_apps = [""]`,
		`hide_apps = ["com.x app"]`,
		`hide_sites = [""]`,
		`hide_sites = ["https://bank.com"]`,
		`hide_sites = ["bank.com/login"]`,
		`hide_sites = ["Bank.com"]`,
		`hide_sites = ["bank com"]`,
	} {
		if _, err := Parse("config.toml", []byte("[privacy]\n"+src+"\n")); err == nil || !strings.Contains(err.Error(), "[privacy]") {
			t.Errorf("%s: err = %v, want a [privacy] error", src, err)
		}
	}
}

func TestHides(t *testing.T) {
	p := Privacy{HideApps: []string{"com.example.diary"}, HideSites: []string{"bank.com"}}
	url := func(s string) *string { return &s }
	for _, tc := range []struct {
		bundle string
		url    *string
		want   bool
	}{
		{"com.example.diary", nil, true},
		{"com.apple.Safari", nil, false},
		{"com.apple.Safari", url("bank.com/login"), true},
		{"com.apple.Safari", url("www.bank.com"), true},
		{"com.apple.Safari", url("notbank.com/x"), false},
		{"com.apple.Safari", url("bank.com.evil.io/"), false},
		{"com.apple.Safari", url(""), false},
		{"com.apple.Safari", url("bank.com:8443/x"), true},
	} {
		if got := p.Hides(tc.bundle, tc.url); got != tc.want {
			u := "<nil>"
			if tc.url != nil {
				u = *tc.url
			}
			t.Errorf("Hides(%s, %s) = %v, want %v", tc.bundle, u, got, tc.want)
		}
	}
	if (Privacy{}).Hides("x", nil) {
		t.Error("an empty list hides nothing")
	}
}

func TestHost(t *testing.T) {
	url := func(s string) *string { return &s }
	for in, want := range map[string]string{"www.Bank.com:443/x": "bank.com", "news.com": "news.com", "": "", "a b/": ""} {
		if got := Host(url(in)); got != want {
			t.Errorf("Host(%q) = %q, want %q", in, got, want)
		}
	}
	if Host(nil) != "" {
		t.Error("Host(nil) is not empty")
	}
}
