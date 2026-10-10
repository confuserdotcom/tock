package report

import (
	"testing"
	"time"

	"tock/internal/classify"
)

func TestBuildTop(t *testing.T) {
	sp := func(s string) *string { return &s }
	from := time.Date(2026, 10, 4, 0, 0, 0, 0, time.UTC)
	vs := []classify.SpanView{
		{Start: 0, End: 30 * 60_000, BundleID: "arc", App: "Arc", URL: sp("www.github.com/a")},
		{Start: 0, End: 10 * 60_000, BundleID: "arc", App: "Arc", URL: sp("youtube.com/watch")},
		{Start: 0, End: 50 * 60_000, BundleID: "term", App: "Term"},
		{Start: 0, End: 90 * 60_000, BundleID: "tock.idle", App: "Idle"},
		{Start: 0, End: 5 * 60_000, BundleID: "arc", App: "Arc", URL: sp("secret.example"), Private: true},
	}
	ls := []classify.Label{
		{Category: "Code", Kind: "deep", Project: "tock"},
		{Category: "Fun", Kind: "distract"},
		{Category: "Code", Kind: "deep"},
		{},
		{},
	}
	top := BuildTop(from, from.AddDate(0, 0, 7), vs, ls)
	if top.TotalMS != 95*60_000 {
		t.Fatalf("total = %s", Dur(top.TotalMS))
	}
	c := top.Sections[TopCategory]
	if len(c) != 3 || c[0].Name != "Code" || c[0].MS != 80*60_000 || c[0].Kind != "deep" || c[2].Name != Unlabelled {
		t.Fatalf("categories = %+v", c)
	}
	a := top.Sections[TopApp]
	if len(a) != 2 || a[0].Name != "Term" || a[1].Name != "Arc" || a[1].MS != 45*60_000 || a[1].Mostly != "Code" {
		t.Fatalf("apps = %+v", a)
	}
	s := top.Sections[TopSite]
	if len(s) != 2 || s[0].Name != "github.com" || s[1].Name != "youtube.com" {
		t.Fatalf("sites leak a private host or keep www: %+v", s)
	}
	if p := top.Sections[TopProject]; len(p) != 1 || p[0].Name != "tock" {
		t.Fatalf("projects = %+v", p)
	}
}
