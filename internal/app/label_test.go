package app

import (
	"os"
	"strings"
	"testing"

	"tock/internal/apptest"
	"tock/internal/classify"
	"tock/internal/report"
)

func TestIgnoreAndUndo(t *testing.T) {
	e := apptest.New(t, apptest.Rules)
	e.Seed(t, apptest.DaySpans()...)
	a := open(t, e)
	if err := a.Ignore("Arc · chatgpt.com/c", true); err != nil {
		t.Fatal(err)
	}
	if items, _ := a.Inbox(7); len(items) != 2 {
		t.Fatalf("ignored key still listed: %+v", items)
	}
	if err := a.Ignore("Arc · chatgpt.com/c", false); err != nil {
		t.Fatal(err)
	}
	if items, _ := a.Inbox(7); len(items) != 3 {
		t.Fatalf("un-ignore failed: %+v", items)
	}
}

func TestTag(t *testing.T) {
	e := apptest.New(t, apptest.Rules)
	e.Seed(t, apptest.DaySpans()...)
	a := open(t, e)
	if err := a.Tag([]int64{4, 7}, classify.Want{Category: "Study"}); err != nil {
		t.Fatal(err)
	}
	if err := a.Tag([]int64{4}, classify.Want{Project: "ATLAS"}); err != nil {
		t.Fatal(err)
	}
	d, _ := a.Day(e.Clock)
	for _, c := range d.Categories {
		if c.Name == "Study" {
			if report.Dur(c.MS) != "12m" || len(c.Projects) != 1 || c.Projects[0].Name != "ATLAS" {
				t.Fatalf("Study row = %+v", c)
			}
			return
		}
	}
	t.Fatalf("no Study row: %+v", d.Categories)
}

func TestTagUnknownSpan(t *testing.T) {
	e := apptest.New(t, apptest.Rules)
	e.Seed(t, apptest.DaySpans()...)
	a := open(t, e)
	if err := a.Tag([]int64{999}, classify.Want{Category: "Study"}); err == nil || err.Error() != "no span with id 999" {
		t.Fatalf("unknown span: %v", err)
	}
}

func TestProposeLearnAppliesToHistory(t *testing.T) {
	e := apptest.New(t, apptest.Rules)
	e.Seed(t, apptest.DaySpans()...)
	e.Seed(t, apptest.Span{ID: 20, Start: "10:00", End: "10:30", Bundle: "arc", App: "Arc", Title: "Old chat", URL: "chatgpt.com/c/9", Day: -3})
	a := open(t, e)
	want := classify.Want{Category: "Study"}
	cands, err := a.Propose("Arc · chatgpt.com/c", want)
	if err != nil || len(cands) == 0 || !cands[0].Safe {
		t.Fatalf("candidates %+v, %v", cands, err)
	}
	if err := a.Learn("Arc · chatgpt.com/c", cands[0], want); err != nil {
		t.Fatal(err)
	}
	body, _ := os.ReadFile(e.Learned)
	if !strings.Contains(string(body), `# learned 2026-10-10 from "Arc · chatgpt.com/c", 3 spans`) {
		t.Fatalf("learned.toml:\n%s", body)
	}
	d, _ := a.Day(e.Clock.AddDate(0, 0, -3))
	var study int64
	for _, c := range d.Categories {
		if c.Name == "Study" {
			study = c.MS
		}
	}
	if report.Dur(study) != "30m" {
		t.Fatalf("learned rule did not reach a span 3 days back: Study = %s", report.Dur(study))
	}
}
