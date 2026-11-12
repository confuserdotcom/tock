package bar

import (
	"strings"
	"testing"
	"time"

	"tock/internal/app"
	"tock/internal/focus"
	"tock/internal/store"
)

func TestBudgetIsland(t *testing.T) {
	const h = int64(3600_000)
	n := store.Notice{ID: 7, Topic: focus.TopicBudget, Title: "tock: 85% of its 40h budget", Body: "34h spent, 6h left."}
	runs := time.Date(2026, 11, 20, 12, 0, 0, 0, time.UTC).UnixMilli()
	i := BudgetIsland(n, app.NoticeData{Project: "tock", BudgetMS: 40 * h, SpentMS: 34 * h, RunsOutMS: runs}, time.UTC)
	if i.State != IslandBudget || !i.Open || i.Notice != 7 || i.AutoClose == 0 {
		t.Fatalf("island = %+v", i)
	}
	if i.Face.Dot != DotBreak || i.Face.Left != "tock" || i.Face.Right != "85%" {
		t.Fatalf("face = %+v", i.Face)
	}
	p := i.Panel
	if p == nil || p.Lead != n.Title {
		t.Fatalf("panel = %+v", p)
	}
	cells := map[string]Cell{}
	for _, c := range p.Cells {
		cells[c.K] = c
	}
	if cells["Spent"].V != "34h00" || cells["Budget"].V != "40h" || cells["Left"].V != "6h00" || cells["Runs out"].V != "Fri 20 Nov" {
		t.Fatalf("cells = %+v", p.Cells)
	}
	if len(p.Strip) != budgetCells || p.Strip[0] != "break" || p.Strip[budgetCells-1] != "none" {
		t.Fatalf("strip = %v", p.Strip)
	}
	if len(p.Axis) != 2 || p.Axis[1] != "40h" {
		t.Fatalf("axis = %v", p.Axis)
	}
	if len(p.Buttons) != 2 || p.Buttons[0].ID != IDDashboard || p.Buttons[1].ID != IDNotchClose {
		t.Fatalf("buttons = %+v", p.Buttons)
	}

	over := BudgetIsland(n, app.NoticeData{Project: "tock", BudgetMS: 40 * h, SpentMS: 50 * h}, time.UTC)
	if over.Face.Dot != "distract" || over.Face.Right != "125%" {
		t.Fatalf("over face = %+v", over.Face)
	}
	oc := map[string]Cell{}
	for _, c := range over.Panel.Cells {
		oc[c.K] = c
	}
	if oc["Over"].V != "10h00" || oc["Over"].Tone != "bad" {
		t.Fatalf("over cells = %+v", over.Panel.Cells)
	}
	if _, ok := oc["Runs out"]; ok {
		t.Fatal("over budget still shows a run-out day")
	}
	if s := over.Panel.Strip; s[0] != "break" || s[budgetCells-1] != "distract" {
		t.Fatalf("over strip = %v", s)
	}

	// A notice queued by an older trackd carries no numbers: its text stands in.
	old := BudgetIsland(n, app.NoticeData{}, time.UTC)
	if old.Panel.Lead != n.Title || old.Panel.Sub != n.Body || len(old.Panel.Cells) != 0 || old.Panel.Strip != nil {
		t.Fatalf("old = %+v", old.Panel)
	}
	if !strings.HasPrefix(old.Face.Left, "tock") {
		t.Fatalf("old face = %+v", old.Face)
	}
}

func TestBudgetIsNotchTopic(t *testing.T) {
	if !NotchTopic(focus.TopicBudget) {
		t.Fatal("budget notices go to a banner")
	}
	if Rank(IslandBudget) != Rank(IslandRecap) || Rank(IslandBudget) <= Rank(IslandLabel) {
		t.Fatalf("rank = %d", Rank(IslandBudget))
	}
}
