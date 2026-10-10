package classify

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestAppendLearnedNewestFirstAndRoundTrip(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "cfg")
	path := filepath.Join(dir, LearnedFile)
	r1 := Rule{URL: `^(www\.)?chatgpt\.com(/|$)`, Category: "Code"}
	r2 := Rule{Bundle: []string{"term"}, Title: `^it's "quoted" \d$`, Project: "tock"}
	if err := AppendLearned(path, r1, `learned 2026-10-10 from "Arc · chatgpt.com/c", 4 spans`); err != nil {
		t.Fatal(err)
	}
	if err := AppendLearned(path, r2, "learned 2026-10-11 from \"x\nmultiline\", 1 spans"); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	if !strings.HasPrefix(text, "# tock learned rules") {
		t.Fatalf("missing header:\n%s", text)
	}
	if strings.Index(text, "2026-10-11") > strings.Index(text, "2026-10-10") {
		t.Fatalf("newest rule must come first:\n%s", text)
	}
	rs, err := Parse([]byte(baseRules), data)
	if err != nil {
		t.Fatalf("written file does not parse: %v\n%s", err, text)
	}
	if len(rs.Learned) != 2 || rs.Learned[0].Title != r2.Title || rs.Learned[1].URL != r1.URL || rs.Learned[0].Bundle[0] != "term" {
		t.Fatalf("round trip = %+v", rs.Learned)
	}
	for _, p := range []string{path, dir} {
		fi, err := os.Stat(p)
		if err != nil {
			t.Fatal(err)
		}
		want := os.FileMode(0o600)
		if fi.IsDir() {
			want = 0o700
		}
		if fi.Mode().Perm() != want {
			t.Errorf("%s mode = %v, want %v", p, fi.Mode().Perm(), want)
		}
	}
}

func TestAppendLearnedKeepsUserEdits(t *testing.T) {
	path := filepath.Join(t.TempDir(), LearnedFile)
	orig := "# my notes\n\n# keep me\n[[rule]]\napp = 'x'\ncategory = 'Code'\n"
	if err := os.WriteFile(path, []byte(orig), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := AppendLearned(path, Rule{App: "y", Category: "Code"}, "new"); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(path)
	text := string(data)
	if !strings.HasPrefix(text, "# my notes\n\n# new\n[[rule]]\napp = \"y\"") || !strings.Contains(text, "# keep me\n[[rule]]\napp = 'x'") {
		t.Fatalf("user content not preserved or new rule misplaced:\n%s", text)
	}
}

func TestAppendLearnedRefusesCorruptFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), LearnedFile)
	bad := "[[rule]\nbroken"
	if err := os.WriteFile(path, []byte(bad), 0o600); err != nil {
		t.Fatal(err)
	}
	err := AppendLearned(path, Rule{App: "y", Category: "Code"}, "new")
	if err == nil || !strings.Contains(err.Error(), "not modified") {
		t.Fatalf("err = %v, want parse error mentioning not modified", err)
	}
	if data, _ := os.ReadFile(path); string(data) != bad {
		t.Fatal("corrupt learned.toml was modified")
	}
}
