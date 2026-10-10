package classify

import "testing"

func guessHistory() ([]SpanView, []Label) {
	lab := func(c, p string) Label { return Label{Category: c, Project: p, CategorySource: SourceLearned} }
	none := Label{}
	vs := []SpanView{
		// github.com is labelled Code/tock: a new github path guesses the same.
		{ID: 1, Start: 0, End: mins(30), BundleID: "arc", App: "Arc", Title: sp("tock PR"), URL: sp("github.com/me/tock")},
		{ID: 2, Start: mins(40), End: mins(45), BundleID: "arc", App: "Arc", Title: sp("issues"), URL: sp("github.com/notifications")},
		// A terminal is mostly Code, a little Writing: 10/12 > 60 %.
		{ID: 3, Start: mins(60), End: mins(70), BundleID: "term", App: "Term", Title: sp("vim main.go")},
		{ID: 4, Start: mins(70), End: mins(72), BundleID: "term", App: "Term", Title: sp("notes")},
		{ID: 5, Start: mins(80), End: mins(84), BundleID: "term", App: "Term", Title: sp("htop")},
		// An unknown app between two Study views is guessed from context.
		{ID: 6, Start: mins(100), End: mins(130), BundleID: "pdf", App: "PDF", Title: sp("Analyse chapter 3")},
		{ID: 7, Start: mins(131), End: mins(140), BundleID: "calc", App: "Calc"},
		{ID: 8, Start: mins(141), End: mins(170), BundleID: "pdf", App: "PDF", Title: sp("Analyse chapter 4")},
		// Nothing labelled near or alike: no guess.
		{ID: 9, Start: mins(500), End: mins(510), BundleID: "game", App: "Game"},
		// Private views are never guessed.
		{ID: 10, Start: mins(600), End: mins(610), BundleID: "arc", App: "Arc", Private: true},
		{ID: 11, Start: mins(611), End: mins(700), BundleID: "tock.idle", App: "Idle"},
	}
	ls := []Label{lab("Code", "tock"), none, lab("Code", ""), lab("Writing", ""), none,
		lab("Study", ""), none, lab("Study", ""), none, none, none}
	return vs, ls
}

func TestGuesses(t *testing.T) {
	g := Guesses(guessHistory())
	if len(g) != 3 {
		t.Fatalf("guesses = %+v", g)
	}
	if x := g["Arc · github.com/notifications"]; x.Category != "Code" || x.Project != "tock" || x.Reason != GuessSite || x.Share != 1 {
		t.Errorf("github guess = %+v", x)
	}
	if x := g["Term · htop"]; x.Category != "Code" || x.Project != "" || x.Reason != GuessApp {
		t.Errorf("terminal guess = %+v", x)
	}
	if x := g["Calc"]; x.Category != "Study" || x.Reason != GuessNear {
		t.Errorf("context guess = %+v", x)
	}
}

func TestGuessesNeedEvidence(t *testing.T) {
	vs := []SpanView{
		{ID: 1, Start: 0, End: mins(2), BundleID: "term", App: "Term", Title: sp("a")},
		{ID: 2, Start: mins(10), End: mins(14), BundleID: "term", App: "Term", Title: sp("b")},
	}
	if g := Guesses(vs, []Label{{Category: "Code"}, {}}); len(g) != 0 {
		t.Fatalf("guessed from 2 minutes: %+v", g)
	}
}
