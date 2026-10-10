package tui

import (
	"testing"

	tea "charm.land/bubbletea/v2"

	"tock/internal/apptest"
	"tock/internal/store"
)

func TestBannerPriority(t *testing.T) {
	e := apptest.New(t, apptest.Rules)
	e.Seed(t, apptest.DaySpans()...)
	e.Heartbeat(t, e.Clock.UnixMilli())
	m := newModel(t, e)
	mustContain(t, m, "● trackd\n")
	if b := m.banner(); b != "" {
		t.Fatalf("healthy: banner %q", b)
	}
	e.State(t, store.AutomationKey("company.thebrowser.Browser"), "denied")
	m = step(t, m, tickMsg{})
	mustContain(t, m, "no URLs from Arc: allow trackd in System Settings › Privacy & Security › Automation")
	e.State(t, store.KeyAXTrusted, "0")
	m = step(t, m, tickMsg{})
	mustContain(t, m, "no window titles: allow trackd")
	e.WriteRules(t, apptest.Rules+"\n[[rule]]\napp = '('\ncategory = \"Code\"\n")
	m = step(t, m, tickMsg{})
	mustContain(t, m, "rule error, using the last good rules: rules.toml: rule #4", "Code            2h14")
	e.Clock = e.Clock.Add(31e9)
	m = step(t, m, tickMsg{})
	mustContain(t, m, "trackd is not running — showing history", "trackd down")
}

func TestTabsHelpAndQuit(t *testing.T) {
	m, _ := seeded(t)
	m = press(t, m, "tab")
	mustContain(t, m, "Week of")
	m = press(t, m, "tab")
	mustContain(t, m, "Inbox ·")
	m = press(t, m, "tab")
	mustContain(t, m, "no session · s starts one")
	m = press(t, m, "tab")
	mustContain(t, m, "rules.toml · 3 rules")
	m = press(t, m, "tab")
	mustContain(t, m, "Today  score")
	m = press(t, m, "shift+tab")
	mustContain(t, m, "rules.toml · 3 rules")
	m = press(t, m, "shift+tab")
	mustContain(t, m, "no session · s starts one")
	m = press(t, m, "shift+tab")
	mustContain(t, m, "Inbox ·")
	m = press(t, m, "?")
	mustContain(t, m, "tock — keys")
	m = press(t, m, "esc")
	mustNotContain(t, m, "tock — keys")
	if _, cmd := m.Update(key("q")); cmd == nil {
		t.Fatal("q must quit")
	} else if _, ok := cmd().(tea.QuitMsg); !ok {
		t.Fatal("q must return tea.Quit")
	}
}

func TestFlashExpires(t *testing.T) {
	m, e := seeded(t)
	m = press(t, m, "k", "k", "p") // idle row
	mustContain(t, m, "idle time has no label")
	e.Clock = e.Clock.Add(flashFor)
	m = step(t, m, tickMsg{})
	mustNotContain(t, m, "idle time has no label")
}
