package report

import (
	"strings"
	"testing"
)

func TestDur(t *testing.T) {
	for ms, want := range map[int64]string{0: "0s", 45_000: "45s", 60_000: "1m", 41*60_000 + 59_000: "41m", 3_600_000: "1h00", 2*3_600_000 + 14*60_000: "2h14"} {
		if got := Dur(ms); got != want {
			t.Errorf("Dur(%d) = %q, want %q", ms, got, want)
		}
	}
}

func TestBar(t *testing.T) {
	cases := []struct {
		ms, max int64
		cells   int
		want    string
	}{
		{100, 100, 12, strings.Repeat("█", 12)},
		{50, 100, 12, strings.Repeat("█", 6)},
		{1, 100, 12, "▌"},
		{0, 100, 12, ""},
		{13, 24, 12, strings.Repeat("█", 6) + "▌"},
		{100, 100, 20, strings.Repeat("█", 20)},
		{5, 100, 0, ""},
	}
	for _, c := range cases {
		if got := Bar(c.ms, c.max, c.cells); got != c.want {
			t.Errorf("Bar(%d,%d,%d) = %q, want %q", c.ms, c.max, c.cells, got, c.want)
		}
	}
}

func TestPad(t *testing.T) {
	if got := Pad("é", 3); got != "é  " {
		t.Fatalf("Pad counts runes: %q", got)
	}
	if got := Pad("long", 2); got != "long" {
		t.Fatalf("Pad never truncates: %q", got)
	}
}

func TestSafeStripsControls(t *testing.T) {
	if got := Safe("a\x1b[31mb\x07\u0085"); got != "a?[31mb??" {
		t.Fatalf("Safe = %q", got)
	}
}

func TestLabelText(t *testing.T) {
	for _, c := range []struct{ cat, proj, want string }{
		{"Study", "ATLAS", "Study / ATLAS"}, {"Study", "", "Study"}, {"", "ATLAS", "project ATLAS"},
	} {
		if got := LabelText(c.cat, c.proj); got != c.want {
			t.Errorf("LabelText(%q,%q) = %q, want %q", c.cat, c.proj, got, c.want)
		}
	}
}
