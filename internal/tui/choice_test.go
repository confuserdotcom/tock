package tui

import (
	"errors"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/charmbracelet/x/exp/golden"

	"tock/internal/config"
)

type choiceFake struct {
	saves    []map[string]bool
	saveErr  error
	opens    int
	link     string
	remember int
}

func newChoiceT(t *testing.T, always bool) (Choice, *choiceFake) {
	t.Helper()
	f := &choiceFake{link: "http://127.0.0.1:7421/#tok"}
	m := NewChoice(config.Default(), always, ChoiceDeps{
		Save: func(c map[string]bool) error {
			if f.saveErr != nil {
				return f.saveErr
			}
			f.saves = append(f.saves, c)
			return nil
		},
		Open:     func() (string, error) { f.opens++; return f.link, nil },
		Remember: func() error { f.remember++; return nil },
	})
	return choiceStep(t, m, tea.WindowSizeMsg{Width: 90, Height: 30}), f
}

func choiceStep(t *testing.T, m Choice, msg tea.Msg) Choice {
	t.Helper()
	next, cmd := m.Update(msg)
	m = next.(Choice)
	for cmd != nil {
		out := cmd()
		if _, ok := out.(tea.QuitMsg); ok || out == nil {
			return m
		}
		next, cmd = m.Update(out)
		m = next.(Choice)
	}
	return m
}

func choicePress(t *testing.T, m Choice, keys ...string) Choice {
	t.Helper()
	for _, k := range keys {
		m = choiceStep(t, m, key(k))
	}
	return m
}

func choiceScreen(m Choice) string { return ansi.Strip(m.View().Content) }

// row is the screen line naming feature name.
func row(t *testing.T, m Choice, name string) string {
	t.Helper()
	for _, l := range strings.Split(choiceScreen(m), "\n") {
		if strings.Contains(l, name) {
			return l
		}
	}
	t.Fatalf("no row %q:\n%s", name, choiceScreen(m))
	return ""
}

func TestGoldenChoice(t *testing.T) {
	m, _ := newChoiceT(t, false)
	golden.RequireEqual(t, choiceScreen(m))
}

// Space toggles the row under the cursor; nothing is written until enter.
func TestChoiceToggleThenSave(t *testing.T) {
	m, f := newChoiceT(t, false)
	ocr := indexOf(t, "ocr")
	m = choicePress(t, m, strings.Split(strings.Repeat("j", ocr), "")...)
	m = choicePress(t, m, " ")
	if l := row(t, m, "Screen text"); !strings.Contains(l, "●") || !strings.Contains(l, "changed") {
		t.Fatalf("toggled row = %q", l)
	}
	if len(f.saves) != 0 {
		t.Fatal("saved before enter")
	}
	golden.RequireEqual(t, choiceScreen(m))
	m = choicePress(t, m, "enter")
	if len(f.saves) != 1 || len(f.saves[0]) != 1 || f.saves[0]["ocr"] != true {
		t.Fatalf("saves = %v", f.saves)
	}
	if l := row(t, m, "Screen text"); strings.Contains(l, "changed") {
		t.Fatalf("saved row still changed: %q", l)
	}
	if !strings.Contains(choiceScreen(m), "saved: Screen text on") {
		t.Fatalf("no saved line:\n%s", choiceScreen(m))
	}
	if got := m.Saved(); got["ocr"] != true || len(got) != 1 {
		t.Fatalf("Saved = %v", got)
	}
	// Toggling back twice is no change: enter writes nothing.
	m = choicePress(t, m, " ", " ", "enter")
	if len(f.saves) != 1 {
		t.Fatalf("saves = %v", f.saves)
	}
}

func TestChoiceSaveError(t *testing.T) {
	m, f := newChoiceT(t, false)
	f.saveErr = errors.New("config: set [ocr] enabled: change it by hand")
	m = choicePress(t, m, " ", "enter")
	if !strings.Contains(choiceScreen(m), "change it by hand") || len(m.Saved()) != 0 {
		t.Fatalf("screen:\n%s", choiceScreen(m))
	}
	if !strings.Contains(row(t, m, "Local model labels"), "changed") {
		t.Fatal("pending change dropped after a failed save")
	}
}

// q quits; pending changes are reported, not written.
func TestChoiceQuitPending(t *testing.T) {
	m, f := newChoiceT(t, false)
	m = choicePress(t, m, " ")
	next, cmd := m.Update(key("q"))
	m = next.(Choice)
	if cmd == nil {
		t.Fatal("q did not quit")
	}
	if _, ok := cmd().(tea.QuitMsg); !ok {
		t.Fatal("q did not quit")
	}
	if len(f.saves) != 0 || m.Pending() != 1 {
		t.Fatalf("saves %v pending %d", f.saves, m.Pending())
	}
}

func TestChoiceDashboardAsks(t *testing.T) {
	m, f := newChoiceT(t, false)
	m = choicePress(t, m, "G", "enter")
	if f.opens != 0 {
		t.Fatal("opened without asking")
	}
	golden.RequireEqual(t, choiceScreen(m))
	m = choicePress(t, m, "n")
	if f.opens != 0 || strings.Contains(choiceScreen(m), "Open the web dashboard?") {
		t.Fatal("not now still open")
	}
	m = choicePress(t, m, "enter", "o")
	if f.opens != 1 || !strings.Contains(choiceScreen(m), f.link) {
		t.Fatalf("opens %d:\n%s", f.opens, choiceScreen(m))
	}
	m = choicePress(t, m, "enter", "a")
	if f.opens != 2 || f.remember != 1 {
		t.Fatalf("always: opens %d remember %d", f.opens, f.remember)
	}
	// Remembered: no question.
	m = choicePress(t, m, "enter")
	if f.opens != 3 || strings.Contains(choiceScreen(m), "Open the web dashboard?") {
		t.Fatalf("asked again; opens %d", f.opens)
	}
}

func TestChoiceDashboardAlways(t *testing.T) {
	m, f := newChoiceT(t, true)
	m = choicePress(t, m, "G", "enter")
	if f.opens != 1 {
		t.Fatalf("opens = %d", f.opens)
	}
}

func TestChoiceCursorBounds(t *testing.T) {
	m, _ := newChoiceT(t, false)
	m = choicePress(t, m, "k", "up")
	if !strings.HasPrefix(strings.TrimSpace(row(t, m, "Local model labels")), "›") {
		t.Fatal("cursor left the top")
	}
	m = choicePress(t, m, "G", "j", "down")
	if !strings.HasPrefix(strings.TrimSpace(row(t, m, "Open dashboard")), "›") {
		t.Fatal("cursor left the bottom")
	}
	m = choicePress(t, m, " ") // space on the dashboard row does nothing
	if m.Pending() != 0 {
		t.Fatal("space on the dashboard row changed something")
	}
}

func indexOf(t *testing.T, id string) int {
	t.Helper()
	for i, f := range config.Features {
		if f.ID == id {
			return i
		}
	}
	t.Fatalf("no feature %s", id)
	return 0
}
