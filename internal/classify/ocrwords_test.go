package classify

import (
	"fmt"
	"regexp"
	"slices"
	"strings"
	"testing"
	"unicode"
)

func TestKeywords(t *testing.T) {
	deny := []*regexp.Regexp{regexp.MustCompile("(?i)secret")}
	cases := []struct {
		name string
		in   []string
		want []string
	}{
		{"frequency then alphabetical", []string{"kubectl rollout kubectl", "Deployment rollout kubectl"}, []string{"kubectl", "rollout", "deployment"}},
		{"drops digits emails urls", []string{"pay bob@x.org 123456 integrale integrale v2 https://a.com/x www.b.fr c.io/path"}, []string{"integrale", "pay"}},
		{"en and fr stop words", []string{"le chat et la fonction with the and"}, []string{"chat", "fonction"}},
		{"accents kept, lowercased", []string{"Théorème INTÉGRALE"}, []string{"intégrale", "théorème"}},
		{"too short and too long", []string{"ab abc " + strings.Repeat("x", 25)}, []string{"abc"}},
		{"token-like", []string{"deadbeefcafe0 aZkQpLmX qwrtzxcvb getUserName"}, nil},
		{"hex words", []string{"deadbeefcafe facade"}, []string{"facade"}},
		{"deny title patterns", []string{"secretive notes"}, []string{"notes"}},
		{"punctuation splits words", []string{"(matrix)—vector, \"proof\"."}, []string{"matrix", "proof", "vector"}},
		{"empty", nil, nil},
	}
	for _, c := range cases {
		got := Keywords(c.in, deny)
		if !slices.Equal(got, c.want) {
			t.Errorf("%s: Keywords = %q, want %q", c.name, got, c.want)
		}
	}
}

func TestKeywordsBounded(t *testing.T) {
	var b strings.Builder
	for i := range 100 {
		fmt.Fprintf(&b, "word%c%c ", 'a'+i%26, 'a'+i/26)
	}
	got := Keywords([]string{b.String()}, nil)
	if len(got) != MaxWordsPerRead {
		t.Fatalf("len = %d, want %d", len(got), MaxWordsPerRead)
	}
	for _, w := range got {
		for _, r := range w {
			if !unicode.IsLetter(r) {
				t.Fatalf("%q has a non-letter", w)
			}
		}
	}
}

func TestMergeWords(t *testing.T) {
	if got := MergeWords([]string{"a1", "b1"}, []string{"b1", "c1"}); !slices.Equal(got, []string{"b1", "c1", "a1"}) {
		t.Fatalf("merge = %q", got)
	}
	var old, add []string
	for i := range 40 {
		old = append(old, fmt.Sprintf("o%02d", i))
		add = append(add, fmt.Sprintf("n%02d", i))
	}
	got := MergeWords(old, add)
	if len(got) != MaxWordsPerSpan || got[0] != "n00" || got[39] != "n39" || got[40] != "o00" {
		t.Fatalf("cap: len %d, %q", len(got), got)
	}
}
