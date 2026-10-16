package classify

import (
	"fmt"
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/BurntSushi/toml"
)

// Name kinds for Duplicates and MergeNames.
const (
	NameCategory = "category"
	NameProject  = "project"
)

// Dup is a pair of declared names of one kind that look like the same thing.
type Dup struct {
	Kind string // NameCategory or NameProject
	A, B string // in declaration order
}

// Duplicates pairs the categories, and the projects, whose names fold to the
// same text (case, spaces, punctuation, & for "and", a plural ending) or are
// one edit apart, two for names of 8 runes or more. Names under 5 runes
// only pair when they fold equal.
func Duplicates(rs *Ruleset) []Dup {
	var out []Dup
	pairs := func(kind string, names []string) {
		for i := range names {
			for j := i + 1; j < len(names); j++ {
				if similar(names[i], names[j]) {
					out = append(out, Dup{Kind: kind, A: names[i], B: names[j]})
				}
			}
		}
	}
	cats := make([]string, len(rs.Categories))
	for i, c := range rs.Categories {
		cats[i] = c.Name
	}
	projs := make([]string, len(rs.Projects))
	for i, p := range rs.Projects {
		projs[i] = p.Name
	}
	pairs(NameCategory, cats)
	pairs(NameProject, projs)
	return out
}

// plain lowercases s, spells & as "and" and keeps letters and digits only.
func plain(s string) string {
	return strings.Map(func(r rune) rune {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			return r
		}
		return -1
	}, strings.ReplaceAll(strings.ToLower(s), "&", "and"))
}

// fold is plain in the singular: a trailing "ies" becomes "y", else one
// trailing s goes.
func fold(s string) string {
	p := plain(s)
	switch {
	case utf8.RuneCountInString(p) <= 3:
	case strings.HasSuffix(p, "ies"):
		p = strings.TrimSuffix(p, "ies") + "y"
	default:
		p = strings.TrimSuffix(p, "s")
	}
	return p
}

func similar(a, b string) bool {
	if fold(a) == fold(b) {
		return true
	}
	fa, fb := plain(a), plain(b)
	n := min(utf8.RuneCountInString(fa), utf8.RuneCountInString(fb))
	switch {
	case n < 5:
		return false
	case n < 8:
		return editDistance(fa, fb) <= 1
	}
	return editDistance(fa, fb) <= 2
}

// editDistance is the Levenshtein distance between a and b in runes.
func editDistance(a, b string) int {
	ra, rb := []rune(a), []rune(b)
	prev := make([]int, len(rb)+1)
	cur := make([]int, len(rb)+1)
	for j := range prev {
		prev[j] = j
	}
	for i := 1; i <= len(ra); i++ {
		cur[0] = i
		for j := 1; j <= len(rb); j++ {
			cost := 1
			if ra[i-1] == rb[j-1] {
				cost = 0
			}
			cur[j] = min(prev[j]+1, cur[j-1]+1, prev[j-1]+cost)
		}
		prev, cur = cur, prev
	}
	return prev[len(rb)]
}

var (
	valueLine  = regexp.MustCompile(`^(\s*)(category|project)(\s*=\s*)("(?:[^"\\]|\\.)*"|'[^']*')(\s*(?:#.*)?)$`)
	tableStart = regexp.MustCompile(`^\s*\[\[\s*(category|project)\s*\]\]\s*(#.*)?$`)
	nameLine   = regexp.MustCompile(`^\s*name\s*=\s*("(?:[^"\\]|\\.)*"|'[^']*')`)
)

// Merged is the result of MergeNames: the declared spellings of the two
// names and the rewritten file contents.
type Merged struct {
	From, To       string
	Rules, Learned []byte
}

// MergeNames folds the declared name from into to (both of kind): the
// [[kind]] table named from is dropped from rules.toml, and every
// `kind = "from"` value in rules.toml and learned.toml becomes to. The
// result must parse with one name fewer, or nothing changes. A project
// with repos or a rate is refused, so neither is lost.
func MergeNames(rulesData, learnedData []byte, kind, from, to string) (Merged, error) {
	rs, err := Parse(rulesData, learnedData)
	if err != nil {
		return Merged{}, err
	}
	var src, dst string
	switch kind {
	case NameCategory:
		f, ok1 := rs.catByLower[nameKey(from)]
		t, ok2 := rs.catByLower[nameKey(to)]
		if !ok1 || !ok2 {
			return Merged{}, fmt.Errorf("merge: %q and %q must both be declared categories", from, to)
		}
		src, dst = f.Name, t.Name
	case NameProject:
		f, ok1 := rs.projByLower[nameKey(from)]
		t, ok2 := rs.projByLower[nameKey(to)]
		if !ok1 || !ok2 {
			return Merged{}, fmt.Errorf("merge: %q and %q must both be declared projects", from, to)
		}
		if len(f.Repos) > 0 || f.Rate != nil {
			return Merged{}, fmt.Errorf("merge: project %q has repos or a rate: move them to %q in rules.toml first", f.Name, t.Name)
		}
		src, dst = f.Name, t.Name
	default:
		return Merged{}, fmt.Errorf("merge: kind %q", kind)
	}
	if src == dst {
		return Merged{}, fmt.Errorf("merge: %q is the same name", src)
	}

	rules := []byte(dropTable(renameValues(string(rulesData), kind, src, dst), kind, src))
	learned := []byte(renameValues(string(learnedData), kind, src, dst))
	after, err := Parse(rules, learned)
	if err != nil {
		return Merged{}, fmt.Errorf("merge: the rewritten files do not parse (%v); edit them by hand", err)
	}
	if len(after.Categories)+len(after.Projects) != len(rs.Categories)+len(rs.Projects)-1 ||
		len(after.Rules) != len(rs.Rules) || len(after.Learned) != len(rs.Learned) {
		return Merged{}, fmt.Errorf("merge: can't rewrite this layout safely; edit rules.toml by hand")
	}
	return Merged{From: src, To: dst, Rules: rules, Learned: learned}, nil
}

// unquote reads a TOML basic or literal string token.
func unquote(tok string) string {
	if strings.HasPrefix(tok, "'") {
		return strings.Trim(tok, "'")
	}
	var v struct{ V string }
	if _, err := toml.Decode("v = "+tok, &v); err != nil {
		return ""
	}
	return v.V
}

// renameValues rewrites `kind = "from"` lines (case-insensitive) to to.
func renameValues(text, kind, from, to string) string {
	lines := strings.SplitAfter(text, "\n")
	for i, l := range lines {
		body := strings.TrimRight(l, "\r\n")
		m := valueLine.FindStringSubmatch(body)
		if m == nil || m[2] != kind || !strings.EqualFold(strings.TrimSpace(unquote(m[4])), from) {
			continue
		}
		lines[i] = m[1] + m[2] + m[3] + tomlString(to) + m[5] + l[len(body):]
	}
	return strings.Join(lines, "")
}

// dropTable removes the [[kind]] table whose name is name, with its comment
// lines above and one blank line after.
func dropTable(text, kind, name string) string {
	lines := strings.SplitAfter(text, "\n")
	for n, l := range lines {
		m := tableStart.FindStringSubmatch(strings.TrimRight(l, "\r\n"))
		if m == nil || m[1] != kind {
			continue
		}
		hi := n + 1
		match := false
		for hi < len(lines) && !isBlank(lines[hi]) && !tableHeader.MatchString(lines[hi]) {
			if nm := nameLine.FindStringSubmatch(lines[hi]); nm != nil && strings.EqualFold(strings.TrimSpace(unquote(nm[1])), name) {
				match = true
			}
			hi++
		}
		if !match {
			continue
		}
		lo := n
		for lo > 0 && isComment(lines[lo-1]) {
			lo--
		}
		if hi < len(lines) && isBlank(lines[hi]) {
			hi++
		}
		return strings.Join(lines[:lo], "") + strings.Join(lines[hi:], "")
	}
	return text
}
