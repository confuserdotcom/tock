package classify

import (
	"strings"
	"testing"
)

func TestDirRule(t *testing.T) {
	rs, err := Parse([]byte(baseRules+`
[[rule]]
dir = '^~/code/tock(/|$)'
project = "tock"

[[rule]]
bundle = ["term"]
category = "Code"
`), nil)
	if err != nil {
		t.Fatal(err)
	}
	if got := rs.Rules[0].Describe(); got != `dir='^~/code/tock(/|$)'` {
		t.Fatalf("Describe = %s", got)
	}
	for _, c := range []struct {
		dir  *string
		want string
	}{
		{sp("~/code/tock/sub"), "tock"},
		{sp("~/code/tock"), "tock"},
		{sp("~/code/tockx"), ""},
		{nil, ""},
	} {
		v := SpanView{ID: 1, Start: 0, End: 1, BundleID: "term", App: "T", Title: sp("zsh"), Dir: c.dir}
		if l := rs.Label(v, nil); l.Project != c.want || l.Category != "Code" {
			t.Errorf("dir %v: label %+v, want project %q", c.dir, l, c.want)
		}
	}
}

func TestDirRuleBadRegex(t *testing.T) {
	_, err := Parse([]byte(baseRules+"[[rule]]\ndir = '('\nproject = \"tock\"\n"), nil)
	if err == nil || !strings.Contains(err.Error(), "bad dir regex") {
		t.Fatalf("err = %v", err)
	}
}

func TestDirLearnedRoundTrip(t *testing.T) {
	rs, err := Parse([]byte(baseRules), []byte("[[rule]]\ndir = '^~/x'\nproject = \"tock\"\n"))
	if err != nil {
		t.Fatal(err)
	}
	if rs.Learned[0].Dir != "^~/x" {
		t.Fatalf("learned dir = %q", rs.Learned[0].Dir)
	}
}
