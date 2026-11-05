package bar

import (
	"strings"
	"testing"

	"tock/internal/app"
	"tock/internal/focus"
	"tock/internal/store"
)

func TestLoadLabelsAndPick(t *testing.T) {
	e, a := seeded(t)
	es, err := LoadLabels(a, e.Clock)
	if err != nil {
		t.Fatal(err)
	}
	if len(es) == 0 || len(es) > labelEntries {
		t.Fatalf("entries = %d", len(es))
	}
	first := es[0]
	if first.Key == "" || first.Dur == "" || first.Picked != -1 || len(first.Opts) == 0 || len(first.Opts) > 3 {
		t.Fatalf("entry = %+v", first)
	}
	for _, o := range first.Opts {
		if o.Kind == "" {
			t.Fatalf("option without a kind: %+v", first.Opts)
		}
	}
	saved, err := Pick(a, first, Option{Label: "Study", Kind: "deep"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(saved, "Rule saved") && !strings.HasPrefix(saved, "Labelled") {
		t.Fatalf("saved = %q", saved)
	}
	again, err := LoadLabels(a, e.Clock)
	if err != nil {
		t.Fatal(err)
	}
	for _, x := range again {
		if x.Key == first.Key {
			t.Fatalf("picked key %q still in the inbox", first.Key)
		}
	}
	if _, err := Pick(a, first, Option{Label: "No such category"}); err == nil {
		t.Fatal("unknown label picked")
	}
}

func TestMoment(t *testing.T) {
	e, a := seeded(t)
	i, ok, err := Moment(a, store.Notice{ID: 4, Topic: focus.TopicRecap}, app.NoticeData{}, e.Clock, 0)
	if err != nil || !ok || i.State != IslandRecap || i.Face.Right == "0s" || len(i.Panel.Strip) != stripCells {
		t.Fatalf("recap = %+v %v %v", i, ok, err)
	}
	i, ok, err = Moment(a, store.Notice{ID: 5, Topic: focus.TopicDone, Title: "Session done"}, app.NoticeData{Session: 999}, e.Clock, 0)
	if err != nil || !ok || i.State != IslandDone || i.Panel.Lead != "Session done" {
		t.Fatalf("done of a gone session = %+v %v %v", i, ok, err)
	}
	if _, ok, _ := Moment(a, store.Notice{Topic: focus.TopicUpcoming}, app.NoticeData{}, e.Clock, 0); ok {
		t.Fatal("up next opens a moment")
	}
}
