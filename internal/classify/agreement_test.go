package classify

import (
	"math"
	"testing"
)

func TestAgreement(t *testing.T) {
	rs := modelRules(t)
	views := []SpanView{
		tv("chromium", "Funny dogs - YouTube - Chromium", 60_000), // rule agrees with the hand label
		tv("wez", "nvim notes.md", 120_000),                       // rule says Code, the hand says Study
		tv("mail", "inbox", 30_000),                               // nothing labels it
		tv("chromium", "Lofi - YouTube - Chromium", 600_000),      // no hand label: not measured
	}
	for i := range views {
		views[i].ID = int64(i + 1)
	}
	ovs := map[int64]Override{
		1: {Category: "Entertainment"},
		2: {Category: "Study"},
		3: {Category: "Code"},
		4: {Project: "Thermo"}, // a project only: says nothing of the category
	}
	a := rs.Agreement(views, ovs, 5)
	if a.Spans != 3 || a.Agreed != 1 || a.Unlabelled != 1 {
		t.Fatalf("spans %d agreed %d unlabelled %d, want 3 1 1", a.Spans, a.Agreed, a.Unlabelled)
	}
	if a.MS != 210_000 || a.AgreedMS != 60_000 || a.UnlabelledMS != 30_000 {
		t.Fatalf("ms %d agreed %d unlabelled %d", a.MS, a.AgreedMS, a.UnlabelledMS)
	}
	if len(a.Confusions) != 1 || a.Confusions[0] != (Confusion{Label: "Study", Guess: "Code", MS: 120_000, N: 1}) {
		t.Fatalf("confusions = %+v", a.Confusions)
	}
	byCat := map[string]AgreementRow{}
	for _, r := range a.PerCategory {
		byCat[r.Category] = r
	}
	// Code: one hand label (missed), and one guess of Code that was wrong.
	if r := byCat["Code"]; r.MS != 30_000 || r.AgreedMS != 0 || r.GuessedMS != 120_000 {
		t.Fatalf("Code row = %+v", r)
	}
	if r := byCat["Entertainment"]; r.MS != 60_000 || r.AgreedMS != 60_000 || r.GuessedMS != 60_000 {
		t.Fatalf("Entertainment row = %+v", r)
	}
}

func TestAgreementHoldsOut(t *testing.T) {
	// No fold's model may train on a key that fold scores: the same window
	// labelled by hand on two days is one key, and must not teach itself.
	rs := modelRules(t)
	var views []SpanView
	ovs := map[int64]Override{}
	for i := range 40 {
		v := tv("zathura", "Exercice "+string(rune('A'+i%20))+".pdf", 60_000)
		v.ID = int64(i + 1)
		views = append(views, v)
		if i%3 != 0 {
			ovs[v.ID] = Override{Category: "Study"}
		}
	}
	ex := rs.TrainingPart(views, rs.LabelAll(views, ovs))
	scored := 0
	for _, f := range splitFolds(views, ovs, ex, 5) {
		keys := map[string]bool{}
		for _, i := range f.held {
			keys[Key(views[i])] = true
			scored++
		}
		for _, e := range f.train {
			if keys[Key(e.View)] {
				t.Fatalf("fold trains on %q, which it scores", Key(e.View))
			}
		}
		if len(f.train) == 0 {
			t.Fatal("a fold trains on nothing")
		}
	}
	if want := len(ovs); scored != want {
		t.Fatalf("scored %d hand labels, want each of the %d once", scored, want)
	}
}

func TestWilson(t *testing.T) {
	lo, hi := Wilson(80, 100)
	if math.Abs(lo-0.711) > 0.005 || math.Abs(hi-0.867) > 0.005 {
		t.Fatalf("wilson(80,100) = %.3f..%.3f", lo, hi)
	}
	if lo, hi := Wilson(0, 0); lo != 0 || hi != 1 {
		t.Fatalf("wilson(0,0) = %v..%v", lo, hi)
	}
}
