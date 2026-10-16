package app

import (
	"os"
	"strings"
	"testing"

	"tock/internal/apptest"
	"tock/internal/classify"
)

func TestCategoryLifecycle(t *testing.T) {
	e := apptest.New(t, apptest.Rules)
	e.Seed(t, apptest.DaySpans()...)
	a := open(t, e)

	if err := a.AddCategory(CategorySpec{Name: "Reading", Kind: classify.KindNeutral, AFKMS: 10 * 60_000}); err != nil {
		t.Fatal(err)
	}
	if !a.Rules().HasCategory("reading") || a.Rules().AFKWindow("Reading") != 10*60_000 {
		t.Fatalf("categories %+v", a.Rules().Categories)
	}
	if b, err := os.ReadFile(e.Rules + ".bak"); err != nil || string(b) != apptest.Rules {
		t.Fatalf("backup: %v", err)
	}
	for _, bad := range []CategorySpec{{Name: "reading", Kind: "deep"}, {Name: "X", Kind: "loud"}, {Name: "X"},
		{Name: "a/b", Kind: "deep"}, {Name: "X", Kind: "deep", AFKMS: 60_000}} {
		if err := a.AddCategory(bad); err == nil {
			t.Errorf("AddCategory(%+v) accepted", bad)
		}
	}

	if err := a.EditCategory(CategorySpec{Name: "read", Kind: classify.KindBrowse, AFKMS: -1}); err != nil {
		t.Fatal(err)
	}
	if a.Rules().Kind("Reading") != classify.KindBrowse || a.Rules().AFKWindow("Reading") != classify.DefaultAFKMS {
		t.Fatalf("after edit %+v", a.Rules().Categories)
	}

	// Tag a span Communication by hand, then rename it: the tag follows.
	if err := a.Tag([]int64{4}, classify.Want{Category: "Communication"}); err != nil {
		t.Fatal(err)
	}
	from, err := a.RenameName(classify.NameCategory, "comm", "Messages")
	if err != nil || from != "Communication" {
		t.Fatalf("rename: %q %v", from, err)
	}
	if !strings.Contains(timeline(t, a), "11:26-11:35 Arc Messages") {
		t.Fatalf("timeline:\n%s", timeline(t, a))
	}
	if _, err := a.RemoveName(classify.NameCategory, "Messages"); err == nil || !strings.Contains(err.Error(), "labels you set by hand") {
		t.Fatalf("removed a hand-labelled category: %v", err)
	}
	if _, err := a.RemoveName(classify.NameCategory, "Code"); err == nil {
		t.Fatal("removed a category rules name")
	}
	if d, err := a.RemoveName(classify.NameCategory, "reading"); err != nil || d != "Reading" || a.Rules().HasCategory("Reading") {
		t.Fatalf("remove: %q %v", d, err)
	}
}
