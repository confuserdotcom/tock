package agentlog

import (
	"testing"

	"tock/internal/classify"
)

func TestAttach(t *testing.T) {
	m := int64(60_000)
	str := func(s string) *string { return &s }
	views := []classify.SpanView{
		{ID: 1, BundleID: "term", Start: 10 * m, End: 20 * m, Title: str("tock — ✳ Fix the inbox — node")},
		{ID: 2, BundleID: "term", Start: 10 * m, End: 20 * m, Title: str("tock — ✳ Fix the inbox"), Dir: str("~/kept")},
		{ID: 3, BundleID: "arc", Start: 10 * m, End: 20 * m, Title: str("Fix the inbox - Google")},
		{ID: 4, BundleID: "term", Start: 300 * m, End: 310 * m, Title: str("✳ Fix the inbox")}, // long after
		{ID: 5, BundleID: "term", Start: 10 * m, End: 20 * m, Title: str("✳ Fix the inbox"), Private: true},
		{ID: 6, BundleID: "term", Start: 100 * m, End: 105 * m, Title: str("✳ Fix the inbox")}, // nearer the later session
		{ID: 7, BundleID: "term", Start: 10 * m, End: 20 * m, Title: str("✳ Tea")},             // title too short to trust
	}
	sessions := []Session{
		{ID: "a", Dir: "/h/tock", Titles: []string{"Fix the inbox"}, Events: []int64{5 * m, 15 * m, 40 * m}},
		{ID: "b", Dir: "/h/tock/web", Titles: []string{"Fix the inbox"}, Events: []int64{90 * m, 110 * m}},
		{ID: "c", Dir: "/h/x", Titles: []string{"Tea"}, Events: []int64{10 * m}},
	}
	Attach(views, sessions, []string{"term"}, "/h")
	want := map[int64]string{1: "~/tock", 2: "~/kept", 3: "", 4: "", 5: "", 6: "~/tock/web", 7: ""}
	for _, v := range views {
		got := ""
		if v.Dir != nil {
			got = *v.Dir
		}
		if got != want[v.ID] {
			t.Errorf("view %d: dir %q, want %q", v.ID, got, want[v.ID])
		}
	}
}
