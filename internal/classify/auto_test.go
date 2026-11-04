package classify

import "testing"

func TestAutoPicks(t *testing.T) {
	items := []Item{
		{Key: "site"}, {Key: "app"}, {Key: "title"}, {Key: "weak"}, {Key: "proj"}, {Key: "done"}, {Key: "priv", Private: true}, {Key: "none"},
	}
	g := map[string]Guess{
		"site":  {Want: Want{Category: "Entertainment"}, Share: 0.88, Reason: GuessSite},
		"app":   {Want: Want{Category: "Code", Project: "tock"}, Share: 0.85, Reason: GuessApp},
		"title": {Want: Want{Category: "Study"}, Share: 1, Reason: GuessTitle},
		"weak":  {Want: Want{Category: "Study"}, Share: 0.84, Reason: GuessSite},
		"proj":  {Want: Want{Project: "Excellence"}, Share: 1, Reason: GuessSite},
		"done":  {Want: Want{Category: "Study"}, Share: 1, Reason: GuessSite},
		"priv":  {Want: Want{Category: "Study"}, Share: 1, Reason: GuessApp},
	}
	got := AutoPicks(items, g, map[string]bool{"done": true}, 0.85)
	if len(got) != 2 || got[0].Key != "site" || got[0].Category != "Entertainment" || got[1].Key != "app" || got[1].Project != "tock" {
		t.Fatalf("picks = %+v", got)
	}
	if AutoPicks(items, g, nil, 0) != nil {
		t.Fatal("0 must pick nothing")
	}
}
