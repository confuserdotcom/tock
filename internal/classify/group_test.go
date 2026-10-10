package classify

import "testing"

func TestInboxGroupsUnlabelled(t *testing.T) {
	rs := mustParse(t, labelRules, "")
	views := []SpanView{
		{ID: 1, Start: 0, End: 60_000, BundleID: "arc", App: "Arc", Title: sp("Chat A"), URL: sp("chatgpt.com/c/1")},
		{ID: 2, Start: 100_000, End: 220_000, BundleID: "arc", App: "Arc", Title: sp("Chat B"), URL: sp("chatgpt.com/c/2")},
		{ID: 3, Start: 300_000, End: 330_000, BundleID: "arc", App: "Arc", Private: true},
		{ID: 4, Start: 400_000, End: 900_000, BundleID: "term", App: "WezTerm", Title: sp("x")},
		{ID: 5, Start: 900_000, End: 990_000, BundleID: "tock.idle", App: "Idle"},
		{ID: 6, Start: 990_000, End: 995_000, BundleID: "fin", App: "Finder", Title: sp("Downloads")},
	}
	labels := rs.LabelAll(views, nil)
	items := Inbox(views, labels, map[string]bool{"Finder · downloads": true})
	if len(items) != 2 {
		t.Fatalf("items = %+v, want chatgpt and private", items)
	}
	if it := items[0]; it.Key != "Arc · chatgpt.com/c" || it.TotalMS != 180_000 || it.Spans != 2 || it.Example != "Chat B" || it.App != "Arc" {
		t.Fatalf("first item = %+v", it)
	}
	if it := items[1]; it.Key != "Arc (private)" || it.Example != "" {
		t.Fatalf("private item = %+v", it)
	}
}

func TestGroupSortsByTimeThenKey(t *testing.T) {
	views := []SpanView{
		{Start: 0, End: 10, App: "B", Title: sp("b")},
		{Start: 0, End: 10, App: "A", Title: sp("a")},
		{Start: 0, End: 50, App: "C", Title: sp("c")},
	}
	items := Group(views, func(int) bool { return true })
	if items[0].App != "C" || items[1].App != "A" || items[2].App != "B" {
		t.Fatalf("order = %v %v %v", items[0].App, items[1].App, items[2].App)
	}
}
