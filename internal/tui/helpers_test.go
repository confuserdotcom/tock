package tui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"tock/internal/app"
	"tock/internal/apptest"
	"tock/internal/suggest"
)

// newModel opens a seeded app and returns a 100×30 model with its first
// newModel opens a seeded app and returns a 100×30 model with its first
// load done. The refresh timer is off; tests send tickMsg themselves.
func newModel(t *testing.T, e *apptest.Env) Model {
	t.Helper()
	return newModelWith(t, e, nil)
}

// newModelWith is newModel with a suggestion client (nil: none).
func newModelWith(t *testing.T, e *apptest.Env, c suggest.Client) Model {
	t.Helper()
	a, err := app.Open(app.Paths{DB: e.DB, Rules: e.Rules, Learned: e.Learned, Config: e.Config}, e.NowFunc())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { a.Close() })
	m := New(a, e.NowFunc())
	if c != nil {
		m = m.WithSuggester(c)
	}
	m.tick = nil
	m.clockTick = nil
	m = step(t, m, tea.WindowSizeMsg{Width: 100, Height: 30})
	return runCmd(t, m, m.Init())
}

// seeded is newModel over apptest.DaySpans.
func seeded(t *testing.T) (Model, *apptest.Env) {
	t.Helper()
	e := apptest.New(t, apptest.Rules)
	e.Seed(t, apptest.DaySpans()...)
	return newModel(t, e), e
}

// step delivers msg and runs every command it produces, synchronously.
func step(t *testing.T, m Model, msg tea.Msg) Model {
	t.Helper()
	next, cmd := m.Update(msg)
	return runCmd(t, next.(Model), cmd)
}

func runCmd(t *testing.T, m Model, cmd tea.Cmd) Model {
	t.Helper()
	if cmd == nil {
		return m
	}
	switch msg := cmd().(type) {
	case nil, tea.QuitMsg:
	case tea.BatchMsg:
		for _, c := range msg {
			m = runCmd(t, m, c)
		}
	default:
		m = step(t, m, msg)
	}
	return m
}

// press sends keys: named keys ("enter", "esc", "backspace", "tab",
// "shift+tab", "up", "down", "ctrl+c", "ctrl+d", "ctrl+u") or single
// characters.
func press(t *testing.T, m Model, keys ...string) Model {
	t.Helper()
	for _, k := range keys {
		m = step(t, m, key(k))
	}
	return m
}

// typeText sends each rune of s as a key.
func typeText(t *testing.T, m Model, s string) Model {
	t.Helper()
	for _, r := range s {
		m = step(t, m, key(string(r)))
	}
	return m
}

func key(k string) tea.KeyPressMsg {
	named := map[string]tea.Key{
		"enter":     {Code: tea.KeyEnter},
		"esc":       {Code: tea.KeyEscape},
		"backspace": {Code: tea.KeyBackspace},
		"tab":       {Code: tea.KeyTab},
		"shift+tab": {Code: tea.KeyTab, Mod: tea.ModShift},
		"up":        {Code: tea.KeyUp},
		"down":      {Code: tea.KeyDown},
		"ctrl+c":    {Code: 'c', Mod: tea.ModCtrl},
		"ctrl+d":    {Code: 'd', Mod: tea.ModCtrl},
		"ctrl+u":    {Code: 'u', Mod: tea.ModCtrl},
		"ctrl+k":    {Code: 'k', Mod: tea.ModCtrl},
	}
	if k2, ok := named[k]; ok {
		return tea.KeyPressMsg(k2)
	}
	r := []rune(k)[0]
	return tea.KeyPressMsg{Code: r, Text: k}
}

// screen is the rendered view without colours.
func screen(m Model) string { return ansi.Strip(m.View().Content) }

func mustContain(t *testing.T, m Model, want ...string) {
	t.Helper()
	s := screen(m)
	for _, w := range want {
		if !strings.Contains(s, w) {
			t.Fatalf("screen lacks %q:\n%s", w, s)
		}
	}
}

func mustNotContain(t *testing.T, m Model, bad ...string) {
	t.Helper()
	s := screen(m)
	for _, b := range bad {
		if strings.Contains(s, b) {
			t.Fatalf("screen has %q:\n%s", b, s)
		}
	}
}

// line returns the first screen line containing s.
func line(t *testing.T, m Model, s string) string {
	t.Helper()
	for _, l := range strings.Split(screen(m), "\n") {
		if strings.Contains(l, s) {
			return l
		}
	}
	t.Fatalf("no line with %q:\n%s", s, screen(m))
	return ""
}

func windowSize(w, h int) tea.WindowSizeMsg { return tea.WindowSizeMsg{Width: w, Height: h} }
