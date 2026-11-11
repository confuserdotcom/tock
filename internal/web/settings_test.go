package web

import (
	"strings"
	"testing"
)

// The settings page lists every feature with its file value and whether it
// waits for its group, and writes changes as tock choice does.
func TestSettingsAPI(t *testing.T) {
	f := newFixture(t)
	var s settingsJSON
	f.get("/api/settings", &s)
	byID := map[string]featureJSON{}
	for _, x := range s.Features {
		byID[x.ID] = x
	}
	if len(s.Features) < 10 || !byID["focus"].IsGroup || byID["wake_screen"].Needs != "focus" {
		t.Fatalf("features: %+v", s.Features)
	}
	if !byID["focus"].On || byID["claude"].On {
		t.Fatalf("defaults: focus %+v claude %+v", byID["focus"], byID["claude"])
	}

	if code, _, b := f.do("POST", "/api/settings", testToken, map[string]any{"features": map[string]bool{"focus": false, "hotkey": true}}); code != 200 {
		t.Fatalf("save: %d %s", code, b)
	}
	s = settingsJSON{}
	f.get("/api/settings", &s)
	for _, x := range s.Features {
		byID[x.ID] = x
	}
	if byID["focus"].On || !byID["hotkey"].On || !byID["hotkey"].Waits || byID["hotkey"].Live {
		t.Fatalf("after focus off: focus %+v hotkey %+v", byID["focus"], byID["hotkey"])
	}
	var st statusJSON
	f.get("/api/status", &st)
	if st.Focus {
		t.Fatal("status still says focus on")
	}

	if code, _, b := f.do("POST", "/api/settings", testToken, map[string]any{"features": map[string]bool{"nope": true}}); code != 400 || !strings.Contains(string(b), "nope") {
		t.Fatalf("unknown feature: %d %s", code, b)
	}

	if code, _, b := f.do("POST", "/api/settings/hide", testToken, map[string]any{"kind": "site", "value": "bank.example", "hide": true}); code != 200 {
		t.Fatalf("hide: %d %s", code, b)
	}
	s = settingsJSON{}
	f.get("/api/settings", &s)
	if len(s.HideSites) != 1 || s.HideSites[0] != "bank.example" {
		t.Fatalf("hide sites: %v", s.HideSites)
	}
	if code, _, _ := f.do("POST", "/api/settings/hide", testToken, map[string]any{"kind": "site", "value": "bank.example", "hide": false}); code != 200 {
		t.Fatal("unhide")
	}
	if code, _, _ := f.do("POST", "/api/settings/hide", testToken, map[string]any{"kind": "file", "value": "x", "hide": true}); code != 400 {
		t.Fatal("bad kind accepted")
	}
}
