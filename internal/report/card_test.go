package report

import (
	"testing"

	"tock/internal/focus"
)

func TestSessionCardText(t *testing.T) {
	min := func(n int64) int64 { return n * 60_000 }
	if got := ShareText([]focus.Share{{Name: "Analyse 1", MS: min(24)}, {Name: "Anki", MS: min(12)}, {Name: "Reading", MS: min(6)}}); got != "Analyse 1 24m · Anki 12m · Reading 6m" {
		t.Fatalf("ShareText = %q", got)
	}
	if got := ShareText(nil); got != "" {
		t.Fatalf("ShareText(nil) = %q", got)
	}
	for rating, want := range map[int]string{0: "unrated", 1: "★☆☆☆☆", 4: "★★★★☆", 5: "★★★★★"} {
		if got := Stars(rating); got != want {
			t.Errorf("Stars(%d) = %q, want %q", rating, got, want)
		}
	}

	start, end := hm(t, "10:00"), hm(t, "10:50")
	manual := SessionDetail{Sess: focus.Session{Kind: focus.KindManual, StartMS: start, EndMS: end,
		DeepMS: min(42), LongestMS: min(32), Interruptions: 1}}
	if got := manual.RunLine(); got != "1 interruption · longest run 32m" {
		t.Errorf("RunLine = %q", got)
	}
	if got := manual.Summary(zone); got != "10:00–10:50 · 84% deep · 1 interruption" {
		t.Errorf("Summary = %q", got)
	}
	if got := manual.DeepPct(); got != 84 {
		t.Errorf("DeepPct = %d", got)
	}
	calm := manual
	calm.Sess.Interruptions = 0
	if got := calm.RunLine(); got != "no interruptions · longest run 32m" {
		t.Errorf("RunLine 0 = %q", got)
	}
	many := manual
	many.Sess.Interruptions = 3
	if got := many.Summary(zone); got != "10:00–10:50 · 84% deep · 3 interruptions" {
		t.Errorf("Summary 3 = %q", got)
	}

	auto := SessionDetail{Sess: focus.Session{Kind: focus.KindAuto, StartMS: start, EndMS: start + min(40),
		DeepMS: min(38), LongestMS: min(40)}}
	if got := auto.RunLine(); got != "auto-detected · longest run 40m" {
		t.Errorf("auto RunLine = %q", got)
	}
	if got := auto.Summary(zone); got != "10:00–10:40 · 95% deep · auto-detected" {
		t.Errorf("auto Summary = %q", got)
	}

	short := SessionDetail{Sess: focus.Session{StartMS: start, EndMS: start + min(50), DeepMS: min(41)}}
	if got := short.DeepPct(); got != 82 {
		t.Errorf("41 of 50 = %d, want 82", got)
	}
	if got := (SessionDetail{Sess: focus.Session{StartMS: start, EndMS: start, DeepMS: 5}}).DeepPct(); got != 0 {
		t.Errorf("zero length = %d", got)
	}
}

func TestUrgeLine(t *testing.T) {
	cases := []struct {
		resisted, given int
		want            string
	}{
		{0, 0, ""},
		{2, 1, "urges 2 resisted · 1 given in"},
		{3, 0, "urges 3 resisted · 0 given in"},
		{0, 3, "urges 0 resisted · 3 given in"},
	}
	for _, c := range cases {
		d := SessionDetail{Sess: focus.Session{UrgesResisted: c.resisted, UrgesGiven: c.given}}
		if got := d.UrgeLine(); got != c.want {
			t.Errorf("UrgeLine(%d, %d) = %q, want %q", c.resisted, c.given, got, c.want)
		}
	}
}
