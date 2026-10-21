package classify

import (
	"fmt"
	"strings"
	"testing"
)

func remotePieces(vs []SpanView) string {
	parts := make([]string, len(vs))
	for i, v := range vs {
		id := fmt.Sprint(v.ID)
		if r, ok := RemoteRowID(v.ID); ok {
			id = fmt.Sprintf("r%d", r)
		}
		parts[i] = fmt.Sprintf("%s:%d-%d %s", id, v.Start, v.End, v.App)
	}
	return strings.Join(parts, ", ")
}

func TestMarkRemote(t *testing.T) {
	views := []SpanView{
		{ID: 1, Start: 0, End: 10, BundleID: "a", App: "A"},
		{ID: 2, Start: 10, End: 40, BundleID: idleB, App: "Idle"},
		{ID: 3, Start: 40, End: 50, BundleID: "a", App: "A"},
	}
	title := "~/tock"
	marks := []RemoteMark{
		{ID: 5, Machine: "ryuk", View: SpanView{Start: 5, End: 30, BundleID: "kitty", App: "kitty", Title: &title}},
		{ID: 6, Machine: "ryuk", View: SpanView{Start: 20, End: 45, BundleID: "firefox", App: "firefox"}}, // overlaps 5
		{ID: 7, Machine: "ryuk", View: SpanView{Start: 60, End: 80, BundleID: "kitty", App: "kitty"}},     // in a gap, past to
	}
	got := remotePieces(MarkRemote(views, marks, 0, 70))
	want := "1:0-10 A, r5:10-30 ryuk · kitty, r6:30-40 ryuk · firefox, 3:40-50 A, r7:60-70 ryuk · kitty"
	if got != want {
		t.Fatalf("got  %s\nwant %s", got, want)
	}
	out := MarkRemote(views, marks, 0, 70)
	if !IsRemote(out[1]) || IsRemote(out[0]) || out[1].BundleID != "kitty" || *out[1].Title != "~/tock" {
		t.Fatalf("remote view = %+v", out[1])
	}
	if _, ok := MeetingEventID(out[1].ID); ok {
		t.Fatal("a remote id must not read as a meeting")
	}
	if e, ok := MeetingEventID(-(MeetingIDBase + 3)); !ok || e != 3 {
		t.Fatal("meeting ids still work")
	}
	if got := MarkRemote(views, nil, 0, 70); len(got) != 3 {
		t.Fatal("no marks leaves views alone")
	}
	labels := make([]Label, len(out))
	if items := Inbox(out, labels, nil); strings.Contains(fmt.Sprint(items), "kitty") {
		t.Fatalf("remote views must stay out of the inbox: %+v", items)
	}
}

func TestMarkRemoteLatestSwitchWins(t *testing.T) {
	inCall := SpanView{ID: 4, Start: 100 * mn, End: 130 * mn, BundleID: "zoom", App: "Zoom", InCall: true}
	views := []SpanView{
		{ID: 1, Start: 0, End: 60 * mn, BundleID: "arc", App: "Arc"},
		{ID: 2, Start: 60 * mn, End: 90 * mn, BundleID: "wez", App: "Wez"},
		inCall,
	}
	marks := []RemoteMark{
		// started before the Mac's view: the Mac switched later and keeps it
		{ID: 5, Machine: "ryuk", View: SpanView{Start: -10 * mn, End: 10 * mn, BundleID: "pdf", App: "zathura"}},
		// a page turn on ryuk at 20 min: ryuk is in front until it stops
		{ID: 6, Machine: "ryuk", View: SpanView{Start: 20 * mn, End: 40 * mn, BundleID: "pdf", App: "zathura"}},
		// within the grace after the Mac switched: clock skew, the Mac keeps it
		{ID: 7, Machine: "ryuk", View: SpanView{Start: 60*mn + 10_000, End: 70 * mn, BundleID: "pdf", App: "zathura"}},
		// never over a call
		{ID: 8, Machine: "ryuk", View: SpanView{Start: 110 * mn, End: 120 * mn, BundleID: "pdf", App: "zathura"}},
	}
	got := remotePieces(MarkRemote(views, marks, -60*mn, 200*mn))
	want := fmt.Sprintf("r5:%d-0 ryuk · zathura, 1:0-%d Arc, r6:%d-%d ryuk · zathura, 1:%d-%d Arc, 2:%d-%d Wez, 4:%d-%d Zoom",
		-10*mn, 20*mn, 20*mn, 40*mn, 40*mn, 60*mn, 60*mn, 90*mn, 100*mn, 130*mn)
	if got != want {
		t.Fatalf("got  %s\nwant %s", got, want)
	}
}
