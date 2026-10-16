package tui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"tock/internal/apptest"
)

// planModel is a seeded model with a todo.txt (written from lines) as its
// [plan] file and command (if any), opened on the plan tab.
func planModel(t *testing.T, command string, lines ...string) (Model, string) {
	t.Helper()
	e := apptest.New(t, apptest.Rules)
	e.Seed(t, apptest.DaySpans()...)
	file := filepath.Join(t.TempDir(), "todo.txt")
	if err := os.WriteFile(file, []byte(strings.Join(lines, "\n")+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg := "[plan]\nfile = \"" + file + "\"\n"
	if command != "" {
		cfg += "command = '" + command + "'\n"
	}
	e.WriteConfig(t, cfg)
	return press(t, newModel(t, e), "6"), file
}

func fileLines(t *testing.T, path string) []string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return strings.Split(strings.TrimRight(string(b), "\n"), "\n")
}

// apptest.Now is Saturday 2026-10-10.
var planLines = []string{
	"(D) late one due:2026-10-09",
	"(A) Thermo · TD 1 +thermo @sam-07h00 due:2026-10-10 id:W41-1",
	"(B) Méca · cours +meca @dim-09h00 due:2026-10-11 id:W41-2",
	"x 2026-10-10 Algèbre · tuteur +alg1 due:2026-10-10 pri:A",
	"x 2026-10-02 old history",
	"buy paper",
}

func TestPlanSections(t *testing.T) {
	m, _ := planModel(t, "", planLines...)
	mustContain(t, m, "Overdue · 1 left", "Today · 1 left", "Tomorrow · 1 left", "No date · 1 left", "Done",
		"[ ] D late one", "[ ] A Thermo · TD 1", "@sam-07h00 +thermo", "[x]   Algèbre · tuteur")
	mustNotContain(t, m, "old history", "due:", "id:W41")
	if r := m.plan.selected(); r == nil || r.Text != "late one" {
		t.Fatalf("cursor on %+v", r)
	}
}

func TestPlanToggleAndUndo(t *testing.T) {
	m, file := planModel(t, "", planLines...)
	m = press(t, m, "j", "enter")
	if got := fileLines(t, file)[1]; got != "x 2026-10-10 Thermo · TD 1 +thermo @sam-07h00 due:2026-10-10 id:W41-1 pri:A" {
		t.Fatalf("ticked line = %q", got)
	}
	mustContain(t, m, "done: Thermo · TD 1 (u undo)", "[x]   Thermo · TD 1")
	m = press(t, m, "u")
	if got := fileLines(t, file)[1]; got != planLines[1] {
		t.Fatalf("after undo = %q", got)
	}
	mustContain(t, m, "undone", "[ ] A Thermo · TD 1")
	m = step(t, m, tea.KeyPressMsg{Code: tea.KeySpace, Text: " "})
	if !strings.HasPrefix(fileLines(t, file)[1], "x 2026-10-10 ") {
		t.Fatalf("space did not tick: %q", fileLines(t, file)[1])
	}
}

func TestPlanAddDeleteUndo(t *testing.T) {
	m, file := planModel(t, "", planLines...)
	m = press(t, m, "a")
	mustContain(t, m, "Add a task", "enter add · esc cancel")
	m = typeText(t, m, "(B) call the prof +ana1 due:2026-10-10")
	m = press(t, m, "enter")
	ls := fileLines(t, file)
	if got := ls[len(ls)-1]; got != "(B) 2026-10-10 call the prof +ana1 due:2026-10-10" {
		t.Fatalf("added = %q", got)
	}
	mustContain(t, m, "added: call the prof", "Today · 2 left")
	m = press(t, m, "a")
	m = typeText(t, m, "q")
	m = press(t, m, "esc")
	if len(fileLines(t, file)) != len(ls) {
		t.Fatal("esc must not add")
	}
	m = press(t, m, "g", "d")
	if got := fileLines(t, file)[0]; got == planLines[0] {
		t.Fatal("d did not delete the first line")
	}
	mustContain(t, m, "deleted: late one")
	m = press(t, m, "u")
	if got := fileLines(t, file)[0]; got != planLines[0] {
		t.Fatalf("undo delete put back %q at the top", got)
	}
}

func TestPlanCommand(t *testing.T) {
	cmd := `echo "(A) from the planner +thermo due:2026-10-10" >> "$TOCK_PLAN_FILE"; ` +
		`printf "{\"lines\":[\"Maintenant · Thermo TD\"],\"seen\":{\"thermo\":2820}}"`
	m, file := planModel(t, cmd, planLines...)
	mustContain(t, m, "Maintenant · Thermo TD", "from the planner", "47m seen")
	if n := strings.Count(screen(m), "47m seen"); n != 1 {
		t.Errorf("seen shown %d times, want once per project", n)
	}
	n := len(fileLines(t, file))
	m = step(t, m, tickMsg{}) // within planEvery: no second run
	if len(fileLines(t, file)) != n {
		t.Error("the command ran again before a minute")
	}
	m = press(t, m, "g", "enter") // an edit reruns it
	if len(fileLines(t, file)) != n+1 {
		t.Error("an edit must rerun the command")
	}
}

func TestPlanCommandPlainAndError(t *testing.T) {
	m, _ := planModel(t, `echo "line one"; echo; echo "line two"`, "buy paper")
	mustContain(t, m, "line one", "line two", "buy paper")
	m, _ = planModel(t, `echo "bad thing" >&2; exit 3`, "buy paper")
	mustContain(t, m, "✗ plan command: bad thing", "buy paper")
}

func TestPlanNoFile(t *testing.T) {
	m, _ := seeded(t)
	m = press(t, m, "6", "a")
	mustContain(t, m, "No todo.txt to show.", "no plan file · set [plan] file")
}

func TestPlanGoneLine(t *testing.T) {
	m, file := planModel(t, "", "one", "two")
	if err := os.WriteFile(file, []byte("other\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	m = press(t, m, "enter")
	mustContain(t, m, "that line changed in the file meanwhile", "other")
}

func TestPlanClickSelects(t *testing.T) {
	m, _ := planModel(t, "", planLines...)
	y := -1
	for i, l := range strings.Split(screen(m), "\n") {
		if strings.Contains(l, "buy paper") {
			y = i
		}
	}
	if y < 0 {
		t.Fatal("no buy paper row")
	}
	m = step(t, m, tea.MouseClickMsg{X: 10, Y: y, Button: tea.MouseLeft})
	if r := m.plan.selected(); r == nil || r.Text != "buy paper" {
		t.Fatalf("click selected %+v", r)
	}
}
