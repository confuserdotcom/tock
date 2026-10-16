package main

import (
	"strings"
	"testing"
)

func TestAwayListAnswerSkip(t *testing.T) {
	te := newTestEnv(t, testRules)
	te.seed(t, daySpans()...)
	code, out := te.run(t, "", "away")
	want := "11:41–12:22    41m\n12:25–14:50   2h25\nlabel: tock away 11:41 <Category>[/<Project>] · leave idle: tock away --skip 11:41\n"
	if code != 0 || out != want {
		t.Fatalf("list: code %d\n%s", code, out)
	}
	if code, out := te.run(t, "", "away", "11:41", "comm/atl"); code != 0 || out != "labelled 11:41–12:22 (41m) → Communication / ATLAS\n" {
		t.Fatalf("answer: code %d %q %q", code, out, te.errb.String())
	}
	if code, out := te.run(t, "", "away", "--skip", "12:25"); code != 0 || out != "left 12:25–14:50 (2h25) idle\n" {
		t.Fatalf("skip: code %d %q", code, out)
	}
	if code, out := te.run(t, "", "away"); code != 0 || out != "no time away to label\n" {
		t.Fatalf("after: code %d %q", code, out)
	}
	if code, out := te.run(t, "", "today"); code != 0 || !strings.Contains(out, "Communication    41m  ███▌          ATLAS 41m") {
		t.Fatalf("today:\n%s", out)
	}
}

func TestAwayErrors(t *testing.T) {
	te := newTestEnv(t, testRules)
	te.seed(t, daySpans()...)
	if code, _ := te.run(t, "", "away", "11:41"); code != 2 {
		t.Fatalf("missing label: code %d", code)
	}
	if code, _ := te.run(t, "", "away", "--skip"); code != 2 {
		t.Fatalf("missing time: code %d", code)
	}
	if code, _ := te.run(t, "", "away", "11:40", "Study"); code != 1 ||
		te.errb.String() != "tock: no time away starts at 11:40 (try 11:41, 12:25)\n" {
		t.Fatalf("unknown time: code %d %q", code, te.errb.String())
	}
	if code, _ := te.run(t, "", "away", "--date", "2026-10-09", "--skip", "11:41"); code != 1 ||
		te.errb.String() != "tock: no time away to label on 2026-10-09\n" {
		t.Fatalf("other day: code %d %q", code, te.errb.String())
	}
	if code, _ := te.run(t, "", "away", "11:41", "Nope"); code == 0 {
		t.Fatal("unknown category accepted")
	}
}

func TestAwaySame(t *testing.T) {
	te := newTestEnv(t, testRules)
	te.seed(t, seedSpan{1, "09:00", "10:00", "term", "WezTerm", "tock │ x", "", false},
		seedSpan{2, "10:00", "10:30", "tock.idle", "Idle", "", "", false},
		seedSpan{3, "10:30", "11:00", "term", "WezTerm", "tock │ y", "", false})
	code, out := te.run(t, "", "away")
	if code != 0 || !strings.Contains(out, "10:00–10:30    30m  after Code / tock\n") || !strings.Contains(out, "tock away <HH:MM> same") {
		t.Fatalf("list: code %d\n%s", code, out)
	}
	if code, out := te.run(t, "", "away", "10:00", "same"); code != 0 || out != "labelled 10:00–10:30 (30m) → Code / tock\n" {
		t.Fatalf("same: code %d %q %q", code, out, te.errb.String())
	}
}
