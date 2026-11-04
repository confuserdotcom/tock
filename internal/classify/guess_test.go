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
	if x := g["Arc · github.com"]; x.Category != "Code" || x.Project != "tock" || x.Reason != GuessSite || x.Share != 1 {
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

// TestGuessesTitleSameSiteOnly: on the web, title words only count from
// labelled pages of the same site; a shop sharing "pro" with a study page
// is not guessed Study.
func TestGuessesTitleSameSiteOnly(t *testing.T) {
	vs := []SpanView{
		{ID: 1, Start: 0, End: mins(30), BundleID: "arc", App: "Arc", Title: sp("MacBook Pro exercises"), URL: sp("notes.example.org/pro")},
		{ID: 2, Start: mins(40), End: mins(50), BundleID: "pdf", App: "PDF", Title: sp("MacBook Pro manual")},
		{ID: 3, Start: mins(100), End: mins(110), BundleID: "arc", App: "Arc", Title: sp("MacBook Pro 14"), URL: sp("ultrapc.ma/macbook")},
		{ID: 4, Start: mins(200), End: mins(230), BundleID: "arc", App: "Arc", Title: sp("Kitchen"), URL: sp("a.test/x")},
		{ID: 5, Start: mins(240), End: mins(270), BundleID: "arc", App: "Arc", Title: sp("Bath"), URL: sp("b.test/x")},
		{ID: 6, Start: mins(280), End: mins(310), BundleID: "arc", App: "Arc", Title: sp("Garden"), URL: sp("c.test/x")},
		{ID: 7, Start: mins(320), End: mins(350), BundleID: "arc", App: "Arc", Title: sp("Garage"), URL: sp("d.test/x")},
		{ID: 8, Start: mins(360), End: mins(390), BundleID: "arc", App: "Arc", Title: sp("Roof"), URL: sp("e.test/x")},
		{ID: 9, Start: mins(400), End: mins(430), BundleID: "arc", App: "Arc", Title: sp("Porch"), URL: sp("f.test/x")},
	}
	h := Label{Category: "Home"}
	ls := []Label{{Category: "Study"}, {Category: "Study"}, {}, h, h, h, h, h, h}
	if g, ok := Guesses(vs, ls)["Arc · ultrapc.ma"]; ok {
		t.Fatalf("shop guessed from other sites' titles: %+v", g)
	}
}

// TestGuessesTitleSameAppOnly: an app's title words only count from labelled
// views of that app.
func TestGuessesTitleSameAppOnly(t *testing.T) {
	vs := []SpanView{
		{ID: 1, Start: 0, End: mins(30), BundleID: "tv", App: "TV", Title: sp("Open season")},
		{ID: 2, Start: mins(40), End: mins(41), BundleID: "qt", App: "QuickTime", Title: sp("Open")},
	}
	h := Label{Category: "Home"}
	ls := []Label{{Category: "Fun"}, {}}
	for i := int64(0); i < 4; i++ {
		vs = append(vs, SpanView{ID: 10 + i, Start: mins(100 + 40*i), End: mins(130 + 40*i), BundleID: "x", App: "X", Title: sp(string(rune('a' + i)))})
		ls = append(ls, h)
	}
	if g, ok := Guesses(vs, ls)["QuickTime · open"]; ok {
		t.Fatalf("guessed from another app's titles: %+v", g)
	}
}
