package classify

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const reorderCats = `[[category]]
name = "Code"
kind = "deep"

`

func TestSwapRules(t *testing.T) {
	tests := []struct {
		name, in, want string
		i              int
	}{
		{
			name: "comments follow their rule",
			in:   reorderCats + "# a\n[[rule]]\napp = 'A'\ncategory = \"Code\"\n\n# b\n[[rule]]\napp = 'B'\ncategory = \"Code\"\n",
			want: reorderCats + "# b\n[[rule]]\napp = 'B'\ncategory = \"Code\"\n\n# a\n[[rule]]\napp = 'A'\ncategory = \"Code\"\n",
		},
		{
			name: "section banner stays in place",
			in:   reorderCats + "[[rule]]\napp = 'A'\ncategory = \"Code\"\n\n# --- section ---\n\n# b\n[[rule]]\napp = 'B'\ncategory = \"Code\"\n",
			want: reorderCats + "# b\n[[rule]]\napp = 'B'\ncategory = \"Code\"\n\n# --- section ---\n\n[[rule]]\napp = 'A'\ncategory = \"Code\"\n",
		},
		{
			name: "header with trailing comment, last pair",
			in:   reorderCats + "[[rule]]\napp = 'A'\ncategory = \"Code\"\n\n[[rule]] # second\napp = 'B'\ncategory = \"Code\"\n\n[[rule]]\napp = 'C'\ncategory = \"Code\"\n",
			i:    1,
			want: reorderCats + "[[rule]]\napp = 'A'\ncategory = \"Code\"\n\n[[rule]]\napp = 'C'\ncategory = \"Code\"\n\n[[rule]] # second\napp = 'B'\ncategory = \"Code\"\n",
		},
		{
			name: "no trailing newline",
			in:   reorderCats + "[[rule]]\napp = 'A'\ncategory = \"Code\"\n\n[[rule]]\napp = 'B'\ncategory = \"Code\"",
			want: reorderCats + "[[rule]]\napp = 'B'\ncategory = \"Code\"\n\n[[rule]]\napp = 'A'\ncategory = \"Code\"",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := SwapRules(RulesFile, []byte(tt.in), tt.i)
			if err != nil {
				t.Fatal(err)
			}
			if string(got) != tt.want {
				t.Errorf("got\n%s\nwant\n%s", got, tt.want)
			}
		})
	}
}

func TestSwapRulesRefuses(t *testing.T) {
	two := reorderCats + "[[rule]]\napp = 'A'\ncategory = \"Code\"\n\n[[rule]]\napp = 'B'\ncategory = \"Code\"\n"
	tests := []struct {
		name, in, msg string
		i             int
	}{
		{"before first", two, "no rules", -1},
		{"after last", two, "no rules", 1},
		{"table between", reorderCats + "[[rule]]\napp = 'A'\ncategory = \"Code\"\n\n[[project]]\nname = \"p\"\n\n[[rule]]\napp = 'B'\ncategory = \"Code\"\n", "another table", 0},
		{"blank line inside a rule", reorderCats + "[[rule]]\napp = 'A'\n\ncategory = \"Code\"\n\n[[rule]]\napp = 'B'\ncategory = \"Code\"\n", "edit the file", 0},
		{"does not decode", "[[rule]\napp = 'A'\n", "rules.toml", 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := SwapRules(RulesFile, []byte(tt.in), tt.i)
			if err == nil || !strings.Contains(err.Error(), tt.msg) {
				t.Fatalf("err = %v, want it to mention %q", err, tt.msg)
			}
		})
	}
}

// Every adjacent swap of the starter file parses and swapping back restores
// the bytes exactly.
func TestSwapRulesStarterRoundTrip(t *testing.T) {
	rs, err := Parse(Starter, nil)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i+1 < len(rs.Rules); i++ {
		once, err := SwapRules(RulesFile, Starter, i)
		if err != nil {
			t.Fatalf("swap %d: %v", i, err)
		}
		got, err := Parse(once, nil)
		if err != nil {
			t.Fatalf("swap %d: %v", i, err)
		}
		if got.Rules[i].Describe() != rs.Rules[i+1].Describe() || got.Rules[i+1].Describe() != rs.Rules[i].Describe() {
			t.Fatalf("swap %d did not exchange the rules", i)
		}
		twice, err := SwapRules(RulesFile, once, i)
		if err != nil || string(twice) != string(Starter) {
			t.Fatalf("swap %d back: %v", i, err)
		}
	}
}

func TestMoveRule(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "cfg")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, LearnedFile)
	in := learnedHeader + "# a\n[[rule]]\napp = 'A'\nproject = \"p\"\n\n# b\n[[rule]]\napp = 'B'\nproject = \"p\"\n"
	if err := os.WriteFile(path, []byte(in), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := MoveRule(path, LearnedFile, 0, true); !errors.Is(err, ErrRuleEdge) {
		t.Fatalf("up from first: %v", err)
	}
	if err := MoveRule(path, LearnedFile, 1, false); !errors.Is(err, ErrRuleEdge) {
		t.Fatalf("down from last: %v", err)
	}
	if err := MoveRule(path, LearnedFile, 1, true); err != nil {
		t.Fatal(err)
	}
	got, _ := os.ReadFile(path)
	if want := learnedHeader + "# b\n[[rule]]\napp = 'B'\nproject = \"p\"\n\n# a\n[[rule]]\napp = 'A'\nproject = \"p\"\n"; string(got) != want {
		t.Fatalf("file =\n%s", got)
	}
	if fi, _ := os.Stat(path); fi.Mode().Perm() != 0o600 {
		t.Errorf("mode = %v", fi.Mode().Perm())
	}

	bad := "[[rule]\n"
	os.WriteFile(path, []byte(bad), 0o600)
	if err := MoveRule(path, LearnedFile, 0, false); err == nil {
		t.Fatal("bad file: no error")
	}
	if got, _ := os.ReadFile(path); string(got) != bad {
		t.Fatal("bad file was modified")
	}
}

func TestRemoveRules(t *testing.T) {
	in := "# head\n\n# a\n[[rule]]\napp = 'a'\ncategory = \"Code\"\n\n# b\n[[rule]]\napp = 'b'\ncategory = \"Code\"\n\n[[rule]]\napp = 'c'\ncategory = \"Code\"\n"
	out, err := RemoveRules(LearnedFile, []byte(in), []int{0, 2})
	if err != nil {
		t.Fatal(err)
	}
	if want := "# head\n\n# b\n[[rule]]\napp = 'b'\ncategory = \"Code\"\n\n"; string(out) != want {
		t.Fatalf("got %q", out)
	}
	if _, err := RemoveRules(LearnedFile, []byte(in), []int{3}); err == nil {
		t.Fatal("removed a missing rule")
	}
}
