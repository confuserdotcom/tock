package focus

import (
	"testing"

	"tock/internal/classify"
	"tock/internal/config"
)

func TestIsMeeting(t *testing.T) {
	cfg := config.DefaultFocus()
	u := func(s string) *string { return &s }
	tests := []struct {
		v    classify.SpanView
		want bool
	}{
		{classify.SpanView{BundleID: "us.zoom.xos"}, true},
		{classify.SpanView{BundleID: "com.apple.FaceTime"}, true},
		{classify.SpanView{BundleID: "arc", URL: u("meet.google.com/abc-defg-hij")}, true},
		{classify.SpanView{BundleID: "arc", URL: u("eu.whereby.com/room")}, true},
		{classify.SpanView{BundleID: "arc", URL: u("MEET.GOOGLE.COM/x")}, true},
		{classify.SpanView{BundleID: "arc", URL: u("notmeet.google.com/x")}, false},
		{classify.SpanView{BundleID: "arc", URL: u("www.youtube.com/watch")}, false},
		{classify.SpanView{BundleID: "term"}, false},
	}
	for _, tt := range tests {
		if got := IsMeeting(tt.v, cfg); got != tt.want {
			t.Errorf("IsMeeting(%s %v) = %v, want %v", tt.v.BundleID, tt.v.URL, got, tt.want)
		}
	}
}

func TestWindowStats(t *testing.T) {
	deep, dis := classify.KindDeep, classify.KindDistract
	vs, ls := world(
		vw{from: at(10, 0, 0), to: at(10, 10, 0), kind: deep},
		vw{from: at(10, 10, 0), to: at(10, 10, 30), kind: dis}, // 30 s: tolerated
		vw{from: at(10, 10, 30), to: at(10, 20, 0), kind: deep},
		vw{from: at(10, 20, 0), to: at(10, 22, 0), kind: dis}, // 2 min: breaks the stretch
		vw{from: at(10, 22, 0), to: at(10, 25, 0), kind: deep},
		vw{from: at(10, 25, 0), to: at(10, 40, 0), kind: deep, bundle: "us.zoom.xos"}, // meeting: skipped
		vw{from: at(10, 40, 0), to: at(10, 50, 0), kind: "idle"},
		vw{from: at(10, 50, 0), to: at(10, 55, 0), kind: deep},
		vw{from: at(10, 55, 0), to: at(10, 58, 0), kind: dis, url: "meet.google.com/x"}, // meeting: skipped
	)
	cfg := config.DefaultFocus()
	tests := []struct {
		from, to int64
		want     Stats
	}{
		{at(10, 0, 0), at(11, 0, 0), Stats{DeepMS: 27*minute + 30_000, DistractMS: 2*minute + 30_000, LongestMS: 19*minute + 30_000}},
		{at(10, 5, 0), at(10, 15, 0), Stats{DeepMS: 9*minute + 30_000, DistractMS: 30_000, LongestMS: 9*minute + 30_000}},
		{at(12, 0, 0), at(13, 0, 0), Stats{}},
	}
	for _, tt := range tests {
		if got := WindowStats(vs, ls, tt.from, tt.to, cfg); got != tt.want {
			t.Errorf("WindowStats(%d, %d) = %+v, want %+v", tt.from, tt.to, got, tt.want)
		}
	}
}
