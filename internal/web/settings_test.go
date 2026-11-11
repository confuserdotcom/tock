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

// Hours, presets and the invoice details read back as they were written,
// and a bad value is refused with the reason and leaves the file alone.
func TestSettingsValues(t *testing.T) {
	f := newFixture(t)
	post := func(path string, body any) (int, string) {
		code, _, b := f.do("POST", path, testToken, body)
		return code, string(b)
	}
	if code, b := post("/api/settings/hours", map[string]any{"set": true, "start": "08:30", "end": "17:00", "days": []string{"mon", "tue", "wed"}}); code != 200 {
		t.Fatalf("hours: %d %s", code, b)
	}
	if code, b := post("/api/settings/presets", map[string]any{"presets": []int{45, 90}}); code != 200 {
		t.Fatalf("presets: %d %s", code, b)
	}
	inv := map[string]any{
		"name": "Ada Lovelace", "address": []string{"1 Analytical Row", "London"}, "currency": "EUR", "note": "Due in 30 days.",
		"taxId": "GB123", "round": "15m", "roundMode": "up", "minorUnits": 2, "taxLabel": "VAT", "taxRate": "20", "taxNote": "",
		"clients": []map[string]any{{"name": "Babbage & Co", "address": []string{"2 Engine St"}, "taxId": ""}},
	}
	if code, b := post("/api/settings/invoice", inv); code != 200 {
		t.Fatalf("invoice: %d %s", code, b)
	}
	var s settingsJSON
	f.get("/api/settings", &s)
	if !s.Hours.Set || s.Hours.Start != "08:30" || s.Hours.End != "17:00" || strings.Join(s.Hours.Days, ",") != "mon,tue,wed" {
		t.Fatalf("hours: %+v", s.Hours)
	}
	if len(s.Presets) != 2 || s.Presets[0] != 45 {
		t.Fatalf("presets: %v", s.Presets)
	}
	i := s.Invoice
	if i.Name != "Ada Lovelace" || len(i.Address) != 2 || i.Round != "15m" || i.RoundMode != "up" || i.TaxRate != "20" || len(i.Clients) != 1 || i.Clients[0].Name != "Babbage & Co" {
		t.Fatalf("invoice: %+v", i)
	}

	if code, b := post("/api/settings/hours", map[string]any{"set": true, "start": "18:00", "end": "09:00", "days": []string{"mon"}}); code != 400 || !strings.Contains(b, "not before end") {
		t.Fatalf("bad hours: %d %s", code, b)
	}
	if code, b := post("/api/settings/presets", map[string]any{"presets": []int{}}); code != 400 {
		t.Fatalf("no presets: %d %s", code, b)
	}
	if code, _ := post("/api/settings/hours", map[string]any{"set": false}); code != 200 {
		t.Fatal("hours off")
	}
	s = settingsJSON{}
	f.get("/api/settings", &s)
	if s.Hours.Set {
		t.Fatalf("hours still set: %+v", s.Hours)
	}
}
