package classify

import (
	"slices"
	"testing"
)

func tv(bundle, title string, ms int64) SpanView {
	return SpanView{ID: 1, End: ms, BundleID: bundle, App: bundle, Title: &title}
}

func TestFeatures(t *testing.T) {
	meta, title := Features(tv("chromium", "Apprendre à conjecturer - Chromium", 0))
	if !slices.Equal(meta, []string{"b:chromium"}) {
		t.Fatalf("meta = %v", meta)
	}
	slices.Sort(title)
	// accents folded, the app suffix gone, words cut to their first 6 letters
	if want := []string{"appren", "conjec"}; !slices.Equal(title, want) {
		t.Fatalf("title = %v", title)
	}
	u := "www.youtube.com/watch"
	v := tv("arc", "Cours 3 - YouTube", 0)
	v.URL = &u
	if meta, title := Features(v); !slices.Equal(meta, []string{"b:arc", "h:youtube.com"}) || !slices.Equal(title, []string{"cour"}) {
		t.Fatalf("features = %v %v", meta, title)
	}
	v.Private = true
	if meta, title := Features(v); !slices.Equal(meta, []string{"b:arc"}) || title != nil {
		t.Fatalf("private features = %v %v", meta, title)
	}
}

func modelRules(t *testing.T) *Ruleset {
	t.Helper()
	rs, err := Parse([]byte(`
[[category]]
name = "Study"
kind = "deep"

[[category]]
name = "Entertainment"
kind = "distract"

[[category]]
name = "Code"
kind = "deep"

[[project]]
name = "Thermo"
category = "Study"

[[rule]]
bundle = ["chromium"]
category = "Entertainment"

[[rule]]
bundle = ["wez"]
title = 'nvim'
category = "Code"
`), nil)
	if err != nil {
		t.Fatal(err)
	}
	return rs
}

func TestModelLearnsFromLabels(t *testing.T) {
	ex := []Example{
		{View: tv("zathura", "Thermodynamique MPSI.pdf", 0), Category: "Study", Weight: 1},
		{View: tv("obsidian", "Thermodynamique - TD 2 - Licence", 0), Category: "Study", Weight: 1},
		{View: tv("obsidian", "Conjecture de Collatz - Licence", 0), Category: "Study", Weight: 1},
		{View: tv("zathura", "Physique MPSI chapitre 2.pdf", 0), Category: "Study", Weight: 1},
		{View: tv("zathura", "Physique MPSI exercices.pdf", 0), Category: "Study", Weight: 1},
		{View: tv("chromium", "Lofi beats - YouTube", 0), Category: "Entertainment", Weight: 1},
		{View: tv("chromium", "Funny cats - YouTube", 0), Category: "Entertainment", Weight: 1},
		{View: tv("wez", "nvim main.go", 0), Category: "Code", Weight: 1},
	}
	m := Train(ex, nil)
	if c, p, n := m.Predict(tv("zathura", "Physique MPSI.pdf", 0)); c != "Study" || p < 0.8 || n < 2 {
		t.Fatalf("predict = %s %.2f %d", c, p, n)
	}
	if c, p, _ := m.PredictTitle(tv("chromium", "Thermodynamique MPSI - Chromium", 0), ""); c != "Study" || p < 0.75 {
		t.Fatalf("predict title = %s %.2f", c, p)
	}
	if _, _, n := m.Predict(tv("unknown", "zzz qqq", 0)); n != 0 {
		t.Fatalf("unknown features counted: %d", n)
	}
}

func TestModelSeed(t *testing.T) {
	m := Train(nil, map[string][]string{"Study": SeedWords})
	if c, p, _ := m.PredictTitle(tv("chromium", "Apprendre à conjecturer - Chromium", 0), ""); c != "Study" || p < 0.9 {
		t.Fatalf("seed = %s %.2f", c, p)
	}
}

func TestApplyModel(t *testing.T) {
	rs := modelRules(t)
	views := []SpanView{
		tv("chromium", "Exercice de thermodynamique - Chromium", 60_000), // generic rule, title says Study
		tv("chromium", "Funny dogs - YouTube - Chromium", 60_000),        // generic rule, title agrees
		tv("zathura", "Exercice thermodynamique PCSI.pdf", 60_000),       // no rule
		tv("wez", "nvim exercice thermodynamique", 60_000),               // a rule with a title: never
		tv("chromium", "Exercice thermodynamique - Chromium", 60_000),    // manual: never
	}
	ovs := map[int64]Override{}
	labels := rs.LabelAll(views, ovs)
	labels[4] = Label{Category: "Entertainment", CategorySource: SourceManual}
	ex := []Example{
		{View: tv("zathura", "Thermodynamique MPSI.pdf", 0), Category: "Study", Weight: 3},
		{View: tv("obsidian", "Exercice thermodynamique - TD", 0), Category: "Study", Weight: 3},
		{View: tv("obsidian", "Exercice 3 de thermodynamique", 0), Category: "Study", Weight: 3},
		{View: tv("chromium", "Funny cats - YouTube", 0), Category: "Entertainment", Weight: 3},
		{View: tv("wez", "nvim main.go", 0), Category: "Code", Weight: 3},
	}
	rs.ApplyModel(Train(ex, nil), views, labels)
	got := make([]string, len(labels))
	for i, l := range labels {
		got[i] = l.Category + "/" + string(l.CategorySource)
	}
	want := []string{"Study/model", "Entertainment/rule", "Study/model", "Code/rule", "Entertainment/manual"}
	if !slices.Equal(got, want) {
		t.Fatalf("labels = %v", got)
	}
	if labels[0].Kind != "deep" {
		t.Fatalf("kind = %q", labels[0].Kind)
	}
}

func TestTrainingSet(t *testing.T) {
	rs := modelRules(t)
	views := []SpanView{
		tv("chromium", "a - Chromium", 10*60_000),
		tv("chromium", "a - Chromium", 10*60_000), // same key: one example
		tv("wez", "nvim x", 60_000),
		tv("mail", "inbox", 60_000), // unlabelled
	}
	labels := rs.LabelAll(views, nil)
	labels[2].CategorySource = SourceManual
	ex := rs.TrainingSet(views, labels)
	if len(ex) != 2 || ex[0].Category != "Entertainment" || ex[1].Category != "Code" {
		t.Fatalf("examples = %+v", ex)
	}
	if ex[0].Weight >= ex[1].Weight {
		t.Fatalf("a generic rule outweighs a manual label: %v vs %v", ex[0].Weight, ex[1].Weight)
	}
}

func TestApplyModelScreen(t *testing.T) {
	rs := modelRules(t)
	screen := func(v SpanView, ws ...string) SpanView { v.Screen = ws; return v }
	views := []SpanView{
		screen(tv("ghostty", "zsh", 60_000), "thermodynamique", "exercice", "entropie"), // unlabelled, the screen says Study
		tv("ghostty", "zsh", 60_000), // same window, nothing read: stays unlabelled
		screen(tv("zathura", "Exercice thermodynamique PCSI.pdf", 60_000), "exercice"), // the title alone decides: model
	}
	views[1].ID, views[2].ID = 2, 3
	pv := screen(tv("ghostty", "zsh", 60_000), "thermodynamique", "exercice", "entropie")
	pv.ID, pv.Private = 4, true // never from a private window
	views = append(views, pv)
	labels := rs.LabelAll(views, nil)
	ex := []Example{
		{View: tv("zathura", "Thermodynamique MPSI.pdf", 0), Category: "Study", Weight: 3},
		{View: tv("obsidian", "Exercice thermodynamique - TD", 0), Category: "Study", Weight: 3},
		{View: tv("obsidian", "Exercice 3 de thermodynamique", 0), Category: "Study", Weight: 3},
		{View: tv("chromium", "Funny cats - YouTube", 0), Category: "Entertainment", Weight: 3},
		{View: tv("wez", "nvim main.go", 0), Category: "Code", Weight: 3},
	}
	rs.ApplyModel(Train(ex, nil), views, labels)
	got := make([]string, len(labels))
	for i, l := range labels {
		got[i] = l.Category + "/" + string(l.CategorySource)
	}
	want := []string{"Study/screen", "/none", "Study/model", "/none"}
	if !slices.Equal(got, want) {
		t.Fatalf("labels = %v, want %v", got, want)
	}
}

func TestScreenWordsWeighLess(t *testing.T) {
	ex := []Example{
		{View: tv("a", "alpha", 0), Category: "Study", Weight: 1},
		{View: tv("a", "alpha", 0), Category: "Study", Weight: 1},
		{View: tv("a", "beta", 0), Category: "Code", Weight: 1},
		{View: tv("a", "beta", 0), Category: "Code", Weight: 1},
	}
	m := Train(ex, nil)
	v := tv("a", "alpha", 0)
	v.Screen = []string{"beta"}
	if c, _, n := m.Predict(v); c != "Study" || n != 2 {
		t.Fatalf("title beats screen: %s %d", c, n)
	}
}
