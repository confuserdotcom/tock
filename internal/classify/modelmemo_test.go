package classify

import (
	"fmt"
	"slices"
	"testing"
)

func memoFixture(t *testing.T) (*Ruleset, []SpanView, []Example) {
	t.Helper()
	rs := modelRules(t)
	views := []SpanView{
		tv("chromium", "Exercice de thermodynamique - Chromium", 60_000),
		tv("chromium", "Funny dogs - YouTube - Chromium", 60_000),
		tv("zathura", "Exercice thermodynamique PCSI.pdf", 60_000),
		tv("wez", "nvim exercice thermodynamique", 60_000),
	}
	ex := []Example{
		{View: tv("zathura", "Thermodynamique MPSI.pdf", 0), Category: "Study", Weight: 3},
		{View: tv("obsidian", "Exercice thermodynamique - TD", 0), Category: "Study", Weight: 3},
		{View: tv("obsidian", "Exercice 3 de thermodynamique", 0), Category: "Study", Weight: 3},
		{View: tv("chromium", "Funny cats - YouTube", 0), Category: "Entertainment", Weight: 3},
		{View: tv("wez", "nvim main.go", 0), Category: "Code", Weight: 3},
	}
	return rs, views, ex
}

func categories(ls []Label) []string {
	out := make([]string, len(ls))
	for i, l := range ls {
		out[i] = l.Category + "/" + string(l.CategorySource)
	}
	return out
}

// TestModelMemoSameLabels: a second apply with the same model gives the
// same labels and does not run the model again.
func TestModelMemoSameLabels(t *testing.T) {
	rs, views, ex := memoFixture(t)
	m := Train(ex, nil)
	first := rs.LabelAll(views, nil)
	rs.ApplyModel(m, views, first)
	again := func() []Label {
		ls := rs.LabelAll(views, nil)
		rs.ApplyModel(m, views, ls)
		return ls
	}
	if got := categories(again()); !slices.Equal(got, categories(first)) {
		t.Fatalf("memo changed labels: %v vs %v", got, categories(first))
	}
	cold := testing.AllocsPerRun(20, func() {
		rs.mu.Lock()
		clear(rs.modelMemo)
		rs.mu.Unlock()
		again()
	})
	warm := testing.AllocsPerRun(20, func() { again() })
	if warm*2 > cold {
		t.Fatalf("memo saves little: %v allocs warm, %v cold", warm, cold)
	}
}

// TestModelMemoNewModel: a different model is asked again.
func TestModelMemoNewModel(t *testing.T) {
	rs, views, ex := memoFixture(t)
	ls := rs.LabelAll(views, nil)
	rs.ApplyModel(Train(ex, nil), views, ls)
	if ls[2].Category != "Study" {
		t.Fatalf("first model: %v", categories(ls))
	}
	ex2 := []Example{
		{View: tv("zathura", "Thermodynamique MPSI.pdf", 0), Category: "Entertainment", Weight: 9},
		{View: tv("obsidian", "Exercice thermodynamique - TD", 0), Category: "Entertainment", Weight: 9},
		{View: tv("obsidian", "Exercice 3 de thermodynamique", 0), Category: "Entertainment", Weight: 9},
		{View: tv("wez", "nvim main.go", 0), Category: "Code", Weight: 3},
	}
	ls = rs.LabelAll(views, nil)
	rs.ApplyModel(Train(ex2, nil), views, ls)
	if ls[2].Category != "Entertainment" {
		t.Fatalf("a new model must not reuse the old answers: %v", categories(ls))
	}
}

// TestModelMemoBounded: the model memo starts over instead of growing.
func TestModelMemoBounded(t *testing.T) {
	rs, _, ex := memoFixture(t)
	m := Train(ex, nil)
	for i := 0; i < memoMax+10; i++ {
		vs := []SpanView{tv("zathura", fmt.Sprint("doc ", i, ".pdf"), 60_000)}
		rs.ApplyModel(m, vs, rs.LabelAll(vs, nil))
	}
	if n := len(rs.modelMemo); n > memoMax {
		t.Fatalf("model memo holds %d", n)
	}
}
