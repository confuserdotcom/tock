package classify

import (
	"strings"
	"testing"
)

const fieldCats = `[[category]]
name = "Code"
kind = "deep"

[[category]]
name = "Video"
kind = "distract"

`

func TestSetRuleField(t *testing.T) {
	tests := []struct {
		name, file, in, want string
		i                    int
		value                string
	}{
		{
			name:  "insert right after the header, comment between keys",
			file:  RulesFile,
			in:    fieldCats + "# watch\n[[rule]]\napp = 'A'\n# why\ncategory = \"Video\"\n\n[[rule]]\napp = 'B'\ncategory = \"Code\"\n",
			want:  fieldCats + "# watch\n[[rule]]\ninterrupt = false\napp = 'A'\n# why\ncategory = \"Video\"\n\n[[rule]]\napp = 'B'\ncategory = \"Code\"\n",
			value: "false",
		},
		{
			name:  "replace in place keeps spacing and comment",
			file:  RulesFile,
			in:    fieldCats + "[[rule]]\napp = 'A'\ninterrupt  =  true # was loud\ncategory = \"Video\"\n",
			want:  fieldCats + "[[rule]]\napp = 'A'\ninterrupt  =  false # was loud\ncategory = \"Video\"\n",
			value: "false",
		},
		{
			name:  "replace after a comment line inside the rule",
			file:  RulesFile,
			in:    fieldCats + "[[rule]]\napp = 'A'\n# note\ninterrupt = true\ncategory = \"Video\"\n",
			want:  fieldCats + "[[rule]]\napp = 'A'\n# note\ninterrupt = false\ncategory = \"Video\"\n",
			value: "false",
		},
		{
			name: "remove the line",
			file: RulesFile,
			in:   fieldCats + "[[rule]]\napp = 'A'\ninterrupt = false\ncategory = \"Video\"\n",
			want: fieldCats + "[[rule]]\napp = 'A'\ncategory = \"Video\"\n",
		},
		{
			name: "remove from a rule without the key changes nothing",
			file: RulesFile,
			in:   fieldCats + "[[rule]]\napp = 'A'\ncategory = \"Video\"\n",
			want: fieldCats + "[[rule]]\napp = 'A'\ncategory = \"Video\"\n",
		},
		{
			name:  "learned.toml second rule only",
			file:  LearnedFile,
			in:    "[[rule]]\nbundle = [\"a\"]\ncategory = \"Video\"\n\n[[rule]]\nbundle = [\"b\"]\ncategory = \"Video\"\n",
			want:  "[[rule]]\nbundle = [\"a\"]\ncategory = \"Video\"\n\n[[rule]]\ninterrupt = false\nbundle = [\"b\"]\ncategory = \"Video\"\n",
			i:     1,
			value: "false",
		},
		{
			name:  "no trailing newline",
			file:  RulesFile,
			in:    fieldCats + "[[rule]]\napp = 'A'\ncategory = \"Video\"",
			want:  fieldCats + "[[rule]]\ninterrupt = false\napp = 'A'\ncategory = \"Video\"",
			value: "false",
		},
		{
			name: "no trailing newline, remove the last line",
			file: RulesFile,
			in:   fieldCats + "[[rule]]\napp = 'A'\ncategory = \"Video\"\ninterrupt = false",
			want: fieldCats + "[[rule]]\napp = 'A'\ncategory = \"Video\"",
		},
		{
			name:  "windows line endings",
			file:  RulesFile,
			in:    strings.ReplaceAll(fieldCats+"[[rule]]\napp = 'A'\ncategory = \"Video\"\n", "\n", "\r\n"),
			want:  strings.ReplaceAll(fieldCats+"[[rule]]\ninterrupt = false\napp = 'A'\ncategory = \"Video\"\n", "\n", "\r\n"),
			value: "false",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := SetRuleField(tt.file, []byte(tt.in), tt.i, "interrupt", tt.value)
			if err != nil {
				t.Fatal(err)
			}
			if string(got) != tt.want {
				t.Fatalf("got\n%q\nwant\n%q", got, tt.want)
			}
			checkOnlyQuietChanged(t, tt.file, tt.in, string(got), tt.i)
		})
	}
}

// checkOnlyQuietChanged parses both texts and checks that rule i's Quiet is
// the only thing that differs.
func checkOnlyQuietChanged(t *testing.T, file, in, out string, i int) {
	t.Helper()
	parse := func(text string) *Ruleset {
		if file == LearnedFile {
			return mustParse(t, fieldCats, text)
		}
		return mustParse(t, text, "")
	}
	a, b := parse(in), parse(out)
	ra, rb := a.Rules, b.Rules
	if file == LearnedFile {
		ra, rb = a.Learned, b.Learned
	}
	if len(ra) != len(rb) {
		t.Fatalf("rule count %d -> %d", len(ra), len(rb))
	}
	for n := range ra {
		x, y := ra[n], rb[n]
		if n != i {
			if x.Quiet != y.Quiet {
				t.Errorf("rule %d Quiet changed", n+1)
			}
		}
		x.Quiet, y.Quiet = false, false
		x.app, x.title, x.url, x.dir = nil, nil, nil, nil
		y.app, y.title, y.url, y.dir = nil, nil, nil, nil
		if x.Fingerprint() != y.Fingerprint() || x.Index != y.Index {
			t.Errorf("rule %d changed: %+v vs %+v", n+1, x, y)
		}
	}
}

func TestSetRuleFieldRange(t *testing.T) {
	in := fieldCats + "[[rule]]\napp = 'A'\ncategory = \"Video\"\n\n[[rule]]\napp = 'B'\ncategory = \"Video\"\n"
	for _, i := range []int{5, -1, 2} {
		_, err := SetRuleField(RulesFile, []byte(in), i, "interrupt", "false")
		if err == nil || !strings.Contains(err.Error(), RulesFile) {
			t.Errorf("i=%d: err = %v, want error naming %s", i, err, RulesFile)
		}
	}
	_, err := SetRuleField(LearnedFile, []byte("[[rule]\n"), 0, "interrupt", "false")
	if err == nil || !strings.Contains(err.Error(), LearnedFile) {
		t.Errorf("bad toml: err = %v, want error naming %s", err, LearnedFile)
	}
}

func TestSetRuleFieldRefuses(t *testing.T) {
	tests := []struct {
		name, in string
		i        int
	}{
		{
			// the scanner counts the [[rule]] line inside the literal string as
			// a rule, so index 1 points into the first rule's string
			name: "fake rule header inside a multi-line string",
			in:   fieldCats + "[[rule]]\napp = 'A'\ncategory = \"Video\"\ntitle = '''\n[[rule]]\n'''\n\n[[rule]]\napp = 'B'\ncategory = \"Video\"\n",
			i:    1,
		},
		{
			// the key line sits inside a multi-line string: editing it would
			// change the title, not the flag
			name: "key text inside a multi-line string",
			in:   fieldCats + "[[rule]]\napp = 'A'\ntitle = '''\ninterrupt = true\n'''\ncategory = \"Video\"\n",
			i:    0,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := SetRuleField(RulesFile, []byte(tt.in), tt.i, "interrupt", "false")
			if err == nil {
				t.Fatalf("expected refusal, got\n%s", got)
			}
			if got != nil {
				t.Errorf("returned %q alongside the error", got)
			}
			if !strings.Contains(err.Error(), RulesFile) || !strings.Contains(err.Error(), "edit the file") {
				t.Errorf("err = %v, want file name and \"edit the file\"", err)
			}
		})
	}
}
