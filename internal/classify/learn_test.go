package classify

import (
	"errors"
	"testing"
)

const learnRules = baseRules + `
[[category]]
name = "Study"
kind = "deep"

[[project]]
name = "ATLAS"

[[rule]]
url = '^(www\.)?youtube\.com(/|$)'
category = "Entertainment"

[[rule]]
bundle = ["term"]
category = "Code"
`

func mins(n int64) int64 { return n * 60_000 }

func learnHistory() []SpanView {
	return []SpanView{
		{ID: 1, Start: 0, End: mins(3), BundleID: "arc", App: "Arc", Title: sp("XOR check"), URL: sp("chatgpt.com/c/1")},
		{ID: 2, Start: mins(10), End: mins(14), BundleID: "arc", App: "Arc", Title: sp("Proofs"), URL: sp("chatgpt.com/c/2")},
		{ID: 3, Start: mins(20), End: mins(25), BundleID: "arc", App: "Arc", Title: sp("Settings"), URL: sp("chatgpt.com/settings")},
		{ID: 4, Start: mins(30), End: mins(32), BundleID: "arc", App: "Arc", Title: sp("Video"), URL: sp("www.youtube.com/watch")},
		{ID: 5, Start: mins(40), End: mins(41), BundleID: "safari", App: "Safari", Title: sp("Chat"), URL: sp("chatgpt.com/c/3")},
		{ID: 6, Start: mins(50), End: mins(51), BundleID: "tock.idle", App: "Idle"},
	}
}

func TestProposeURLLadder(t *testing.T) {
	rs := mustParse(t, learnRules, "")
	h := learnHistory()
	cands, err := rs.Propose("Arc · chatgpt.com/c", Want{Category: "Study", Project: "ATLAS"}, h, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(cands) != 3 {
		t.Fatalf("got %d candidates, want host, host+segment, bundle+host", len(cands))
	}
	c := cands[0]
	if c.Rule.URL != `^(www\.)?chatgpt\.com(/|$)` || len(c.Rule.Bundle) != 0 {
		t.Fatalf("first candidate = %s", c.Rule.Describe())
	}
	if c.Rule.Category != "Study" || c.Rule.Project != "ATLAS" || c.Rule.File != LearnedFile {
		t.Fatalf("candidate label = %+v", c.Rule)
	}
	// spans 1,2 (this key) + 3 (settings) + 5 (safari) all unlabelled
	if c.Matches != 4 || c.ThisKey != 2 || c.New != 2 || c.NewMS != mins(6) || c.Relabeled != 0 || !c.Safe || c.MatchMS != mins(13) {
		t.Fatalf("host candidate counts = %+v", c)
	}
	if got := cands[1].Rule.URL; got != `^(www\.)?chatgpt\.com/c(/|$)` {
		t.Fatalf("second candidate url = %q", got)
	}
	if cands[1].Matches != 3 {
		t.Fatalf("host+segment matches = %d, want 3 (spans 1,2,5)", cands[1].Matches)
	}
	if b := cands[2].Rule.Bundle; len(b) != 1 || b[0] != "arc" || cands[2].Matches != 3 {
		t.Fatalf("bundle+host = %+v", cands[2])
	}
}

func TestProposeUnsafeWhenRelabelling(t *testing.T) {
	rs := mustParse(t, learnRules, "")
	h := learnHistory()
	h = append(h, SpanView{ID: 7, Start: mins(60), End: mins(61), BundleID: "arc", App: "Arc", Title: sp("Tutorial"), URL: sp("www.youtube.com/watch")})
	// Span 8 (the YouTube home page) has a different key and is
	// Entertainment by rule, so a whole-host rule would relabel it.
	h = append(h, SpanView{ID: 8, Start: mins(70), End: mins(71), BundleID: "arc", App: "Arc", Title: sp("Home"), URL: sp("www.youtube.com")})
	cands, err := rs.Propose("Arc · youtube.com/watch", Want{Category: "Study"}, h, nil)
	if err != nil {
		t.Fatal(err)
	}
	if cands[0].Safe || cands[0].Relabeled != 1 {
		t.Fatalf("host rule relabels the youtube.com home span: %+v", cands[0])
	}
	if !cands[1].Safe || cands[1].ThisKey != 2 {
		t.Fatalf("host+/watch candidate should be safe: %+v", cands[1])
	}
	// A manual override on span 8 means the host rule changes nothing there.
	cands, _ = rs.Propose("Arc · youtube.com/watch", Want{Category: "Study"}, h, map[int64]Override{8: {Category: "Entertainment"}})
	if !cands[0].Safe {
		t.Fatalf("manual spans must not count as relabelled: %+v", cands[0])
	}
}

func TestProposeTitleLadder(t *testing.T) {
	rs := mustParse(t, learnRules, "")
	h := []SpanView{
		{ID: 1, Start: 0, End: mins(5), BundleID: "obs", App: "Obsidian", Title: sp("Topology notes 1 - Vault - Obsidian")},
		{ID: 2, Start: mins(5), End: mins(9), BundleID: "obs", App: "Obsidian", Title: sp("Topology notes 2 - Vault - Obsidian")},
		{ID: 3, Start: mins(9), End: mins(10), BundleID: "obs", App: "Obsidian", Title: sp("Shopping notes - Vault - Obsidian")},
		{ID: 4, Start: mins(10), End: mins(11), BundleID: "term", App: "WezTerm", Title: sp("notes")},
	}
	cands, err := rs.Propose("Obsidian · topology notes # - vault", Want{Category: "Study"}, h, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(cands) != 2 {
		t.Fatalf("want bundle and word candidates (titles differ, so no exact-title one), got %d", len(cands))
	}
	if b := cands[0].Rule.Bundle; len(b) != 1 || b[0] != "obs" || cands[0].Rule.Title != "" || cands[0].Matches != 3 || !cands[0].Safe {
		t.Fatalf("bundle candidate = %+v", cands[0])
	}
	// "topology" is in 2 titles, "notes"/"vault"/"obsidian" in more: topology wins.
	if got := cands[1].Rule.Title; got != wordLeft+"topology"+wordRight || cands[1].Matches != 2 {
		t.Fatalf("word candidate title = %q matches %d", got, cands[1].Matches)
	}
}

func TestProposeExactTitleAndUnicodeWord(t *testing.T) {
	rs := mustParse(t, learnRules, "")
	h := []SpanView{
		{ID: 1, Start: 0, End: mins(1), BundleID: "obs", App: "Obsidian", Title: sp("Raisonnement déductif")},
		{ID: 2, Start: mins(1), End: mins(2), BundleID: "obs", App: "Obsidian", Title: sp("Raisonnement inductif")},
	}
	cands, err := rs.Propose("Obsidian · raisonnement déductif", Want{Category: "Study"}, h, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(cands) != 3 {
		t.Fatalf("want bundle, word, exact title; got %d", len(cands))
	}
	if cands[1].Rule.Title != wordLeft+"déductif"+wordRight || cands[1].Matches != 1 {
		t.Fatalf("word candidate = %q (%d matches)", cands[1].Rule.Title, cands[1].Matches)
	}
	if cands[2].Rule.Title != `^Raisonnement déductif$` || cands[2].Matches != 1 {
		t.Fatalf("exact candidate = %q", cands[2].Rule.Title)
	}
}

func TestProposeErrors(t *testing.T) {
	rs := mustParse(t, learnRules, "")
	h := learnHistory()
	h = append(h, SpanView{ID: 9, BundleID: "arc", App: "Arc", Private: true, End: 1})
	if _, err := rs.Propose("Nope · x", Want{Category: "Study"}, h, nil); !errors.Is(err, ErrNoSpans) {
		t.Fatalf("unknown key: %v", err)
	}
	if _, err := rs.Propose("Arc (private)", Want{Category: "Study"}, h, nil); !errors.Is(err, ErrPrivateKey) {
		t.Fatalf("private key: %v", err)
	}
}

func TestProposeProjectOnly(t *testing.T) {
	rs := mustParse(t, learnRules, "")
	h := []SpanView{
		{ID: 1, Start: 0, End: mins(1), BundleID: "term", App: "WezTerm", Title: sp("atlas build")},
		{ID: 2, Start: mins(1), End: mins(2), BundleID: "term", App: "WezTerm", Title: sp("tock build")},
	}
	cands, err := rs.Propose("WezTerm · atlas build", Want{Project: "ATLAS"}, h, nil)
	if err != nil {
		t.Fatal(err)
	}
	// Bundle-only gives span 2 the ATLAS project too: a gain, not a relabel.
	if c := cands[0]; !c.Safe || c.New != 1 || c.Rule.Category != "" || c.Rule.Project != "ATLAS" {
		t.Fatalf("project-only bundle candidate = %+v", c)
	}
}

// A wrong learned host rule labels every chatgpt.com key. Tagging one of
// them again fixes the rule instead of refusing as unsafe.
func TestProposeFixesLearnedMistake(t *testing.T) {
	wrong := `[[rule]]
url = '^(www\.)?chatgpt\.com(/|$)'
category = "Entertainment"
`
	rs := mustParse(t, learnRules, wrong)
	cands, err := rs.Propose("Arc · chatgpt.com/c", Want{Category: "Study"}, learnHistory(), nil)
	if err != nil {
		t.Fatal(err)
	}
	host := cands[0]
	if !host.Safe || host.Fixed != 2 || host.Relabeled != 0 || len(host.Replaces) != 1 || host.Replaces[0] != 0 {
		t.Fatalf("host candidate = %+v", host)
	}
	// The segment rule does not cover chatgpt.com/settings: it fixes the
	// span it matches but leaves the old rule in place.
	seg := cands[1]
	if !seg.Safe || seg.Fixed != 1 || len(seg.Replaces) != 0 {
		t.Fatalf("segment candidate = %+v", seg)
	}
}
