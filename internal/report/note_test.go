package report

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
)

func TestCleanNote(t *testing.T) {
	for _, c := range []struct {
		in   string
		want string
		n    int
	}{
		{"a\r\nb\tc", "a b c", 5},
		{"a    b", "a b", 3},
		{"  padded  ", "padded", 6},
		{"x\x1b[31my", "x?[31my", 7},
		{"", "", 0},
	} {
		got, n := CleanNote(c.in)
		if got != c.want || n != c.n {
			t.Errorf("CleanNote(%q) = %q, %d; want %q, %d", c.in, got, n, c.want, c.n)
		}
	}
	long := strings.Repeat("é", 212)
	got, n := CleanNote(long)
	if n != 212 || got != strings.Repeat("é", MaxNoteRunes) {
		t.Errorf("long: %d runes, n %d", len([]rune(got)), n)
	}
}

func TestCut(t *testing.T) {
	name := strings.Repeat("a", 30)
	got := Cut(name, 24)
	if !strings.HasSuffix(got, "…") || ansi.StringWidth(got) != 24 {
		t.Errorf("Cut ascii = %q (%d cells)", got, ansi.StringWidth(got))
	}
	cjk := strings.Repeat("漢", 15) // 30 cells
	got = Cut(cjk, 24)
	if !strings.HasSuffix(got, "…") || ansi.StringWidth(got) > 24 {
		t.Errorf("Cut cjk = %q (%d cells)", got, ansi.StringWidth(got))
	}
	for _, r := range strings.TrimSuffix(got, "…") {
		if r != '漢' {
			t.Errorf("cut mid-rune: %q", got)
		}
	}
	if got := Cut("short", 24); got != "short" {
		t.Errorf("Cut short = %q", got)
	}
}
