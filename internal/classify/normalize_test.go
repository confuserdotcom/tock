package classify

import (
	"bufio"
	"os"
	"strings"
	"testing"
	"unicode/utf8"
)

func TestCleanTable(t *testing.T) {
	cases := []struct{ in, want string }{
		{"[1/3] ✳ workflow phase 1 init", "workflow phase 1 init"},
		{"[1/3] ◐ workflow phase 1 init", "workflow phase 1 init"},
		{"⠋ cargo build", "cargo build"},
		{"tock │ ✳ fix tests", "tock │ fix tests"},
		{"(3) Inbox - Gmail", "Inbox - Gmail"},
		{"(99+) general", "general"},
		{"main.go — Edited", "main.go"},
		{"main.go - Edited", "main.go"},
		{"Essay draft [Modified]", "Essay draft"},
		{"● main.go - tock - Visual Studio Code", "main.go - tock - Visual Studio Code"},
		{"* general | Acme - Slack", "general | Acme - Slack"},
		{"notes.md*", "notes.md"},
		{"🚀 Roadmap Q4", "Roadmap Q4"},
		{"Chapter 2 — Notes", "Chapter 2 — Notes"},
		{"Edited", "Edited"},
		{"   spaced    out   ", "spaced out"},
		{"✳", ""},
		{"", ""},
		{"*(3)", ""},
	}
	for _, c := range cases {
		if got := Clean(c.in); got != c.want {
			t.Errorf("Clean(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

// TestNormalizeGolden checks Clean and Key on testdata/normalize.golden.
// Columns: app, title, url, private (0/1), Clean(title), Key. <nil> is a nil
// pointer.
func TestNormalizeGolden(t *testing.T) {
	f, err := os.Open("testdata/normalize.golden")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	n := 0
	for line := 1; sc.Scan(); line++ {
		if strings.HasPrefix(sc.Text(), "#") || sc.Text() == "" {
			continue
		}
		c := strings.Split(sc.Text(), "\t")
		if len(c) != 6 {
			t.Fatalf("line %d: %d columns, want 6", line, len(c))
		}
		v := SpanView{App: c[0], Title: optField(c[1]), URL: optField(c[2]), Private: c[3] == "1"}
		if v.Title != nil {
			if got := Clean(*v.Title); got != c[4] {
				t.Errorf("line %d: Clean(%q) = %q, want %q", line, c[1], got, c[4])
			}
		}
		if got := Key(v); got != c[5] {
			t.Errorf("line %d: Key = %q, want %q", line, got, c[5])
		}
		n++
	}
	if n < 40 {
		t.Fatalf("golden has %d rows, want at least 40", n)
	}
}

// allNoise reports whether every token of s is something Clean removes.
func allNoise(s string) bool {
	for _, tok := range strings.Fields(s) {
		tok = strings.Trim(tok, "*")
		switch {
		case tok == "", isNoiseToken(tok), counterRe.MatchString(tok),
			tok == "—", tok == "-", tok == "Edited", tok == "[Modified]", tok == "—Edited", tok == "-Edited":
		default:
			return false
		}
	}
	return true
}

func FuzzClean(f *testing.F) {
	for _, s := range []string{"[1/3] ✳ x", "(3) Inbox", "a — Edited", "*", "✳", "tock │ y", "*(3)"} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, s string) {
		c := Clean(s)
		if Clean(c) != c {
			t.Fatalf("not idempotent: %q -> %q -> %q", s, c, Clean(c))
		}
		if c != strings.TrimSpace(c) {
			t.Fatalf("untrimmed result %q", c)
		}
		if c == "" && !allNoise(s) {
			t.Fatalf("Clean(%q) is empty but the title is not all noise", s)
		}
		if utf8.ValidString(s) && !utf8.ValidString(c) {
			t.Fatalf("Clean broke UTF-8: %q", c)
		}
	})
}

func FuzzKey(f *testing.F) {
	f.Add("Arc", "x", "chatgpt.com/c/1", false)
	f.Add("Obsidian", "a - V - Obsidian 1.2", "", false)
	f.Fuzz(func(t *testing.T, app, title, url string, private bool) {
		v := SpanView{App: app, Title: &title, Private: private}
		if url != "" {
			v.URL = &url
		}
		k := Key(v)
		if !strings.HasPrefix(k, app) {
			t.Fatalf("Key %q does not start with app %q", k, app)
		}
		if Key(v) != k {
			t.Fatal("Key not deterministic")
		}
	})
}
